# Build, test, promote

Both workflows publish to `ghcr.io/akhil850/k8s-cloudwatch-adapter`. Each candidate is a multi-architecture index for `linux/amd64` and `linux/arm64`.

## Candidate builds

| Event | Behavior |
| --- | --- |
| PR to `master` | Policy tests, Go race tests, and a multi-architecture build; no publishing |
| Push to `master` | Tests and candidate publication under the full/seven-character commit SHA; updates `master` only if the source is still the branch head |
| Manual build | Tests the selected ref; publishes a commit image when `publish=true` |
| Documentation-only push/PR | Policy tests and the `CI` result; no Go tests or image build |
| Git version-tag push | No image build or promotion |

Use `CI` as the required pull-request check. Do not use `[skip ci]` for routine documentation changes: the workflow detects them without leaving a required workflow pending.

The candidate's digest appears in the build job summary and the `image-metadata` artifact. An existing full-SHA image is verified and reused, not rebuilt or overwritten. The seven-character alias also refuses overwrites on a collision. Registry authentication or network errors fail the job rather than causing a rebuild.

## Test in EKS

Deploy the exact candidate digest using your normal deployment process:

```text
ghcr.io/akhil850/k8s-cloudwatch-adapter@sha256:<index-digest>
```

Keep the previous digest for rollback. Follow [non-production validation](nonprod-validation.md), including metric delivery and the relevant scaling/failure checks. A successful build is not evidence of EKS runtime validation.

## Promote the tested image

Run [Promote validated image](https://github.com/akhil850/k8s-cloudwatch-adapter/actions/workflows/promote-image.yml) with the workflow branch set to `master`:

| Input | Value |
| --- | --- |
| `build_run_id` | Successful **Docker build and publish** run that tested the source commit |
| `digest` | Full `sha256:...` index digest tested in EKS |
| `version` | Container version such as `v1.0.0` or `v1.1.0-rc.1` |

Promotion verifies the run's test/build jobs, the source commit's ancestry on `master`, the canonical commit tag's digest, and both platform revision labels. The selected digest must still match its full-SHA tag. These checks also support candidates built by the previous workflow, provided their successful run, commit tag, and revision labels are available.

- Stable version: add the version tag and move `latest` to the same digest.
- Prerelease: add the version tag and leave `latest` unchanged.
- An existing version can only be reused for the same digest. Choosing a stable release explicitly moves `latest`, including when selecting an older release for rollback.

Promotion copies the existing index and verifies the resulting tags' digests. It performs no build, creates no Git tag or GitHub release, and makes no EKS deployment changes. Publication jobs are serialized; GitHub may replace an older pending run with a newer one, so confirm the requested run completed.

Production deployments should use the tested digest. `master` and `latest` are moving aliases and changing them does not restart existing pods.
