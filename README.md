[![Build Status](https://github.com/akhil850/k8s-cloudwatch-adapter/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/akhil850/k8s-cloudwatch-adapter/actions/workflows/docker-publish.yml)

# Kubernetes CloudWatch Adapter

An independently maintained fork of the AWS CloudWatch adapter for Kubernetes, with reliability fixes for existing deployments.

The original implementation was developed by AWS and the contributors to [awslabs/k8s-cloudwatch-adapter](https://github.com/awslabs/k8s-cloudwatch-adapter). This fork is maintained independently and is not maintained or endorsed by AWS. Original copyright and attribution notices are preserved in [NOTICE](NOTICE), and the project remains licensed under [Apache License 2.0](LICENSE).

This README has been modified for the independently maintained fork.

An implementation of the Kubernetes [Custom Metrics API and External Metrics
API](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/#support-for-metrics-apis)
for AWS CloudWatch metrics.

This adapter allows you to scale your Kubernetes deployment using the [Horizontal Pod
Autoscaler](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/) (HPA) with
metrics from AWS CloudWatch.

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

Requires a Kubernetes cluster with API aggregation enabled and AWS credentials configured for the adapter. Configure the service-account IAM role and cross-account permissions for your deployment. Kubernetes and AWS library modernization is still pending; see [ROADMAP.md](ROADMAP.md).

Now deploy the adapter to your Kubernetes cluster:

Apply the CRD first, then the adapter and its RBAC. Raw manifests and Helm charts pin the adapter namespace to `custom-metrics`. To use another namespace, clone the repository and edit the adapter namespace references and chart namespace checks. The authentication RoleBinding and HPA service-account references in `kube-system` must remain there. Configure your IAM role and any private-registry pull secret before applying. See the [non-production validation guide](docs/nonprod-validation.md).

```bash
kubectl apply -f deploy/crd.yaml
kubectl wait --for=condition=Established crd/externalmetrics.metrics.aws --timeout=60s
kubectl apply -f deploy/adapter.yaml
kubectl rollout status deployment/k8s-cloudwatch-adapter -n custom-metrics
```

This creates a new namespace `custom-metrics` and deploys the necessary ClusterRole, Service Account,
Role Binding, along with the deployment of the adapter.

Alternatively the crd and adapter can be deployed using the Helm chart in the `/charts` directory:

```bash
helm upgrade --install k8s-cloudwatch-adapter-crd ./charts/k8s-cloudwatch-adapter-crd \
  --namespace custom-metrics --create-namespace
helm upgrade --install k8s-cloudwatch-adapter ./charts/k8s-cloudwatch-adapter \
  --namespace custom-metrics --values <your-cluster-values.yaml>
```

Both charts default to service account `k8s-cloudwatch-adapter`, independently of the release name. For another account, set `serviceAccount.name` identically in both charts. Set `serviceAccount.create=false` in the adapter chart when using an existing account. Existing releases with generated account names should explicitly retain those names in both charts before upgrading.

The CRD chart retains its CRD on uninstall to preserve metric definitions. Delete it manually only when removing those definitions is intended. Upgrade both charts together. Choose either raw manifests or Helm; Helm cannot automatically adopt resources previously installed with `kubectl`.

### Image selection

Raw manifests and Helm default to `ghcr.io/akhil850/k8s-cloudwatch-adapter:latest` with `imagePullPolicy: Always`. To pin an image, edit the raw Deployment reference or set `image.tag` in Helm:

```bash
helm upgrade --install k8s-cloudwatch-adapter ./charts/k8s-cloudwatch-adapter \
  --namespace custom-metrics --values <your-cluster-values.yaml> \
  --set-string image.tag=<full-commit-sha-or-version>
```

Updating a registry tag does not restart existing pods; use a rollout restart or a changed image reference to deploy another build. Publishing `latest` remains an explicit workflow action.

### Verifying the deployment

Next you can query the APIs to see if the adapter is deployed correctly by running:

```bash
$ kubectl get --raw "/apis/external.metrics.k8s.io/v1beta1" | jq .
{
  "kind": "APIResourceList",
  "apiVersion": "v1",
  "groupVersion": "external.metrics.k8s.io/v1beta1",
  "resources": [
  ]
}
```

## Project Analysis and Enhancements

This fork focuses on reliability improvements for existing deployments. See the [maintenance roadmap](ROADMAP.md) for the prioritized fixes and validation plans.

### Architecture Overview

The `k8s-cloudwatch-adapter` operates as a Kubernetes custom metrics adapter, enabling the Horizontal Pod Autoscaler (HPA) to scale deployments based on AWS CloudWatch metrics. Key components include:

- **`ExternalMetric` CRD:** Defines the CloudWatch metrics to be used for autoscaling.
- **Controller:** Watches `ExternalMetric` resources and maintains an in-memory `MetricCache`.
- **Provider:** Implements the Kubernetes External Metrics API, fetching metric definitions from the cache.
- **AWS Client:** Interacts with the AWS CloudWatch API to retrieve actual metric data.

### Critical Fix: HPA Scale-Down Prevention

A critical enhancement has been implemented in `pkg/provider/provider_external.go` to address HPA behavior during periods of missing CloudWatch data.
**Previous Behavior:** If CloudWatch returned no data for a requested metric, the adapter would report a value of `0` to the HPA, potentially causing an unintended scale-down of pods.
**New Behavior:** The adapter returns an error when CloudWatch provides no data or a query fails. The HPA cannot use that unavailable measurement to recommend a scale-down; other healthy metrics may still permit scale-up. A genuine zero remains zero.

### Future Work

See the [maintenance roadmap](ROADMAP.md) for prioritized reliability fixes, regression coverage, and release checks for this fork.

Our immediate next steps involve:

- Migrating Kubernetes client libraries and the metrics API server together.
- Migrating the AWS SDK for Go from v1 to v2.
- Further code refactoring and bug fixes as identified.

## More docs

### Development images

The GitHub Actions workflow runs package tests with the race detector before building and publishes images to `ghcr.io/akhil850/k8s-cloudwatch-adapter` on pushes to `master` and `reliability-fixes`. Each image is tagged with the full commit SHA; version-tag pushes also publish the version tag. Pull requests build without publishing. Manual runs can optionally publish and update `latest`. Intermediate commits that do not need an image use `[skip ci]`; the final reliability commit builds the combined candidate when pushed.

Each tag contains both `linux/amd64` and `linux/arm64` images for mixed EKS fleets. The Dockerfile uses Go 1.27.2 to cross-compile for each target, runs the package tests before compilation, and packages the static binary with CA certificates and license notices in a minimal runtime image. Library modernization is tracked in [ROADMAP.md](ROADMAP.md); these build changes alone do not establish Kubernetes 1.36/1.37 runtime compatibility.

Use `ghcr.io/akhil850/k8s-cloudwatch-adapter:<full-commit-sha>` as the image in your EKS test deployment. The workflow authenticates with GitHub's built-in token and requires no Docker Hub secrets. New GHCR packages default to private: make the package public in its GitHub package settings for anonymous pulls, or configure an EKS `imagePullSecret` with a GitHub personal access token (classic) that has `read:packages` permission and access to the package.

For local builds, `make docker` builds one image and `make docker-multiarch` builds and publishes both architectures. Set `TAG=<commit-or-version>` to choose a tag; the default is `latest`. Code-generation scripts under `hack/` support the generated Kubernetes clients. A new development branch must be added to the workflow's push filters or published with a manual run.

- [Configuring cross account metric example](docs/cross-account.md)
- [ExternalMetric CRD schema](docs/schema.md)
- [Non-production manifest validation](docs/nonprod-validation.md)

## License

This library is licensed under the Apache 2.0 License.

## Issues

Report issues with this fork in [GitHub Issues](https://github.com/akhil850/k8s-cloudwatch-adapter/issues).
