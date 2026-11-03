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
        environment = patch.dict(os.environ, {
            "IMAGE_NAME": IMAGE, "IMAGE_DIGEST": DIGEST, "GITHUB_SHA": REVISION,
            "GITHUB_REPOSITORY": "example/adapter", "GITHUB_RUN_ID": "456",
            "GITHUB_EVENT_NAME": "push", "GITHUB_REF": "refs/heads/master",
            "GITHUB_OUTPUT": str(self.directory / "output"),
            "GITHUB_STEP_SUMMARY": str(self.directory / "summary"), "PUBLISH": "true",
        })
        environment.start()
        self.addCleanup(environment.stop)
        path_patch = patch.object(ci, "Path", side_effect=lambda name: self.directory / name)
        path_patch.start()
        self.addCleanup(path_patch.stop)
        self.registry = {f"{IMAGE}@{DIGEST}": copy.deepcopy(INDEX)}
        self.head = REVISION
        self.label = REVISION
        self.commands = []
        self.inspections = []
        inspect_patch = patch.object(ci, "inspect", side_effect=self.inspect)
        inspect_patch.start()
        self.addCleanup(inspect_patch.stop)
        run_patch = patch.object(ci, "run", side_effect=self.run_command)
        run_patch.start()
        self.addCleanup(run_patch.stop)

    def inspect(self, reference, optional=False):
        self.inspections.append(reference)
        if reference in self.registry:
            return copy.deepcopy(self.registry[reference])
        if optional:
            return None
        raise RuntimeError("image not found")

    def run_command(self, *args):
        self.commands.append(args)
        if args[:2] == ("gh", "api"):
            return json.dumps({"object": {"sha": self.head}})
        if args[:4] == ("docker", "buildx", "imagetools", "inspect"):
            return json.dumps({"config": {"Labels": {"org.opencontainers.image.revision": self.label}}})
        if args[:4] == ("docker", "buildx", "imagetools", "create"):
            self.registry[args[5]] = copy.deepcopy(self.registry[args[6]])
            return ""
        self.fail(f"Unexpected external command: {args}")

    def publications(self):
        return [args for args in self.commands if args[:4] == ("docker", "buildx", "imagetools", "create")]

    def test_master_build_publishes_only_short_sha_and_latest(self):
        ci.record()
        tags = {key for key in self.registry if "@" not in key}
        self.assertEqual(tags, {f"{IMAGE}:{REVISION[:7]}", f"{IMAGE}:latest"})
        for tag in tags:
            self.assertEqual(self.registry[tag], INDEX)
        self.assertIn(f"{IMAGE}@{DIGEST}", (self.directory / "summary").read_text())
        self.assertEqual(json.loads((self.directory / "image-metadata.json").read_text())["revision"], REVISION)
        self.assertFalse(any("build" in args for args in self.commands))

    def test_manual_current_master_build_also_updates_latest(self):
        os.environ["GITHUB_EVENT_NAME"] = "workflow_dispatch"
        ci.record()
        self.assertEqual(self.registry[f"{IMAGE}:latest"]["digest"], DIGEST)

    def test_branch_build_preserves_latest(self):
        os.environ["GITHUB_EVENT_NAME"] = "workflow_dispatch"
        os.environ["GITHUB_REF"] = "refs/heads/feature"
        self.registry[f"{IMAGE}:latest"] = {"digest": OTHER_DIGEST}
        ci.record()
        self.assertEqual(self.registry[f"{IMAGE}:latest"]["digest"], OTHER_DIGEST)
        self.assertEqual(len(self.publications()), 1)

    def test_old_master_run_cannot_move_latest(self):
        self.head = "f" * 40
        self.registry[f"{IMAGE}:latest"] = {"digest": OTHER_DIGEST}
        ci.record()
        self.assertEqual(self.registry[f"{IMAGE}:latest"]["digest"], OTHER_DIGEST)
        self.assertEqual(self.registry[f"{IMAGE}:{REVISION[:7]}"]["digest"], DIGEST)

    def test_existing_short_sha_image_is_reused(self):
        self.registry[f"{IMAGE}:{REVISION[:7]}"] = copy.deepcopy(INDEX)
        ci.prepare()
        output = (self.directory / "output").read_text()
        self.assertIn("reuse=true", output)
        self.assertIn(f"tag={REVISION[:7]}", output)
        self.assertIn(f"digest={DIGEST}", output)
        self.assertEqual(self.publications(), [])

    def test_new_short_sha_image_can_be_built(self):
        ci.prepare()
        output = (self.directory / "output").read_text()
        self.assertIn("reuse=false", output)
        self.assertIn(f"tag={REVISION[:7]}", output)
        self.assertEqual(self.commands, [])

    def test_short_sha_collision_blocks_build_before_overwrite(self):
        self.registry[f"{IMAGE}:{REVISION[:7]}"] = copy.deepcopy(INDEX)
        self.label = "f" * 40
        with self.assertRaisesRegex(ValueError, "does not match"):
            ci.prepare()
        self.assertEqual(self.publications(), [])

    def test_existing_short_sha_cannot_be_overwritten(self):
        self.registry[f"{IMAGE}:{REVISION[:7]}"] = {"digest": OTHER_DIGEST}
        with self.assertRaisesRegex(ValueError, "Refusing to overwrite"):
            ci.record()
        self.assertEqual(self.publications(), [])

    def test_missing_architecture_blocks_reuse(self):
        self.registry[f"{IMAGE}:{REVISION[:7]}"] = copy.deepcopy(INDEX)
        self.registry[f"{IMAGE}@{DIGEST}"]["manifests"] = INDEX["manifests"][:1]
        with self.assertRaisesRegex(ValueError, "missing linux/arm64"):
            ci.prepare()
        self.assertEqual(self.publications(), [])

    def test_latest_is_verified_after_publication(self):
        original = self.run_command

        def wrong_digest(*args):
            result = original(*args)
            if args[:4] == ("docker", "buildx", "imagetools", "create") and args[5] == f"{IMAGE}:latest":
                self.registry[f"{IMAGE}:latest"]["digest"] = OTHER_DIGEST
            return result

        with patch.object(ci, "run", side_effect=wrong_digest), self.assertRaisesRegex(ValueError, "latest did not preserve"):
            ci.record()

    def test_pr_build_does_not_access_registry(self):
        os.environ["PUBLISH"] = "false"
        ci.prepare()
        self.assertEqual(self.commands, [])
        self.assertEqual(self.inspections, [])
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


