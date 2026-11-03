# Image builds

Registry: `ghcr.io/akhil850/k8s-cloudwatch-adapter`. All published images support `linux/amd64` and `linux/arm64`.

## Two tags

| Tag | Meaning |
| --- | --- |
| `latest` | Current successful master build; updated automatically from the same digest |
| `<short-sha>` | Seven-character commit SHA identifying a specific build |

There is no separate `master` tag, full-SHA tag, version-tag publishing, or manual promotion workflow. Full source SHAs remain in image labels and metadata for traceability and collision checks. Historical registry tags are preserved; this policy applies to new publications.

## Workflow

| Event | Behavior |
| --- | --- |
| PR to `master` | Policy tests, Go race tests, and a multi-architecture build; no publishing |
| Push to `master` | Tests, publishes the short SHA, and updates `latest` if the source is still the branch head |
| Manual build on `master` | Tests and publishes when `publish=true`; a current build also updates `latest` |
| Manual build on another branch | Tests and publishes only its short SHA when `publish=true`; leaves `latest` unchanged |
| Documentation-only push/PR | Policy tests and the `CI` result; no Go tests or image build |
| Git tag push | No image build |

Use `CI` as the required pull-request check. Routine documentation changes do not need `[skip ci]`.

Publication jobs are serialized. Existing short-SHA images are verified against the full source revision and reused. Collisions, mismatched platform labels, and registry transport/authentication errors fail the job rather than overwriting a build. The build summary and `image-metadata` artifact record the digest.

## Deploy a specific build

Use the short SHA tag, or the exact digest tested in staging:

```text
ghcr.io/akhil850/k8s-cloudwatch-adapter:<short-sha>
ghcr.io/akhil850/k8s-cloudwatch-adapter@sha256:<index-digest>
```

Keep the previous digest for rollback. See [non-production validation](nonprod-validation.md) for runtime checks. Moving `latest` does not restart existing pods or deploy to EKS.
