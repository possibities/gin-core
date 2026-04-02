package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeWorkerQueue struct {
	depth int
}

func (f fakeWorkerQueue) QueueDepth() int {
	return f.depth
}

func TestRegistryExposesObservedMetrics(t *testing.T) {
	registry := New(fakeWorkerQueue{depth: 3})
	registry.ObserveHTTPRequest("GET", "/healthz", 200, 25*time.Millisecond)
	registry.ObserveCacheLookup(true)
	registry.ObserveCacheLookup(false)
	registry.ObserveDBQuery("query", "users", 12*time.Millisecond)
	registry.ObserveOutboxPublishAttempt("user.profile.updated.v1", "published")
	registry.ObserveOutboxDispatch("success", 18*time.Millisecond)
	registry.ObserveSchedulerTask("system:outbox_dispatch", "success", 15*time.Millisecond)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	registry.Handler().ServeHTTP(recorder, req)

	body := recorder.Body.String()
	checks := []string{
		"http_requests_total{method=\"GET\",path=\"/healthz\",status=\"200\"} 1",
		"http_request_duration_seconds_bucket{method=\"GET\",path=\"/healthz\",le=\"0.05\"} 1",
		"cache_hit_total 1",
		"cache_miss_total 1",
		"db_query_duration_seconds_bucket{operation=\"query\",table=\"users\",le=\"0.025\"} 1",
		"outbox_publish_attempts_total{result=\"published\",topic=\"user.profile.updated.v1\"} 1",
		"outbox_dispatch_runs_total{result=\"success\"} 1",
		"outbox_dispatch_duration_seconds_bucket{result=\"success\",le=\"0.025\"} 1",
		"scheduler_task_runs_total{result=\"success\",task=\"system:outbox_dispatch\"} 1",
		"scheduler_task_duration_seconds_bucket{result=\"success\",task=\"system:outbox_dispatch\",le=\"0.025\"} 1",
		"worker_queue_depth 3",
		"goroutines_total ",
	}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Fatalf("expected metrics output to contain %q", want)
		}
	}
}
