"""Short-SHA image builds and automatic latest tagging. Python stdlib only."""

import json
import os
from pathlib import Path
import re
import subprocess
import sys


def run(*args):
    return subprocess.check_output(args, text=True).strip()


def valid_digest(value):
    if not re.fullmatch(r"sha256:[0-9a-f]{64}", value):
        raise ValueError("Expected a full sha256 image-index digest")
    return value


def valid_revision(value):
    if not re.fullmatch(r"[0-9a-f]{40}", value):
        raise ValueError("Expected a full source commit SHA")
    return value


def relevant_changes(paths):
    return any(not (p.endswith(".md") or p.startswith("docs/")) for p in paths)


def output(**values):
    with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as stream:
        for key, value in values.items():
            stream.write(f"{key}={value}\n")


def summary(message):
    with open(os.environ["GITHUB_STEP_SUMMARY"], "a", encoding="utf-8") as stream:
        stream.write(message + "\n")


def changes():
    base = os.environ.get("BASE_SHA", "")
    if os.environ["EVENT_NAME"] == "workflow_dispatch" or not base or set(base) == {"0"}:
        relevant = True
    else:
        # PRs compare their target base to GitHub's tested merge commit.
        paths = run("git", "diff", "--no-renames", "--name-only", "-z", base, os.environ["GITHUB_SHA"]).split("\0")
        relevant = relevant_changes([p for p in paths if p])
    output(relevant=str(relevant).lower())


def inspect(reference, optional=False):
    result = subprocess.run(
        ["docker", "buildx", "imagetools", "inspect", reference, "--format", "{{json .Manifest}}"],
        text=True, capture_output=True, check=False,
    )
    if result.returncode:
        # Authentication, rate limits, and transport failures must not be mistaken for absence.
        missing = rf"manifest unknown|manifest_unknown|(?:^|\n)(?:ERROR:\s*)?{re.escape(reference)}: not found\s*$"
        if optional and re.search(missing, result.stderr, re.I):
            return None
        raise RuntimeError(f"Cannot inspect {reference}: {result.stderr.strip()}")
    manifest = json.loads(result.stdout)
    valid_digest(manifest["digest"])
    return manifest


def verify_candidate(image, digest, revision):
    manifest = inspect(f"{image}@{digest}")
    if manifest["digest"] != digest:
        raise ValueError("Registry digest differs from the requested candidate")
    platforms = {
        (m.get("platform", {}).get("os"), m.get("platform", {}).get("architecture")): m
        for m in manifest.get("manifests", [])
    }
    for architecture in ("amd64", "arm64"):
        child = platforms.get(("linux", architecture))
        if child is None:
            raise ValueError(f"Candidate is missing linux/{architecture}")
        config = json.loads(run(
            "docker", "buildx", "imagetools", "inspect", f"{image}@{valid_digest(child['digest'])}",
            "--format", "{{json .Image}}",
        ))
        if config.get("config", {}).get("Labels", {}).get("org.opencontainers.image.revision") != revision:
            raise ValueError(f"linux/{architecture} does not match source commit {revision}")


def immutable_tag(image, tag, digest):
    existing = inspect(f"{image}:{tag}", optional=True)
    if existing and existing["digest"] != digest:
        raise ValueError(f"Refusing to overwrite existing tag {tag} with a different digest")
    if existing is None:
        run("docker", "buildx", "imagetools", "create", "--tag", f"{image}:{tag}", f"{image}@{digest}")
    if inspect(f"{image}:{tag}")["digest"] != digest:
        raise ValueError(f"Tag {tag} did not preserve the candidate digest")


def prepare():
    revision = valid_revision(os.environ["GITHUB_SHA"])
    output(tag=revision[:7])
    if os.environ["PUBLISH"] != "true":
        output(reuse="false")
        return
    image = os.environ["IMAGE_NAME"]
    existing = inspect(f"{image}:{revision[:7]}", optional=True)
    if existing:
        verify_candidate(image, existing["digest"], revision)
        output(reuse="true", digest=existing["digest"])
    else:
        output(reuse="false")


def record():
    image = os.environ["IMAGE_NAME"]
    digest = valid_digest(os.environ["IMAGE_DIGEST"])
    revision = valid_revision(os.environ["GITHUB_SHA"])
    verify_candidate(image, digest, revision)
    immutable_tag(image, revision[:7], digest)
    if (os.environ["GITHUB_EVENT_NAME"] in ("push", "workflow_dispatch")
            and os.environ["GITHUB_REF"] == "refs/heads/master"):
        head = json.loads(run("gh", "api", f"repos/{os.environ['GITHUB_REPOSITORY']}/git/ref/heads/master"))["object"]["sha"]
        if head == revision:
            run("docker", "buildx", "imagetools", "create", "--tag", f"{image}:latest", f"{image}@{digest}")
            if inspect(f"{image}:latest")["digest"] != digest:
                raise ValueError("latest did not preserve the build digest")
        else:
            print("A newer master commit exists; leaving latest unchanged.")
    metadata = dict(image=image, digest=digest, revision=revision, run_id=os.environ["GITHUB_RUN_ID"])
    Path("image-metadata.json").write_text(json.dumps(metadata, indent=2) + "\n", encoding="utf-8")
    summary(f"## Published image\n\nTag: `{image}:{revision[:7]}`\n\n`{image}@{digest}`\n\n"
            f"Commit: `{revision}`; build run: `{metadata['run_id']}`.")


if __name__ == "__main__":
    commands = {"changes": changes, "prepare": prepare, "record": record}
    try:
        commands[sys.argv[1]]()
    except (ValueError, RuntimeError, subprocess.CalledProcessError) as error:
        sys.exit(str(error))