class ValidationTests(unittest.TestCase):
    def test_only_documentation_changes_skip_builds(self):
        self.assertFalse(ci.relevant_changes(["README.md", "ROADMAP.md", "docs/schema.md"]))
        for path in ("pkg/aws/client.go", "Dockerfile", "go.mod", "LICENSE", "NOTICE",
                     ".github/workflows/docker-publish.yml", "hack/ci_images.py"):
            with self.subTest(path=path):
                self.assertTrue(ci.relevant_changes(["README.md", path]))

    def test_invalid_digests_and_revisions(self):
        for digest in ("latest", "sha256:abc", "sha256:" + "x" * 64):
            with self.assertRaises(ValueError):
                ci.valid_digest(digest)
        for revision in ("latest", "1234567", "z" * 40):
            with self.assertRaises(ValueError):
                ci.valid_revision(revision)

    def test_registry_failures_do_not_trigger_rebuild(self):
        for message in ("unauthorized", "connection refused", "429 Too Many Requests", 'template function "json" not found'):
            result = subprocess.CompletedProcess([], 1, stdout="", stderr=message)
            with patch.object(ci.subprocess, "run", return_value=result), self.assertRaises(RuntimeError):
                ci.inspect(f"{IMAGE}:{REVISION[:7]}", optional=True)

    def test_absent_registry_manifest_can_be_built(self):
        reference = f"{IMAGE}:{REVISION[:7]}"
        for message in ("manifest unknown", f"ERROR: {reference}: not found\n"):
            result = subprocess.CompletedProcess([], 1, stdout="", stderr=message)
            with patch.object(ci.subprocess, "run", return_value=result):
                self.assertIsNone(ci.inspect(reference, optional=True))


if __name__ == "__main__":
    unittest.main()
