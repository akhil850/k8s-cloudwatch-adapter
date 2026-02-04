package provider

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

func (p *cloudwatchProvider) GetExternalMetric(namespace string, metricSelector labels.Selector, info provider.ExternalMetricInfo) (*external_metrics.ExternalMetricValueList, error) {
	// Note:
	//		metric name and namespace is used to lookup for the CRD which contains configuration to
	//		call cloudwatch if not found then ignored and label selector is parsed for all the metrics
	klog.V(0).Infof("Received request for namespace: %s, metric name: %s, metric selectors: %s", namespace, info.Metric, metricSelector.String())

	_, selectable := metricSelector.Requirements()
	if !selectable {
		return nil, errors.NewBadRequest("label is set to not selectable. this should not happen")
	}

	externalRequest, found := p.metricCache.GetExternalMetric(namespace, info.Metric)
	if !found {
		return nil, errors.NewBadRequest("no metric query found")
	}

	metricValue, err := p.cwManager.QueryCloudWatch(externalRequest)
	if err != nil {
		klog.Errorf("bad request: %v", err)
		return nil, errors.NewBadRequest(err.Error())
	}

	// --- OLD (Deleted) ---
	// if len(metricValue) == 0 || len(metricValue[0].Values) == 0 {
	//     quantity = *resource.NewMilliQuantity(0, resource.DecimalSI) // <--- DANGEROUS: Sets value to 0
	// } else {
	//     quantity = *resource.NewQuantity(int64(aws.Float64Value(metricValue[0].Values[0])), resource.DecimalSI)
	// }

	// [NEW SAFE LOGIC]
	// If CloudWatch returns empty data, we return an error. 
	// This forces the HPA to "freeze" (pause scaling) instead of scaling down to 0.
	if len(metricValue) == 0 || len(metricValue[0].Values) == 0 {
		klog.Warningf("CloudWatch returned no data for metric %s. Returning error to freeze HPA.", info.Metric)
		return nil, fmt.Errorf("no data points found for metric %s", info.Metric)
	}

	// Data exists, so we proceed safely
	quantity := *resource.NewQuantity(int64(aws.Float64Value(metricValue[0].Values[0])), resource.DecimalSI)

	externalMetricValue := external_metrics.ExternalMetricValue{
		MetricName: info.Metric,
		Value:      quantity,
		Timestamp:  metav1.Now(),
	}

	matchingMetrics := []external_metrics.ExternalMetricValue{externalMetricValue}

	return &external_metrics.ExternalMetricValueList{
		Items: matchingMetrics,
	}, nil
}

func (p *cloudwatchProvider) ListAllExternalMetrics() []provider.ExternalMetricInfo {
	p.valuesLock.RLock()
	defer p.valuesLock.RUnlock()

	// not implemented yet
	var externalMetricsInfo []provider.ExternalMetricInfo
	for _, name := range p.metricCache.ListMetricNames() {
		// only process if name is non-empty
		if name != "" {
			info := provider.ExternalMetricInfo{
				Metric: name,
			}
			externalMetricsInfo = append(externalMetricsInfo, info)
		}
	}
	return externalMetricsInfo
}
