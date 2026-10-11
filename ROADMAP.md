# Maintenance roadmap

This fork supports an existing client deployment of the archived AWS adapter. KEDA does not fit that system's requirements, so the goal is to maintain this adapter and improve its reliability.

Baseline: master commit `351e923`. The deployed Docker Hub repository is `akhil850/k8s-cloudwatch-adapter`, with tags `latest` and `351e923` reported by the maintainer. Ongoing work starts on `adapter-fixes`.

## Completed build and manifest milestone

- [x] Update the Docker builder to Go 1.27.2 and use one root Dockerfile with a minimal runtime, CA certificates, and license notices.
- [x] Build and publish `linux/amd64` and `linux/arm64` images together to GHCR using full commit tags; keep `latest` updates explicit.
- [x] Publish and test image `b9570c5ee2f42043ffca4344058c29730bf24a69`. The maintainer reported successful non-production EKS testing of this image and the migrated manifests.
- [x] Consolidate raw deployment files into CRD-only `deploy/crd.yaml` and adapter/RBAC `deploy/adapter.yaml`; remove duplicate deployment and legacy build/generation paths.
- [x] Pin the adapter namespace to `custom-metrics` in raw manifests and charts, preserving required `kube-system` references.
- [x] Lint and render both charts. The latest consolidated raw/rendered resources pass Kubernetes 1.37 schema validation; the earlier migration also passed 1.36 validation.
- [ ] Recheck the final consolidated, namespace-pinned manifests in non-production EKS before production rollout. This final cleanup has local validation but has not been reported as cluster-tested.

The Kubernetes and AWS libraries remain unchanged in this milestone. Their migrations and the remaining reliability fixes below are still pending.

## Behavior to preserve

- [x] Return an error when CloudWatch returns no datapoints.
- [x] Propagate CloudWatch query failures as errors.
- [x] Keep a valid zero datapoint as zero.

Missing data must never be reported as a successful zero measurement. An error prevents the HPA from using that missing measurement to recommend a scale-down; it does not guarantee that all scaling pauses when the HPA uses multiple metrics.

## Prioritized fixes

1. [ ] **Protect metric discovery from concurrent cache writes.** Use the cache mutex when listing metric names. Also remove the leading empty entries caused by allocating a nonzero slice length before appending. Verify concurrent list/update/remove operations and run the race detector where supported.
2. [ ] **Preserve fractional CloudWatch values.** Replace the integer truncation in the provider with a quantity conversion that preserves useful decimal precision. Verify values such as `0.9`, integer values, and valid zero.
3. [ ] **Make region discovery reliable.** Prefer explicitly configured regions, use SDK metadata discovery with IMDSv2 support and bounded timeouts, and handle failures without panicking or treating error responses as region names. Verify configured-region operation without metadata access and metadata failure paths.
4. [ ] **Handle informer deletion tombstones.** Unwrap `DeletedFinalStateUnknown` and use deletion-aware key extraction. Verify that ordinary deletions and tombstones both remove the metric from the cache and discovery results.
5. [x] **Update remaining Kubernetes manifests.** Helm and standalone registrations now use stable `v1` APIs, consistent permissive CRD schemas, and the published GHCR test image. The maintainer reported successful testing of the migrated manifests. Final consolidation and namespace pinning still need a cluster recheck; see [the validation guide](docs/nonprod-validation.md).

## Regression coverage and release checks

Add focused tests alongside each fix rather than postponing coverage until the end.

- [x] Provider tests for missing results, empty datapoints, query errors, valid zero, and positive integer values.
- [ ] Provider tests for fractional values alongside the precision fix.
- [ ] Cache tests for concurrent access and metric-name listing.
- [ ] Controller tests for ordinary deletions and deletion tombstones.
- [ ] Region discovery tests for configured regions and metadata failures.
- [x] Run `go test ./pkg/...` including the new provider regression tests; compile and verify Linux binaries for both architectures.
- [ ] Run appropriate race checks with the cache concurrency fix.
- [x] Build and publish a new image with a distinct commit tag, preserving the deployed image as a rollback target.
- [x] Validate the baseline image and migrated manifests in non-production, as reported by the maintainer. Tests additionally cover missing-data, query-error, and genuine-zero behavior.
- [ ] Confirm both architecture placements and the final manifest cleanup in EKS before production rollout.

## Later modernization

The build baseline now uses Go 1.27.2 and targets `linux/amd64` and `linux/arm64` through Buildx. Provider regression tests cover missing results, empty datapoints, query failures, genuine zero, and a positive integer measurement. Fractional-value coverage remains part of the precision fix.

Next, migrate the metrics API server from the historical `kubernetes-incubator` module to `sigs.k8s.io/custom-metrics-apiserver` and update Kubernetes libraries together. The upstream `v1.36.0` release is the migration starting point; Kubernetes 1.36 and 1.37 are the reported EKS validation targets. Then migrate AWS SDK v1 to v2. Confirm runtime compatibility in both cluster versions and on both node architectures before rollout.

Dependency modernization starts from the tested build baseline above. Keep library migrations separate from the remaining reliability fixes where practical, and preserve the missing-data behavior throughout.

This roadmap records planned work; unchecked items are not implemented or validated yet.
