# Maintenance roadmap

This fork supports an existing client deployment of the archived AWS adapter. KEDA does not fit that system's requirements, so the goal is to maintain this adapter and improve its reliability.

Original production baseline: `351e923`, deployed from Docker Hub as reported by the maintainer. The build and manifest milestone was merged through PR #1 into `master` at `2a27c66`; its tested GHCR image is `b9570c5ee2f42043ffca4344058c29730bf24a69`. The build milestone and repository maintenance follow-up are maintained on `master`. The merged `adapter-fixes` branch is retired. The `reliability-fixes` branch groups cache concurrency, fractional precision, and deletion handling into three independent commits before dependency migrations.

## Completed build and manifest milestone

- [x] Update the Docker builder to Go 1.27.2 and use one root Dockerfile with a minimal runtime, CA certificates, and license notices.
- [x] Build and publish `linux/amd64` and `linux/arm64` images together to GHCR using full commit tags; keep `latest` updates explicit.
- [x] Publish and test image `b9570c5ee2f42043ffca4344058c29730bf24a69`. The maintainer reported successful non-production EKS testing of this image and the migrated manifests.
- [x] Consolidate raw deployment files into CRD-only `deploy/crd.yaml` and adapter/RBAC `deploy/adapter.yaml`; remove duplicate deployment and legacy build/generation paths.
- [x] Pin the adapter namespace to `custom-metrics` in raw manifests and charts, preserving required `kube-system` references.
- [x] Lint and render both charts. The latest consolidated raw/rendered resources pass Kubernetes 1.37 schema validation; the earlier migration also passed 1.36 validation.
- [ ] Recheck the final consolidated, namespace-pinned manifests in non-production EKS before production rollout. This final cleanup has local validation but has not been reported as cluster-tested.

The Kubernetes and AWS libraries remain unchanged in this milestone. Their migrations and the remaining reliability fixes below are still pending.

## Repository maintenance follow-up

- [x] Remove obsolete Travis CI configuration, Docker Hub badges, missing sample links, and outdated installation output.
- [x] Default raw manifests and Helm to GHCR `latest` with `Always` pulls; allow users to select a commit or version tag. Publishing `latest` remains an explicit workflow action.
- [x] Correct entrypoint arguments, align default Helm service accounts, support architecture placement and configurable HTTPS ports, and retain the CRD on Helm uninstall.
- [x] Correct cross-account IAM documentation to separate target-role CloudWatch permissions, caller `sts:AssumeRole` permissions, and target-role trust.
- [ ] Publish a selected tested image as `latest` before relying on the new default; record its commit and digest.
- [ ] Validate these deployment/chart changes in non-production EKS, including custom service-account settings.

## Behavior to preserve

- [x] Return an error when CloudWatch returns no datapoints.
- [x] Propagate CloudWatch query failures as errors.
- [x] Keep a valid zero datapoint as zero.

Missing data must never be reported as a successful zero measurement. An error prevents the HPA from using that missing measurement to recommend a scale-down; it does not guarantee that all scaling pauses when the HPA uses multiple metrics.

## Prioritized fixes

1. [x] **Protect metric discovery from concurrent cache writes.** Listing now holds the cache read lock and starts with an empty slice, removing leading empty entries. Regression tests cover discovery updates/removals and concurrent list/get/update/remove operations; cache tests passed locally.
2. [x] **Preserve fractional CloudWatch values.** Decimal quantity parsing replaces integer truncation, preserving values such as `0.9` at Kubernetes quantity precision. Provider tests passed for fractional, sub-milli, large, integer, and zero measurements. Missing/nil datapoints and non-finite values return errors.
3. [ ] **Make region discovery reliable.** Prefer explicitly configured regions, use SDK metadata discovery with IMDSv2 support and bounded timeouts, and handle failures without panicking or treating error responses as region names. Verify configured-region operation without metadata access and metadata failure paths.
4. [ ] **Handle informer deletion tombstones.** Unwrap `DeletedFinalStateUnknown` and use deletion-aware key extraction. Verify that ordinary deletions and tombstones both remove the metric from the cache and discovery results.
5. [x] **Update remaining Kubernetes manifests.** Helm and standalone registrations use stable `v1` APIs and consistent permissive CRD schemas. The merged milestone pinned the published GHCR test image, which the maintainer reported testing successfully. The maintenance follow-up defaults to `latest` with an image-tag override. Final consolidation, namespace pinning, and chart follow-up changes still need a cluster recheck; see [the validation guide](docs/nonprod-validation.md).

## Regression coverage and release checks

Add focused tests alongside each fix rather than postponing coverage until the end.

- [x] Provider tests for missing results, empty datapoints, query errors, valid zero, and positive integer values.
- [x] Provider tests for fractional values, quantity precision, and invalid/nil measurements.
- [x] Cache tests for concurrent access and metric-name listing.
- [ ] Controller tests for ordinary deletions and deletion tombstones.
- [ ] Region discovery tests for configured regions and metadata failures.
- [x] Run `go test ./pkg/...` including the new provider regression tests; compile and verify Linux binaries for both architectures.
- [ ] Run the Linux CI race checks added with the cache concurrency fix. The image build now depends on `go test -race ./pkg/...`; CI execution remains pending.
- [x] Build and publish a new image with a distinct commit tag, preserving the deployed image as a rollback target.
- [x] Validate the baseline image and migrated manifests in non-production, as reported by the maintainer. Tests additionally cover missing-data, query-error, and genuine-zero behavior.
- [ ] Confirm both architecture placements and the final manifest cleanup in EKS before production rollout.

## Dependency modernization after reliability fixes

The build baseline now uses Go 1.27.2 and targets `linux/amd64` and `linux/arm64` through Buildx. Provider regression tests cover missing/nil results and datapoints, query failures, genuine zero, integers, fractional values, and non-finite measurements.

After the three reliability fixes, migrate AWS SDK v1 to v2 together with region discovery. Then migrate the metrics API server from the historical `kubernetes-incubator` module to `sigs.k8s.io/custom-metrics-apiserver` and update Kubernetes libraries together, last. The previously identified upstream `v1.36.0` release is a migration candidate; verify the release selection when that work starts. Kubernetes 1.36 and 1.37 are the reported EKS validation targets. Confirm runtime compatibility in both cluster versions and on both node architectures before rollout.

- [ ] Migrate Kubernetes libraries and the metrics API server together, preserving provider regression behavior.
- [ ] Migrate the historical `github.com/awslabs/k8s-cloudwatch-adapter` Go module path to this fork with all imports and generated-client tooling. Preserve the existing `metrics.aws` API group and copyright notices.
- [ ] Update code-generation scripts for the upgraded Kubernetes tooling, removing the old GOPATH output layout and verifying both generated API and client files.
- [ ] Migrate AWS SDK v1 to v2, checking configured regions, IRSA credentials, and cross-account role assumption.
- [ ] Publish and validate each migration on both architectures and reported cluster versions; record image references and rollback settings.

Dependency modernization starts from the tested build baseline above. Keep library migrations separate from the remaining reliability fixes where practical, and preserve the missing-data behavior throughout.

This roadmap records planned work; unchecked items are not implemented or validated yet.
