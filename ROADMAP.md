# Maintenance roadmap

This fork supports an existing client deployment of the archived AWS adapter. KEDA does not fit that system's requirements, so the goal is to maintain this adapter and improve its reliability.

Baseline: master commit `351e923`. The deployed Docker Hub repository is `akhil850/k8s-cloudwatch-adapter`, with tags `latest` and `351e923` reported by the maintainer. Ongoing work starts on `adapter-fixes`.

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
5. [ ] **Update remaining Kubernetes manifests.** Migrate the Helm CRD, Helm APIService, and standalone CRD manifest to stable `v1` APIs with the required CRD schema and versions structure. Keep the combined deployment manifests consistent. Validate rendered charts and manifests against the client's supported Kubernetes versions.

## Regression coverage and release checks

Add focused tests alongside each fix rather than postponing coverage until the end.

- [ ] Provider tests for missing data, query errors, valid zero, and fractional values.
- [ ] Cache tests for concurrent access and metric-name listing.
- [ ] Controller tests for ordinary deletions and deletion tombstones.
- [ ] Region discovery tests for configured regions and metadata failures.
- [ ] Run the package suite and appropriate race checks. The existing `go test ./pkg/...` suite passed during the beta review, but the provider and metric cache had no tests.
- [ ] Build and review a new image with a distinct version or commit tag, preserving the deployed image as a rollback target.
- [ ] Validate the image in a test environment, including missing CloudWatch data and genuine zero measurements, before a production rollout.

## Later modernization

After the reliability fixes, evaluate Go, Kubernetes client, and AWS SDK upgrades against the client's compatibility requirements. Keep dependency modernization separate from behavior changes where practical.

This roadmap records planned work; unchecked items are not implemented or validated yet.
