package aws

import (
	"context"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/awslabs/k8s-cloudwatch-adapter/pkg/apis/metrics/v1alpha1"
	"k8s.io/klog"
)

func NewCloudWatchManager() CloudWatchManager {
	return &cloudwatchManager{
		localRegion: GetLocalRegion(),
	}
}

type cloudwatchManager struct {
	localRegion string
}

func (c *cloudwatchManager) getClient(role *string, region *string) *cloudwatch.Client {
	// Resolve region: explicit spec field → environment variable → IMDS fallback.
	resolvedRegion := c.localRegion
	if region != nil && *region != "" {
		resolvedRegion = *region
	}
	klog.Infof("using AWS Region: %s", resolvedRegion)

	loadOpts := []func(*config.LoadOptions) error{
		config.WithRegion(resolvedRegion),
	}

	if os.Getenv("DEBUG") == "true" {
		loadOpts = append(loadOpts, config.WithClientLogMode(aws.LogRequestWithBody|aws.LogResponseWithBody))
	}

	cfg, err := config.LoadDefaultConfig(context.Background(), loadOpts...)
	if err != nil {
		klog.Fatalf("unable to load AWS SDK config: %v", err)
	}

	// Assume a cross-account role when one is specified.
	if role != nil && *role != "" {
		klog.Infof("using IAM role ARN: %s", *role)
		stsClient := sts.NewFromConfig(cfg)
		cfg.Credentials = aws.NewCredentialsCache(
			stscreds.NewAssumeRoleProvider(stsClient, *role),
		)
	}

	return cloudwatch.NewFromConfig(cfg)
}

func (c *cloudwatchManager) QueryCloudWatch(request v1alpha1.ExternalMetric) ([]types.MetricDataResult, error) {
	role := request.Spec.RoleARN
	region := request.Spec.Region
	cwQuery := toCloudWatchQuery(&request)

	now := time.Now()
	endTime := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), 0, 0, now.Location())
	// CloudWatch metrics have latency; grab a 5-minute window and return the latest value.
	startTime := endTime.Add(-5 * time.Minute)

	cwQuery.EndTime = &endTime
	cwQuery.StartTime = &startTime
	cwQuery.ScanBy = types.ScanByTimestampDescending

	resp, err := c.getClient(role, region).GetMetricData(context.Background(), &cwQuery)
	if err != nil {
		klog.Errorf("CloudWatch GetMetricData error: %v", err)
		return nil, err
	}

	return resp.MetricDataResults, nil
}
