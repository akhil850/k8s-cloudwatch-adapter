[![Build Status](https://github.com/akhil850/k8s-cloudwatch-adapter/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/akhil850/k8s-cloudwatch-adapter/actions/workflows/docker-publish.yml)
[![GitHub
release](https://img.shields.io/github/release/akhil850/k8s-cloudwatch-adapter/all.svg)](https://github.com/akhil850/k8s-cloudwatch-adapter/releases)
[![docker image
size](https://shields.beevelop.com/docker/image/image-size/akhil850/k8s-cloudwatch-adapter/latest.svg)](https://hub.docker.com/r/akhil850/k8s-cloudwatch-adapter)
[![image
layers](https://shields.beevelop.com/docker/image/layers/akhil850/k8s-cloudwatch-adapter/latest.svg)](https://hub.docker.com/r/akhil850/k8s-cloudwatch-adapter)
[![image
pulls](https://shields.beevelop.com/docker/pulls/akhil850/k8s-cloudwatch-adapter.svg)](https://hub.docker.com/r/akhil850/k8s-cloudwatch-adapter)

# Kubernetes Custom Metrics Adapter for Kubernetes

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

Requires a Kubernetes cluster with Metric Server deployed, Amazon EKS cluster is fine too.

Now deploy the adapter to your Kubernetes cluster:

```bash
$ kubectl apply -f https://raw.githubusercontent.com/akhil850/k8s-cloudwatch-adapter/master/deploy/adapter.yaml
namespace/custom-metrics created
clusterrolebinding.rbac.authorization.k8s.io/k8s-cloudwatch-adapter:system:auth-delegator created
rolebinding.rbac.authorization.k8s.io/k8s-cloudwatch-adapter-auth-reader created
deployment.apps/k8s-cloudwatch-adapter created
clusterrolebinding.rbac.authorization.k8s.io/k8s-cloudwatch-adapter-resource-reader created
serviceaccount/k8s-cloudwatch-adapter created
service/k8s-cloudwatch-adapter created
apiservice.apiregistration.k8s.io/v1beta1.external.metrics.k8s.io created
clusterrole.rbac.authorization.k8s.io/k8s-cloudwatch-adapter:external-metrics-reader created
clusterrole.rbac.authorization.k8s.io/k8s-cloudwatch-adapter-resource-reader created
clusterrolebinding.rbac.authorization.k8s.io/k8s-cloudwatch-adapter:external-metrics-reader created
customresourcedefinition.apiextensions.k8s.io/externalmetrics.metrics.aws created
clusterrole.rbac.authorization.k8s.io/k8s-cloudwatch-adapter:crd-metrics-reader created
clusterrolebinding.rbac.authorization.k8s.io/k8s-cloudwatch-adapter:crd-metrics-reader created
```

This creates a new namespace `custom-metrics` and deploys the necessary ClusterRole, Service Account,
Role Binding, along with the deployment of the adapter.

Alternatively the crd and adapter can be deployed using the Helm chart in the `/charts` directory:

```bash
$ helm install k8s-cloudwatch-adapter-crd ./charts/k8s-cloudwatch-adapter-crd
NAME: k8s-cloudwatch-adapter-crd
LAST DEPLOYED: Thu Sep 17 11:36:53 2020
NAMESPACE: default
STATUS: deployed
REVISION: 1
TEST SUITE: None
$ helm install k8s-cloudwatch-adapter ./charts/k8s-cloudwatch-adapter \
>   --namespace custom-metrics \
>   --create-namespace
NAME: k8s-cloudwatch-adapter
LAST DEPLOYED: Fri Aug 14 13:20:17 2020
NAMESPACE: custom-metrics
STATUS: deployed
REVISION: 1
TEST SUITE: None
```

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

## Deploying the sample application

There is a sample SQS application provided in this repository for you to test how the adapter works.
Refer to this [guide](samples/sqs/README.md).

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
**New Behavior:** The adapter now returns an error to the HPA if CloudWatch provides no data. This ensures that the HPA will "freeze" (pause scaling) at its current replica count, preventing aggressive and potentially disruptive scale-downs due to temporary data unavailability.

### Future Work

See the [maintenance roadmap](ROADMAP.md) for prioritized reliability fixes, regression coverage, and release checks for this fork.

Our immediate next steps involve:

- Updating the Go programming language version.
- Upgrading Kubernetes client libraries (`k8s.io/*`).
- Updating the AWS SDK for Go (`github.com/aws/aws-sdk-go`).
- Further code refactoring and bug fixes as identified.

## More docs

### Development images

The GitHub Actions workflow publishes images to `ghcr.io/akhil850/k8s-cloudwatch-adapter` on pushes to `master` and `adapter-fixes`. Each image is tagged with the full commit SHA; version-tag pushes also publish the version tag. Pull requests build without publishing. Manual runs can optionally publish and update `latest`.

Each tag contains both `linux/amd64` and `linux/arm64` images for mixed EKS fleets. The Dockerfile uses Go 1.27.2 to cross-compile for each target, runs the package tests before compilation, and packages the static binary with CA certificates and license notices in a minimal runtime image. Library modernization is tracked in [ROADMAP.md](ROADMAP.md); these build changes alone do not establish Kubernetes 1.36/1.37 runtime compatibility.

Use `ghcr.io/akhil850/k8s-cloudwatch-adapter:<full-commit-sha>` as the image in your EKS test deployment. The workflow authenticates with GitHub's built-in token and requires no Docker Hub secrets. New GHCR packages default to private: make the package public in its GitHub package settings for anonymous pulls, or configure an EKS `imagePullSecret` with a GitHub personal access token (classic) that has `read:packages` permission and access to the package.

- [Configuring cross account metric example](docs/cross-account.md)
- [ExternalMetric CRD schema](docs/schema.md)
- [Non-production manifest validation](docs/nonprod-validation.md)

## License

This library is licensed under the Apache 2.0 License.

## Issues

Report issues with this fork in [GitHub Issues](https://github.com/akhil850/k8s-cloudwatch-adapter/issues).
