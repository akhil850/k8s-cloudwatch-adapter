# Maintenance roadmap

This fork supports an existing client deployment of the archived AWS adapter. KEDA does not fit that system's requirements, so the goal is to maintain this adapter and improve its reliability.

Original production baseline: `351e923`, deployed from Docker Hub as reported by the maintainer. The build and manifest milestone was merged through PR #1 into `master` at `2a27c66`; its tested GHCR image is `b9570c5ee2f42043ffca4344058c29730bf24a69`. The build milestone and repository maintenance follow-up are maintained on `master`. The merged `adapter-fixes` branch is retired. The `reliability-fixes` branch was merged into `master` at `eefcce5` (covering cache concurrency, fractional precision, and deletion handling). The `aws-sdk-v2` branch migrates the adapter from the deprecated AWS SDK v1 to AWS SDK v2.

## Completed build and manifest milestone

- [x] Update the Docker builder to Go 1.27.2 and use one root Dockerfile with a minimal runtime, CA certificates, and license notices.
- [x] Build and publish `linux/amd64` and `linux/arm64` images together to GHCR using full commit tags; keep `latest` updates explicit.
- [x] Publish and test image `b9570c5ee2f42043ffca4344058c29730bf24a69`. The maintainer reported successful non-production EKS testing of this image and the migrated manifests.
- [x] Consolidate raw deployment files into CRD-only `deploy/crd.yaml` and adapter/RBAC `deploy/adapter.yaml`; remove duplicate deployment and legacy build/generation paths.
- [x] Pin the adapter namespace to `custom-metrics` in raw manifests and charts, preserving required `kube-system` references.
- [x] Lint and render both charts. The latest consolidated raw/rendered resources pass Kubernetes 1.37 schema validation; the earlier migration also passed 1.36 validation.
- [ ] Recheck the final consolidated, namespace-pinned manifests in non-production EKS before production rollout. This final cleanup has local validation but has not been reported as cluster-tested.

The Kubernetes libraries remain on v0.17.7 pending modern metrics-apiserver migration. The reliability fixes have been merged into `master`; AWS SDK v2 modernization is implemented on `aws-sdk-v2`.

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
3. [x] **Make region discovery reliable.** Resolve the spec region before environment and shared configuration, using IMDS only as a fallback. The SDK metadata client supports IMDSv2 and a two-second timeout. Successful metadata lookups are cached; failures can recover on a later request. Region/configuration failures return query errors. Regression tests cover precedence, configured-region operation without metadata access, invalid metadata, timeout, and recovery.
4. [x] **Handle informer deletion tombstones.** Deletion-aware keys and tombstone unwrapping now queue deletions correctly. Regression tests passed for ordinary and tombstone deletions, invalid events, cache/discovery removal, and delayed deletions after recreation.
5. [x] **Update remaining Kubernetes manifests.** Helm and standalone registrations use stable `v1` APIs and consistent permissive CRD schemas. The merged milestone pinned the published GHCR test image, which the maintainer reported testing successfully. The maintenance follow-up defaults to `latest` with an image-tag override. Final consolidation, namespace pinning, and chart follow-up changes still need a cluster recheck; see [the validation guide](docs/nonprod-validation.md).

## Regression coverage and release checks

Add focused tests alongside each fix rather than postponing coverage until the end.

