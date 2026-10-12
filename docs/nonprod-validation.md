# Non-production manifest validation

Use a non-production cluster context. Raw manifests and Helm default to GHCR `latest` with `Always` pulls. For repeatable validation, select a commit tag and record it. The previously tested baseline is:

```text
ghcr.io/akhil850/k8s-cloudwatch-adapter:b9570c5ee2f42043ffca4344058c29730bf24a69
```

## Choose one installation method

The raw manifests are split by purpose: `deploy/crd.yaml` defines only the ExternalMetric CRD, and `deploy/adapter.yaml` installs the adapter and all its RBAC into `custom-metrics`. Alternatively, use the two Helm charts together, also in `custom-metrics`. Do not install both methods into the same cluster.

The adapter namespace is pinned in the manifests and charts. Helm requires `--namespace custom-metrics`; for another namespace, clone the repository and update the adapter namespace references and chart namespace checks. Keep the authentication RoleBinding and HPA service-account references in `kube-system`. Cluster-scoped resources, including the CRD and APIService, have no metadata namespace.

The CRD and APIService are cluster-wide. An existing external metrics adapter may already own them; check before applying:

```bash
kubectl config current-context
kubectl get apiservice v1beta1.external.metrics.k8s.io -o yaml
kubectl get crd externalmetrics.metrics.aws -o yaml
```

Back up existing definitions and deployments before replacing an installation. Do not delete the CRD to migrate it: deleting it also deletes its custom resources.

The CRD remains `metrics.aws/v1alpha1`; the external metrics API remains `external.metrics.k8s.io/v1beta1`. The registration objects now use `apiextensions.k8s.io/v1` and `apiregistration.k8s.io/v1`. The permissive structural schema preserves existing custom fields; stricter validation is deferred.

## Configure cluster-specific settings

Retain the existing service-account IAM role annotation and AWS environment configuration that your adapter needs. The role needs `cloudwatch:GetMetricData` and any permissions required by your cross-account configuration. Configure a registry pull secret if the GHCR package is private.

For Helm, use `serviceAccount.irsaIamRole`, `imagePullSecrets`, and your existing values overrides. Both charts default to service account `k8s-cloudwatch-adapter`, regardless of release name. For another account, set `serviceAccount.name` identically in both charts; use `serviceAccount.create=false` in the adapter chart for an existing account. Existing releases with generated account names should explicitly retain those names in both charts before upgrading. Override `image.tag` in Helm or edit the raw Deployment to use the selected test image. Updating `latest` requires an explicit publishing run and does not restart existing pods.

## Apply the raw manifests

```bash
kubectl apply --dry-run=server -f deploy/crd.yaml
kubectl apply -f deploy/crd.yaml
kubectl wait --for=condition=Established crd/externalmetrics.metrics.aws --timeout=60s
kubectl apply --dry-run=server -f deploy/adapter.yaml
kubectl apply -f deploy/adapter.yaml
kubectl rollout status deployment/k8s-cloudwatch-adapter -n custom-metrics --timeout=180s
kubectl get apiservice v1beta1.external.metrics.k8s.io
kubectl get --raw /apis/external.metrics.k8s.io/v1beta1
kubectl logs -n custom-metrics deployment/k8s-cloudwatch-adapter
```

Perform the adapter dry run after adding your IAM and registry settings. Applying `crd.yaml` alone does not install an adapter or grant its service account permissions; those resources belong to `adapter.yaml`.

## Helm alternative

```bash
helm upgrade --install k8s-cloudwatch-adapter-crd ./charts/k8s-cloudwatch-adapter-crd --namespace custom-metrics --create-namespace
helm upgrade --install k8s-cloudwatch-adapter ./charts/k8s-cloudwatch-adapter --namespace custom-metrics --values <your-cluster-values.yaml> --set-string image.tag=<selected-commit-tag>
```

The CRD chart uses `helm.sh/resource-policy: keep` to retain the CRD and its metric definitions on uninstall. A retained CRD requires explicit ownership handling before another release can manage it.

Helm does not automatically adopt resources previously installed with `kubectl`; resolve resource ownership before switching installation methods.

## Runtime checks

- Run the adapter on an `amd64` node and an `arm64` node in turn, using the same image tag and a Deployment node selector. For Helm, put `nodeSelector: {kubernetes.io/arch: amd64}` (or `arm64`) in your values file. Confirm startup and metric retrieval on each.
- Query an existing ExternalMetric by its namespace and resource name through `/apis/external.metrics.k8s.io/v1beta1/namespaces/<namespace>/<metric-name>` and compare its value with CloudWatch.
- Confirm that the HPA reads a normal measurement and a genuine zero correctly.
- In an isolated test setup, simulate a failed CloudWatch query and missing datapoints. Expect an API error and an unknown HPA metric, not a successful zero value.
- Restore access/data and confirm metric recovery. Other healthy HPA metrics can still trigger scale-up when this metric is unknown.
- Query an isolated test metric with a fractional value such as `0.9`. Expect an equivalent quantity such as `900m`, rather than `0`; compare numeric values rather than display formatting.
- Delete an isolated test ExternalMetric and verify it disappears from discovery and queries return an error. Recreate it and verify metric retrieval recovers. Do not delete the CRD.
- While updating test metric definitions, repeatedly request discovery and confirm the adapter remains healthy.

Keep the old image reference and configuration for rollback. Applying these manifests does not upgrade the adapter's Kubernetes or AWS libraries; runtime compatibility still needs testing on both reported cluster versions, 1.36 and 1.37.
