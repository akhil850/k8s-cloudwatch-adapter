# Maintenance roadmap

Maintain the CloudWatch adapter for existing deployments. AWS SDK v2 migration is complete and merged into `master`; Kubernetes modernization is next.

## Completed

- [x] Modernize container builds and publish `linux/amd64` and `linux/arm64` images to GHCR.
- [x] Migrate raw manifests and Helm charts to stable Kubernetes registration APIs; align namespace, service-account, and RBAC settings.
- [x] Fix concurrent metric discovery, decimal precision, and deletion tombstone handling.
- [x] Migrate AWS SDK v1 to v2, including cross-account role configuration.
- [x] Resolve regions from the spec, environment, and shared configuration before a bounded, IMDSv2-capable fallback.
- [x] Reject missing data, incomplete results, non-finite values, and invalid timestamps; preserve valid zeroes and source timestamps.
- [x] Add regression tests and pass Linux CI race tests and multi-architecture builds.

## SDK v2 checkpoint — 2026-11-03

Merged into `master` at `0a7f649`. Validated candidate: `83491d5f23f75a37c894be8c0650e77050130d5a` (image tag `83491d5`).

- **Automated:** Package tests, Linux race tests, and multi-architecture image build passed in [run 41592044874](https://github.com/akhil850/k8s-cloudwatch-adapter/actions/runs/41592044874). Local Linux binaries were also built and their architecture headers verified.
- **EKS:** Maintainer-provided logs and HPA status confirm startup, CloudWatch retrieval, decimal values, and `ScalingActive=True` / `ValidMetricFound` in staging.
- **Scope:** SDK migration is complete. Controlled scaling and the deployment-specific checks below remain unverified.

## Next: Kubernetes modernization

Work on a fresh branch from `master` and upgrade these components together:

- [ ] Replace `github.com/kubernetes-incubator/custom-metrics-apiserver` with `sigs.k8s.io/custom-metrics-apiserver`.
- [ ] Upgrade the `k8s.io/*` libraries from `v0.17.7` and migrate `klog` to `klog/v2`.
- [ ] Update code-generation scripts and regenerate API types, clients, informers, and listers.
- [ ] Preserve `metrics.aws/v1alpha1`, the external metrics API, and existing metric behavior.
- [ ] Pass regression/race tests, build both architectures, and validate on the target EKS versions before release. Manifest schema validation alone does not establish runtime compatibility.

## Rollout validation

Use the [non-production validation guide](docs/nonprod-validation.md). These checks are separate from completed SDK development:

- [ ] Record the deployed image digest and node architecture; retain the previous image for rollback.
- [ ] Compare the raw external metrics API timestamp with the selected CloudWatch datapoint.
- [ ] Exercise controlled HPA scale-up and scale-down with the candidate image.
- [ ] Exercise missing data, query failures, and recovery in an isolated EKS test.
- [ ] Validate cross-account role assumption and runtime placement on both architectures where used.
- [ ] Recheck the consolidated raw manifests and Helm installation paths, including custom service-account settings.
- [ ] Select a validated image for `latest` and record its digest.

## Later

- [ ] Rename the historical `github.com/awslabs/k8s-cloudwatch-adapter` module path to this fork, separately from the Kubernetes upgrade. Preserve the API group and attribution notices.

## Previous milestones

| Milestone | Reference |
| --- | --- |
| Original production baseline | `351e923` |
| Build and manifest modernization | PR #1, merged at `2a27c66`; tested image `b9570c5ee2f42043ffca4344058c29730bf24a69` |
| Cache, precision, and deletion fixes | Merged at `eefcce5` |
| AWS SDK v2 and measurement validation | Merged at `0a7f649`; validated candidate `83491d5` |