- [x] Provider tests for missing results, empty datapoints, query errors, valid zero, and positive integer values.
- [x] Provider tests for fractional values, quantity precision, and invalid/nil measurements.
- [x] Cache tests for concurrent access and metric-name listing.
- [x] Controller tests for ordinary deletions, tombstones, malformed events, and metric recreation.
- [x] Region discovery tests for configured regions, env vars, and metadata failures.
- [x] Provider regression tests rejecting incomplete result statuses and invalid timestamps, and preserving the selected datapoint's source timestamp.
- [x] Run `go test ./pkg/...` including provider and AWS query tests; compile and verify Linux binaries for both architectures.
- [x] Run `go test -race ./pkg/... -count=1` locally with Go 1.27.2 and GCC 15.2.0 on Windows/amd64. All package race tests passed on the reliability baseline.
- [ ] Trigger CI race-test gate on Linux builder for combined commits.
- [x] Build and publish a new image with a distinct commit tag, preserving the deployed image as a rollback target.
- [x] Validate the baseline image and migrated manifests in non-production, as reported by the maintainer. Tests additionally cover missing-data, query-error, and genuine-zero behavior.
- [ ] Confirm both architecture placements and the final manifest cleanup in EKS before production rollout.

## Reliability milestone validation

- [x] Three focused fixes with regression coverage: cache concurrency/listing, fractional values, and deletion handling.
- [x] Run `go test ./pkg/... -count=1` with all three changes; all package tests passed on Go 1.27.2.
- [x] Compile Linux `amd64` and `arm64` binaries with `-mod=readonly`; verify the ELF architecture headers.
- [x] Run all package race tests locally with Go 1.27.2 and GCC 15.2.0; no data races were reported.
- [x] Merge reliability milestone to `master` (commit `eefcce5`).
- [ ] Run the separate Linux CI race-test gate before publishing the combined image (merged with `[skip ci]`; can be run via manual workflow dispatch or tag).
- [ ] Validate combined reliability image candidate in non-production EKS.

## Dependency modernization

The build baseline uses Go 1.27 and targets `linux/amd64` and `linux/arm64` through Buildx. Provider regression tests cover missing/nil results and datapoints, query failures, genuine zero, integers, fractional values, and non-finite measurements.

- [x] **Migrate AWS SDK v1 to v2** (`aws-sdk-v2` branch, commit `7d25b39`):
  - Replaced deprecated monolithic `github.com/aws/aws-sdk-go` (EOL July 2025) with modular `github.com/aws/aws-sdk-go-v2` packages (`aws`, `config`, `service/cloudwatch`, `service/sts`, `credentials/stscreds`).
  - Switched query results from pointer slices (`[]*cloudwatch.MetricDataResult`) to value slices (`[]types.MetricDataResult`) and values from `[]*float64` to `[]float64`.
  - Configured client with `config.LoadDefaultConfig`, context-aware requests, STS assume-role provider, and explicit region precedence.
  - Replaced deprecated `ioutil.ReadAll` with `io.ReadAll` and added 2-second timeout context for EC2 instance metadata lookups.
  - Upgraded `go.mod` module directive to `go 1.24` and verified all `./pkg/...` tests pass.
- [x] Add dedicated unit tests for region resolution and IMDS timeout/failure fallback paths.
- [ ] Migrate Kubernetes libraries and the metrics API server together, preserving provider regression behavior:
  - Migrate from `github.com/kubernetes-incubator/custom-metrics-apiserver` (historical 2020 incubator release) to modern `sigs.k8s.io/custom-metrics-apiserver`.
  - Upgrade `k8s.io/*` dependencies (`client-go`, `apimachinery`, `metrics`, `component-base`, `code-generator`) from `v0.17.7` to match modern Kubernetes releases.
  - Upgrade `klog` from `v1.0.0` to `klog/v2`.
- [ ] Migrate the historical `github.com/awslabs/k8s-cloudwatch-adapter` Go module path to this fork with all imports and generated-client tooling. Preserve the existing `metrics.aws` API group and copyright notices.
- [ ] Update code-generation scripts for the upgraded Kubernetes tooling, removing the old GOPATH output layout and verifying both generated API and client files.
- [ ] Publish and validate each migration on both architectures and reported cluster versions; record image references and rollback settings.

Dependency modernization proceeds in phases: AWS SDK v2 is complete on `aws-sdk-v2`; Kubernetes library and metrics API server modernization follow next.

This roadmap records planned work; unchecked items are not implemented or validated yet.
