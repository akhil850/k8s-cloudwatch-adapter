[![Build Status](https://github.com/akhil850/k8s-cloudwatch-adapter/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/akhil850/k8s-cloudwatch-adapter/actions/workflows/docker-publish.yml)

# Kubernetes CloudWatch Adapter

An independently maintained fork of the AWS CloudWatch adapter for Kubernetes, with reliability fixes for existing deployments.

The original implementation was developed by AWS and the contributors to [awslabs/k8s-cloudwatch-adapter](https://github.com/awslabs/k8s-cloudwatch-adapter). This fork is maintained independently and is not maintained or endorsed by AWS. Original copyright and attribution notices are preserved in [NOTICE](NOTICE), and the project remains licensed under [Apache License 2.0](LICENSE).

This README has been modified for the independently maintained fork.

Scale Kubernetes workloads with the [Horizontal Pod Autoscaler](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/) (HPA) using AWS CloudWatch metrics through the External Metrics API.

## Prerequisites

This adapter requires the following permissions to access metric data from Amazon CloudWatch.

- cloudwatch:GetMetricData

You can create an IAM policy using this template, and attach it to the [Service Account Role](https://docs.aws.amazon.com/eks/latest/userguide/specify-service-account-role.html) if you are using
[IAM Roles for Service Accounts](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html).

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["cloudwatch:GetMetricData"],
      "Resource": "*"
    }
  ]
}
```

## Deploy

Requires Kubernetes API aggregation and AWS credentials with the permissions above.

Apply the CRD first, then the adapter and its RBAC. Raw manifests and Helm charts pin the adapter namespace to `custom-metrics`. To use another namespace, clone the repository and edit the adapter namespace references and chart namespace checks. The authentication RoleBinding and HPA service-account references in `kube-system` must remain there. Configure your IAM role and any private-registry pull secret before applying. See the [non-production validation guide](docs/nonprod-validation.md).

```bash
kubectl apply -f deploy/crd.yaml
kubectl wait --for=condition=Established crd/externalmetrics.metrics.aws --timeout=60s
kubectl apply -f deploy/adapter.yaml
kubectl rollout status deployment/k8s-cloudwatch-adapter -n custom-metrics
```

Alternatively, install both Helm charts:

```bash
helm upgrade --install k8s-cloudwatch-adapter-crd ./charts/k8s-cloudwatch-adapter-crd \
  --namespace custom-metrics --create-namespace
helm upgrade --install k8s-cloudwatch-adapter ./charts/k8s-cloudwatch-adapter \
  --namespace custom-metrics --values <your-cluster-values.yaml>
```

Both charts default to service account `k8s-cloudwatch-adapter`, independently of the release name. For another account, set `serviceAccount.name` identically in both charts. Set `serviceAccount.create=false` in the adapter chart when using an existing account. Existing releases with generated account names should explicitly retain those names in both charts before upgrading.

The CRD chart retains its CRD on uninstall to preserve metric definitions. Delete it manually only when removing those definitions is intended. Upgrade both charts together. Choose either raw manifests or Helm; Helm cannot automatically adopt resources previously installed with `kubectl`.

### Image selection

Raw manifests and Helm default to `ghcr.io/akhil850/k8s-cloudwatch-adapter:latest`, the current successful master build, with `imagePullPolicy: Always`. Use a short commit tag for a specific build, or pin the tested digest in your Deployment. To select a commit, edit the raw Deployment reference or set `image.tag` in Helm:

```bash
helm upgrade --install k8s-cloudwatch-adapter ./charts/k8s-cloudwatch-adapter \
  --namespace custom-metrics --values <your-cluster-values.yaml> \
  --set-string image.tag=<short-commit-sha>
```

Updating a registry tag does not restart existing pods; use a rollout restart or a changed image reference to deploy another build.

### Verifying the deployment

Check API discovery, then query a configured metric:

```bash
kubectl get --raw "/apis/external.metrics.k8s.io/v1beta1" | jq .
kubectl get --raw "/apis/external.metrics.k8s.io/v1beta1/namespaces/<namespace>/<metric-name>" | jq .
```

## Reliability

`ExternalMetric` resources define CloudWatch queries. The controller caches these definitions, and the provider retrieves measurements using AWS SDK v2.

- Missing data, failed queries, incomplete results, and invalid timestamps return errors rather than zero measurements.
- Valid zeroes and decimal values are preserved, along with the source datapoint timestamp.
- Cache access is synchronized, and deletion events remove stale metric definitions.

An unavailable measurement cannot recommend an HPA scale-down; other healthy metrics may still permit scale-up.

## Next milestone

Upgrade Kubernetes client libraries, the metrics API server, and generated clients together. AWS SDK v2 migration is complete and merged into `master`. See [ROADMAP.md](ROADMAP.md) for validation evidence and remaining rollout checks.

## Development images

Images at `ghcr.io/akhil850/k8s-cloudwatch-adapter` support **`linux/amd64` and `linux/arm64`**.

- `latest`: updated automatically by the current successful master build, using the same image digest; no extra build or promotion.
- `<short-sha>`: the seven-character commit tag for a specific build. Existing commit images are verified and reused.
- Branch images: run [Docker build and publish](https://github.com/akhil850/k8s-cloudwatch-adapter/actions/workflows/docker-publish.yml) manually with `publish=true`. These publish only the short SHA and leave `latest` unchanged.
- Local build: `make docker`. Build and publish both architectures: `make docker-multiarch`. Both default to the short commit tag.

Documentation-only changes keep the `CI` result and skip Go tests/image builds. See the [image build guide](docs/releases.md) for details.

Private GHCR packages require an `imagePullSecret` with package read access.

## Documentation

- [Configuring cross account metric example](docs/cross-account.md)
- [ExternalMetric CRD schema](docs/schema.md)
- [Non-production manifest validation](docs/nonprod-validation.md)

## License

This library is licensed under the Apache 2.0 License.

## Issues

Report issues with this fork in [GitHub Issues](https://github.com/akhil850/k8s-cloudwatch-adapter/issues).
