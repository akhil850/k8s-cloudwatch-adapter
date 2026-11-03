package provider

import (
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
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

	// Missing data must not become a successful zero measurement.
	if len(metricValue) == 0 || metricValue[0].Values == nil || len(metricValue[0].Values) == 0 {
		klog.Warningf("CloudWatch returned no data for metric %s. Returning a metric error.", info.Metric)
		return nil, fmt.Errorf("no data points found for metric %s", info.Metric)
	}
	result := metricValue[0]
	if result.StatusCode != types.StatusCodeComplete {
		return nil, fmt.Errorf("CloudWatch result for metric %s is not complete: %s", info.Metric, result.StatusCode)
	}
	if len(result.Timestamps) != len(result.Values) || result.Timestamps[0].IsZero() {
		return nil, fmt.Errorf("missing or invalid CloudWatch timestamps for metric %s", info.Metric)
	}

	rawValue := result.Values[0]
	// Preserve decimals using Kubernetes quantity precision instead of truncating to an integer.
	quantity, err := resource.ParseQuantity(strconv.FormatFloat(float64(rawValue), 'f', -1, 64))
	if err != nil {
		return nil, fmt.Errorf("invalid CloudWatch value for metric %s: %w", info.Metric, err)
	}

	klog.Infof("Successfully fetched CloudWatch metric: %s (Namespace: %s). Value: %s", info.Metric, namespace, quantity.String())

	externalMetricValue := external_metrics.ExternalMetricValue{
		MetricName: info.Metric,
		Value:      quantity,
		Timestamp:  metav1.NewTime(result.Timestamps[0]),
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
