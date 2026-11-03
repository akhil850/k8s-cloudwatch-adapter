package controller

import (
	"testing"

	api "github.com/awslabs/k8s-cloudwatch-adapter/pkg/apis/metrics/v1alpha1"
	listers "github.com/awslabs/k8s-cloudwatch-adapter/pkg/client/listers/metrics/v1alpha1"
	"github.com/awslabs/k8s-cloudwatch-adapter/pkg/metriccache"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

func TestDeletionEventsRemoveCachedMetric(t *testing.T) {
	metric := newExternalMetric()
	metric.Kind = "" // Informer objects need not carry TypeMeta.
	tests := []struct {
		name string
		obj  interface{}
	}{
		{name: "ordinary deletion", obj: metric},
		{name: "tombstone", obj: cache.DeletedFinalStateUnknown{Key: "default/test", Obj: metric}},
		{name: "pointer tombstone", obj: &cache.DeletedFinalStateUnknown{Key: "default/test", Obj: metric}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
			metricCache := metriccache.NewMetricCache()
			metricCache.Update("ExternalMetric/default/test", "test", *metric)
			handler := NewHandler(listers.NewExternalMetricLister(indexer), metricCache)
			c := &Controller{
				metricQueue:   workqueue.NewNamedRateLimitingQueue(NoDelyRateLimiter(), "deletion-test"),
				metricHandler: &handler,
			}
			defer c.metricQueue.ShutDown()
			c.enqueueExternalMetric(tt.obj)
			if c.metricQueue.Len() != 1 {
				t.Fatal("deletion event was not queued")
			}
			if !c.processNextItem() {
				t.Fatal("controller unexpectedly stopped")
			}
			if _, found := metricCache.GetExternalMetric("default", "test"); found {
				t.Fatal("deleted metric remains queryable")
			}
			if names := metricCache.ListMetricNames(); len(names) != 0 {
				t.Fatalf("deleted metric remains in discovery: %v", names)
			}
		})
	}
}

func TestDelayedDeletionPreservesRecreatedMetric(t *testing.T) {
	oldMetric := newExternalMetric()
	newMetric := oldMetric.DeepCopy()
	newMetric.Spec.Queries[0].ID = "replacement"
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	if err := indexer.Add(newMetric); err != nil {
		t.Fatal(err)
	}
	metricCache := metriccache.NewMetricCache()
	metricCache.Update("ExternalMetric/default/test", "test", *oldMetric)
	handler := NewHandler(listers.NewExternalMetricLister(indexer), metricCache)
	c := &Controller{
		metricQueue:   workqueue.NewNamedRateLimitingQueue(NoDelyRateLimiter(), "recreation-test"),
		metricHandler: &handler,
	}
	defer c.metricQueue.ShutDown()
	c.enqueueExternalMetric(cache.DeletedFinalStateUnknown{Key: "default/test", Obj: oldMetric})
	if c.metricQueue.Len() != 1 {
		t.Fatal("tombstone was not queued")
	}
	c.processNextItem()
	got, found := metricCache.GetExternalMetric("default", "test")
	if !found || got.Spec.Queries[0].ID != "replacement" {
		t.Fatalf("recreated metric was not refreshed from the current informer state: %v", got)
	}
	if names := metricCache.ListMetricNames(); len(names) != 1 || names[0] != "test" {
		t.Fatalf("recreated metric missing from discovery: %v", names)
	}
}

func TestMalformedDeletionEventsAreIgnored(t *testing.T) {
	var nilTombstone *cache.DeletedFinalStateUnknown
	var nilMetric *api.ExternalMetric
	for _, obj := range []interface{}{
		cache.DeletedFinalStateUnknown{Key: "default/test", Obj: "wrong type"},
		cache.DeletedFinalStateUnknown{Key: "default/test"},
		cache.DeletedFinalStateUnknown{Key: "default/test", Obj: nilMetric},
		cache.DeletedFinalStateUnknown{Obj: newExternalMetric()},
		cache.DeletedFinalStateUnknown{Key: "invalid/key/extra", Obj: newExternalMetric()},
		nilTombstone,
	} {
		c := &Controller{metricQueue: workqueue.NewNamedRateLimitingQueue(NoDelyRateLimiter(), "invalid-deletion-test")}
		c.enqueueExternalMetric(obj)
		if c.metricQueue.Len() != 0 {
			t.Errorf("invalid event queued: %#v", obj)
		}
		c.metricQueue.ShutDown()
	}
}
