import copy
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch


spec = importlib.util.spec_from_file_location("ci_images", Path(__file__).parents[1] / "ci_images.py")
ci = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ci)

IMAGE = "ghcr.io/example/adapter"
REVISION = "a" * 40
DIGEST = "sha256:" + "b" * 64
OTHER_DIGEST = "sha256:" + "c" * 64
INDEX = {
    "digest": DIGEST,
    "manifests": [
        {"digest": "sha256:" + "d" * 64, "platform": {"os": "linux", "architecture": "amd64"}},
        {"digest": "sha256:" + "e" * 64, "platform": {"os": "linux", "architecture": "arm64"}},
    ],
}


class ImagePolicyTests(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.directory = Path(directory.name)
        env = {
            "IMAGE_NAME": IMAGE, "IMAGE_DIGEST": DIGEST, "GITHUB_SHA": REVISION,
            "GITHUB_REPOSITORY": "example/adapter", "GITHUB_RUN_ID": "456",
            "GITHUB_EVENT_NAME": "push", "GITHUB_REF": "refs/heads/master",
            "GITHUB_OUTPUT": str(self.directory / "output"),
            "GITHUB_STEP_SUMMARY": str(self.directory / "summary"),
            "PUBLISH": "true", "BUILD_RUN_ID": "123", "RELEASE_VERSION": "v1.2.3",
        }
        environment = patch.dict(os.environ, env)
        environment.start()
        self.addCleanup(environment.stop)
        path_patch = patch.object(ci, "Path", side_effect=lambda name: self.directory / name)
        path_patch.start()
        self.addCleanup(path_patch.stop)
        self.registry = {f"{IMAGE}@{DIGEST}": copy.deepcopy(INDEX), f"{IMAGE}:{REVISION}": copy.deepcopy(INDEX)}
        self.build = dict(status="completed", conclusion="success", path=".github/workflows/docker-publish.yml",
                          event="push", head_sha=REVISION, head_repository={"full_name": "example/adapter"})
        self.jobs = [{"name": "test", "conclusion": "success"}, {"name": "build", "conclusion": "success"}]
        self.head = REVISION
        self.merged = True
        self.label = REVISION
        self.commands = []
        inspect_patch = patch.object(ci, "inspect", side_effect=self.inspect)
        inspect_patch.start()
        self.addCleanup(inspect_patch.stop)
        run_patch = patch.object(ci, "run", side_effect=self.run_command)
        run_patch.start()
        self.addCleanup(run_patch.stop)

    def inspect(self, reference, optional=False):
        if reference in self.registry:
            return copy.deepcopy(self.registry[reference])
        if optional:
            return None
        raise RuntimeError("image not found")

    def run_command(self, *args):
        self.commands.append(args)
        if args[:2] == ("gh", "api"):
            if "/jobs?" in args[2]:
                return json.dumps({"jobs": self.jobs})
            if "/git/ref/" in args[2]:
                return json.dumps({"object": {"sha": self.head}})
            return json.dumps(self.build)
        if args[:3] == ("git", "merge-base", "--is-ancestor"):
            if not self.merged:
                raise subprocess.CalledProcessError(1, args)
            return ""
        if args[:4] == ("docker", "buildx", "imagetools", "inspect"):
            return json.dumps({"config": {"Labels": {"org.opencontainers.image.revision": self.label}}})
        if args[:4] == ("docker", "buildx", "imagetools", "create"):
            self.registry[args[5]] = copy.deepcopy(self.registry[args[6]])
            return ""
        self.fail(f"Unexpected external command: {args}")

    def publications(self):
        return [args for args in self.commands if args[:4] == ("docker", "buildx", "imagetools", "create")]

    def test_stable_promotion_preserves_digest_and_both_architectures(self):
        ci.promote()
        for tag in ("v1.2.3", "latest"):
            self.assertEqual(self.registry[f"{IMAGE}:{tag}"], INDEX)
        self.assertEqual(len(self.publications()), 2)
        self.assertFalse(any("build" in args for args in self.commands))

    def test_prerelease_does_not_move_latest(self):
        os.environ["RELEASE_VERSION"] = "v1.2.3-rc.1"
        ci.promote()
        self.assertIn(f"{IMAGE}:v1.2.3-rc.1", self.registry)
        self.assertNotIn(f"{IMAGE}:latest", self.registry)

    def test_existing_version_cannot_be_overwritten(self):
        self.registry[f"{IMAGE}:v1.2.3"] = {"digest": OTHER_DIGEST}
        with self.assertRaisesRegex(ValueError, "Refusing to overwrite"):
            ci.promote()
        self.assertEqual(self.publications(), [])

    def test_promotion_is_idempotent(self):
        ci.promote()
        self.commands.clear()
        ci.promote()
        self.assertEqual(len(self.publications()), 1)  # latest only; version is reused

    def test_reject_untrusted_or_unsuccessful_build_runs(self):
        for key, bad in (("status", "in_progress"), ("conclusion", "failure"),
                         ("path", "other.yml"), ("event", "pull_request"),
                         ("head_repository", {"full_name": "other/repo"})):
            with self.subTest(key=key), patch.dict(self.build, {key: bad}):
                with self.assertRaises(ValueError):
                    ci.promote()
                self.assertEqual(self.publications(), [])

    def test_skipped_tests_or_builds_cannot_be_promoted(self):
        for name in ("test", "build"):
            with self.subTest(job=name):
                self.jobs = [{"name": job, "conclusion": "skipped" if job == name else "success"}
                             for job in ("test", "build")]
                with self.assertRaisesRegex(ValueError, "did not succeed"):
                    ci.promote()
                self.assertEqual(self.publications(), [])

    def test_unmerged_source_cannot_be_promoted(self):
        self.merged = False
        with self.assertRaises(subprocess.CalledProcessError):
            ci.promote()
        self.assertEqual(self.publications(), [])

    def test_only_master_can_promote(self):
        os.environ["GITHUB_REF"] = "refs/heads/feature"
        with self.assertRaisesRegex(ValueError, "from master"):
            ci.promote()
        self.assertEqual(self.commands, [])

    def test_wrong_digest_or_revision_cannot_be_promoted(self):
        self.registry[f"{IMAGE}:{REVISION}"]["digest"] = OTHER_DIGEST
        with self.assertRaisesRegex(ValueError, "differs"):
            ci.promote()
        self.registry[f"{IMAGE}:{REVISION}"] = copy.deepcopy(INDEX)
        self.label = "f" * 40
        with self.assertRaisesRegex(ValueError, "does not match"):
            ci.promote()
        self.assertEqual(self.publications(), [])

    def test_missing_architecture_blocks_promotion(self):
        self.registry[f"{IMAGE}@{DIGEST}"]["manifests"] = INDEX["manifests"][:1]
        with self.assertRaisesRegex(ValueError, "missing linux/arm64"):
            ci.promote()
        self.assertEqual(self.publications(), [])

    def test_existing_commit_image_is_reused(self):
        ci.prepare()
        self.assertIn("reuse=true", (self.directory / "output").read_text())
        self.assertEqual(self.publications(), [])

    def test_pr_build_does_not_access_registry(self):
        os.environ["PUBLISH"] = "false"
        ci.prepare()
        self.assertEqual(self.commands, [])
        self.assertIn("reuse=false", (self.directory / "output").read_text())

    def test_documentation_diff_skips_builds_and_disables_rename_detection(self):
        with patch.dict(os.environ, {"EVENT_NAME": "push", "BASE_SHA": "f" * 40}), \
                patch.object(ci, "run", return_value="README.md\0docs/schema.md\0") as command:
            ci.changes()
        self.assertIn("relevant=false", (self.directory / "output").read_text())
        self.assertIn("--no-renames", command.call_args.args)

    def test_manual_and_initial_runs_force_validation(self):
        for event, base in (("workflow_dispatch", REVISION), ("push", "0" * 40)):
            with self.subTest(event=event), patch.dict(os.environ, {"EVENT_NAME": event, "BASE_SHA": base}):
                ci.changes()
        self.assertEqual((self.directory / "output").read_text().count("relevant=true"), 2)
        self.assertEqual(self.commands, [])

    def test_old_master_run_cannot_move_master_tag(self):
        self.head = "f" * 40
        ci.record()
        self.assertNotIn(f"{IMAGE}:master", self.registry)
        self.assertEqual(json.loads((self.directory / "image-metadata.json").read_text())["digest"], DIGEST)

    def test_current_master_run_records_and_publishes_digest(self):
        ci.record()
        for tag in (REVISION, REVISION[:7], "master"):
            self.assertEqual(self.registry[f"{IMAGE}:{tag}"]["digest"], DIGEST)
        self.assertIn(f"{IMAGE}@{DIGEST}", (self.directory / "summary").read_text())

    def test_short_sha_collision_is_not_overwritten(self):
        self.registry[f"{IMAGE}:{REVISION[:7]}"] = {"digest": OTHER_DIGEST}
        with self.assertRaisesRegex(ValueError, "Refusing to overwrite"):
            ci.record()


class ValidationTests(unittest.TestCase):
    def test_only_documentation_changes_skip_builds(self):
        self.assertFalse(ci.relevant_changes(["README.md", "ROADMAP.md", "docs/schema.md"]))
        for path in ("pkg/aws/client.go", "Dockerfile", "go.mod", "LICENSE", "NOTICE",
                     ".github/workflows/promote-image.yml", "hack/ci_images.py"):
            with self.subTest(path=path):
                self.assertTrue(ci.relevant_changes(["README.md", path]))

    def test_invalid_promotion_inputs(self):
        for version in ("latest", "master", "v1.2", "v01.2.3", "v1.2.3-01", "v1.2.3+build", "v1.2.3;echo bad"):
            with self.subTest(version=version), self.assertRaises(ValueError):
                ci.stable_version(version)
        for digest in ("latest", "sha256:abc", "sha256:" + "x" * 64):
            with self.assertRaises(ValueError):
                ci.valid_digest(digest)

    def test_registry_failures_do_not_trigger_rebuild(self):
        for message in ("unauthorized", "connection refused", "429 Too Many Requests", 'template function "json" not found'):
            result = subprocess.CompletedProcess([], 1, stdout="", stderr=message)
            with patch.object(ci.subprocess, "run", return_value=result), self.assertRaises(RuntimeError):
                ci.inspect(f"{IMAGE}:{REVISION}", optional=True)

    def test_absent_registry_manifest_can_be_built(self):
        for message in ("manifest unknown", f"ERROR: {IMAGE}:{REVISION}: not found\n"):
            result = subprocess.CompletedProcess([], 1, stdout="", stderr=message)
            with patch.object(ci.subprocess, "run", return_value=result):
                self.assertIsNone(ci.inspect(f"{IMAGE}:{REVISION}", optional=True))


if __name__ == "__main__":
    unittest.main()
