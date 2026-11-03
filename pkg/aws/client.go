package aws

import (
	"context"
	"fmt"
	"os"
	"sync"
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
	return &cloudwatchManager{}
}

type cloudwatchManager struct {
	regionMutex sync.Mutex
	localRegion string
}

func (c *cloudwatchManager) getLocalRegion() string {
	c.regionMutex.Lock()
	defer c.regionMutex.Unlock()
	// Cache successful metadata lookups; allow transient failures to recover.
	if c.localRegion == "" {
		c.localRegion = GetLocalRegion()
	}
	return c.localRegion
}

func (c *cloudwatchManager) getClient(role *string, region *string) (*cloudwatch.Client, error) {
	// Let the SDK resolve environment and shared-config regions before using IMDS.
	var loadOpts []func(*config.LoadOptions) error
	if region != nil && *region != "" {
		loadOpts = append(loadOpts, config.WithRegion(*region))
	}

	if os.Getenv("DEBUG") == "true" {
		loadOpts = append(loadOpts, config.WithClientLogMode(aws.LogRequestWithBody|aws.LogResponseWithBody))
	}

	cfg, err := config.LoadDefaultConfig(context.Background(), loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("unable to load AWS SDK config: %w", err)
	}
	if cfg.Region == "" {
		cfg.Region = c.getLocalRegion()
	}
	if cfg.Region == "" {
		return nil, fmt.Errorf("unable to resolve AWS region: set spec.region, AWS_REGION, or AWS_DEFAULT_REGION")
	}
	klog.Infof("using AWS Region: %s", cfg.Region)

	// Assume a cross-account role when one is specified.
	if role != nil && *role != "" {
		klog.Infof("using IAM role ARN: %s", *role)
		stsClient := sts.NewFromConfig(cfg)
		cfg.Credentials = aws.NewCredentialsCache(
			stscreds.NewAssumeRoleProvider(stsClient, *role),
		)
	}

	return cloudwatch.NewFromConfig(cfg), nil
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

	client, err := c.getClient(role, region)
	if err != nil {
		return nil, err
	}
	resp, err := client.GetMetricData(context.Background(), &cwQuery)
	if err != nil {
		klog.Errorf("CloudWatch GetMetricData error: %v", err)
		return nil, err
	}

	return resp.MetricDataResults, nil
}
