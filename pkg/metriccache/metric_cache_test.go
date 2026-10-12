package metriccache

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/awslabs/k8s-cloudwatch-adapter/pkg/apis/metrics/v1alpha1"
)

func TestListMetricNames(t *testing.T) {
	cache := NewMetricCache()
	assertNames := func(want []string) {
		t.Helper()
		got := cache.ListMetricNames()
		sort.Strings(got)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("metric names = %v, want %v", got, want)
		}
	}
	assertNames([]string{})
	cache.Update("ExternalMetric/default/first", "first", v1alpha1.ExternalMetric{})
	cache.Update("ExternalMetric/default/second", "second", v1alpha1.ExternalMetric{})
	assertNames([]string{"first", "second"})
	cache.Update("ExternalMetric/default/first", "renamed", v1alpha1.ExternalMetric{})
	assertNames([]string{"renamed", "second"})
	cache.Remove("ExternalMetric/default/first")
	assertNames([]string{"second"})
	cache.Remove("ExternalMetric/default/second")
	assertNames([]string{})
}

func TestConcurrentCacheOperations(t *testing.T) {
	cache := NewMetricCache()
	start := make(chan struct{})
	var workers sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		name := fmt.Sprintf("metric-%d", worker)
		workers.Add(2)
		go func() {
			defer workers.Done()
			<-start
			for i := 0; i < 1000; i++ {
				cache.Update(externalMetricKey("default", name), name, v1alpha1.ExternalMetric{})
				cache.Remove(externalMetricKey("default", name))
			}
		}()
		go func() {
			defer workers.Done()
			<-start
			for i := 0; i < 1000; i++ {
				cache.GetExternalMetric("default", name)
				for _, metricName := range cache.ListMetricNames() {
					if !strings.HasPrefix(metricName, "metric-") {
						t.Errorf("discovery returned unexpected name %q", metricName)
					}
				}
			}
		}()
	}
	close(start)
	workers.Wait()
	if names := cache.ListMetricNames(); len(names) != 0 {
		t.Fatalf("metrics remain after all removals: %v", names)
	}
}
