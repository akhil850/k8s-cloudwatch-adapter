// Regression tests added for this independently maintained fork.
package provider

import (
	"errors"
	"math"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/awslabs/k8s-cloudwatch-adapter/pkg/apis/metrics/v1alpha1"
	"github.com/awslabs/k8s-cloudwatch-adapter/pkg/metriccache"
	apiprovider "github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
)

type stubCloudWatch struct {
	results []*cloudwatch.MetricDataResult
	err     error
}

func (s stubCloudWatch) QueryCloudWatch(v1alpha1.ExternalMetric) ([]*cloudwatch.MetricDataResult, error) {
	return s.results, s.err
}

func TestExternalMetricMissingDataAndZero(t *testing.T) {
	tests := []struct {
		name      string
		results   []*cloudwatch.MetricDataResult
		err       error
		wantError bool
		want      int64
	}{
		{name: "no results", wantError: true},
		{name: "nil result", results: []*cloudwatch.MetricDataResult{nil}, wantError: true},
		{name: "no datapoints", results: []*cloudwatch.MetricDataResult{{}}, wantError: true},
		{name: "nil datapoint", results: []*cloudwatch.MetricDataResult{{Values: []*float64{nil}}}, wantError: true},
		{name: "query failure", err: errors.New("CloudWatch unavailable"), wantError: true},
		{name: "genuine zero", results: []*cloudwatch.MetricDataResult{{Values: []*float64{aws.Float64(0)}}}},
		{name: "positive measurement", results: []*cloudwatch.MetricDataResult{{Values: []*float64{aws.Float64(42)}}}, want: 42},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := metriccache.NewMetricCache()
			cache.Update("ExternalMetric/default/test", "test", v1alpha1.ExternalMetric{})
			p := &cloudwatchProvider{metricCache: cache, cwManager: stubCloudWatch{results: tt.results, err: tt.err}}
			got, err := p.GetExternalMetric("default", labels.Everything(), apiprovider.ExternalMetricInfo{Metric: "test"})
			if tt.wantError {
				if err == nil || got != nil {
					t.Fatalf("missing data or failure must return an error and no measurement: got %v, err %v", got, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Items) != 1 || got.Items[0].Value.Value() != tt.want {
				t.Fatalf("got %v, want one measurement of %d", got, tt.want)
			}
		})
	}
}

func TestExternalMetricDecimalPrecision(t *testing.T) {
	tests := []struct {
		name  string
		value float64
		want  string
	}{
		{name: "fraction below one", value: 0.9, want: "900m"},
		{name: "fraction above one", value: 42.125, want: "42.125"},
		{name: "sub milli measurement", value: 0.0001, want: "100u"},
		{name: "nano precision", value: 1.234567891, want: "1.234567891"},
		{name: "minimum quantity precision", value: 0.0000000001, want: "1n"},
		{name: "negative fraction", value: -0.9, want: "-900m"},
		{name: "integer", value: 42, want: "42"},
		{name: "large measurement", value: 1000000000000.125, want: "1000000000000.125"},
		{name: "genuine zero", value: 0, want: "0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := metriccache.NewMetricCache()
			cache.Update("ExternalMetric/default/test", "test", v1alpha1.ExternalMetric{})
			p := &cloudwatchProvider{metricCache: cache, cwManager: stubCloudWatch{
				results: []*cloudwatch.MetricDataResult{{Values: []*float64{aws.Float64(tt.value)}}},
			}}
			got, err := p.GetExternalMetric("default", labels.Everything(), apiprovider.ExternalMetricInfo{Metric: "test"})
			if err != nil {
				t.Fatal(err)
			}
			want := resource.MustParse(tt.want)
			if len(got.Items) != 1 || got.Items[0].Value.Cmp(want) != 0 {
				t.Fatalf("got %v, want one measurement of %s", got, want.String())
			}
		})
	}
}

func TestExternalMetricRejectsNonFiniteValues(t *testing.T) {
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		cache := metriccache.NewMetricCache()
		cache.Update("ExternalMetric/default/test", "test", v1alpha1.ExternalMetric{})
		p := &cloudwatchProvider{metricCache: cache, cwManager: stubCloudWatch{
			results: []*cloudwatch.MetricDataResult{{Values: []*float64{aws.Float64(value)}}},
		}}
		got, err := p.GetExternalMetric("default", labels.Everything(), apiprovider.ExternalMetricInfo{Metric: "test"})
		if err == nil || got != nil {
			t.Fatalf("non-finite value %v must return an error, got %v, err %v", value, got, err)
		}
	}
}
