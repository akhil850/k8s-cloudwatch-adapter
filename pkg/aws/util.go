package aws

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/awslabs/k8s-cloudwatch-adapter/pkg/apis/metrics/v1alpha1"

	"k8s.io/klog"
)

// GetLocalRegion retrieves the region from the EC2 identity document using the
// SDK's IMDSv2-capable client. Call only when no configured region is available.
func GetLocalRegion() string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := imds.New(imds.Options{}).GetRegion(ctx, &imds.GetRegionInput{})
	if err != nil {
		klog.Errorf("unable to get current region information: %v", err)
		return ""
	}
	return resp.Region
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
