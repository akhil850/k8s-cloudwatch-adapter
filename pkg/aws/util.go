package aws

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/awslabs/k8s-cloudwatch-adapter/pkg/apis/metrics/v1alpha1"

	"k8s.io/klog"
)

// GetLocalRegion gets the region ID from the EC2 instance metadata (IMDSv1 fallback).
// Prefer setting the AWS_REGION / AWS_DEFAULT_REGION environment variable or the
// region field on the ExternalMetric spec; this path is used only when neither is set.
func GetLocalRegion() string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://169.254.169.254/latest/meta-data/placement/availability-zone/", nil)
	if err != nil {
		klog.Errorf("unable to build instance metadata request: %v", err)
		return ""
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		klog.Errorf("unable to get current region information: %v", err)
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		klog.Errorf("cannot read response from instance metadata: %v", err)
		return ""
	}

	if len(body) == 0 {
		klog.Errorf("empty response from instance metadata")
		return ""
	}

	// strip the last character from AZ to get region ID (e.g. "us-east-1a" → "us-east-1")
	return string(body[:len(body)-1])
}

func toCloudWatchQuery(externalMetric *v1alpha1.ExternalMetric) cloudwatch.GetMetricDataInput {
	queries := externalMetric.Spec.Queries

	cwMetricQueries := make([]types.MetricDataQuery, len(queries))
	for i, q := range queries {
		q := q
		mdq := types.MetricDataQuery{
			Id:         aws.String(q.ID),
			Label:      aws.String(q.Label),
			ReturnData: q.ReturnData,
		}

		if len(q.Expression) == 0 {
			dimensions := make([]types.Dimension, len(q.MetricStat.Metric.Dimensions))
			for j := range q.MetricStat.Metric.Dimensions {
				dimensions[j] = types.Dimension{
					Name:  aws.String(q.MetricStat.Metric.Dimensions[j].Name),
					Value: aws.String(q.MetricStat.Metric.Dimensions[j].Value),
				}
			}

			period := int32(q.MetricStat.Period)
			mdq.MetricStat = &types.MetricStat{
				Metric: &types.Metric{
					Dimensions: dimensions,
					MetricName: aws.String(q.MetricStat.Metric.MetricName),
					Namespace:  aws.String(q.MetricStat.Metric.Namespace),
				},
				Period: aws.Int32(period),
				Stat:   aws.String(q.MetricStat.Stat),
				Unit:   types.StandardUnit(q.MetricStat.Unit),
			}
		} else {
			mdq.Expression = aws.String(q.Expression)
		}

		cwMetricQueries[i] = mdq
	}

	return cloudwatch.GetMetricDataInput{
		MetricDataQueries: cwMetricQueries,
	}
}
