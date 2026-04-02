package metrics

import (
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type WorkerQueue interface {
	QueueDepth() int
}

type Registry struct {
	registry               *prometheus.Registry
	httpRequests           *prometheus.CounterVec
	httpDuration           *prometheus.HistogramVec
	dbQueryDuration        *prometheus.HistogramVec
	outboxPublishAttempts  *prometheus.CounterVec
	outboxDispatchRuns     *prometheus.CounterVec
	outboxDispatchDuration *prometheus.HistogramVec
	schedulerTaskRuns      *prometheus.CounterVec
	schedulerTaskDuration  *prometheus.HistogramVec
	cacheHits              prometheus.Counter
	cacheMisses            prometheus.Counter
}

func New(worker WorkerQueue) *Registry {
	registry := prometheus.NewRegistry()
	httpRequests := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests by method, route, and status.",
		},
		[]string{"method", "path", "status"},
	)
	httpDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds by method and route.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
	dbQueryDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "db_query_duration_seconds",
			Help:    "Database query latency in seconds by operation and table.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation", "table"},
	)
	outboxPublishAttempts := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "outbox_publish_attempts_total",
			Help: "Total number of outbox publish attempts by topic and result.",
		},
		[]string{"topic", "result"},
	)
	outboxDispatchRuns := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "outbox_dispatch_runs_total",
			Help: "Total number of outbox dispatch runs by result.",
		},
		[]string{"result"},
	)
	outboxDispatchDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "outbox_dispatch_duration_seconds",
			Help:    "Outbox dispatch latency in seconds by result.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"result"},
	)
	schedulerTaskRuns := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "scheduler_task_runs_total",
			Help: "Total number of scheduler task runs by task and result.",
		},
		[]string{"task", "result"},
	)
	schedulerTaskDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "scheduler_task_duration_seconds",
			Help:    "Scheduler task latency in seconds by task and result.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"task", "result"},
	)
	cacheHits := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "cache_hit_total",
		Help: "Total number of cache hits.",
	})
	cacheMisses := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "cache_miss_total",
		Help: "Total number of cache misses.",
	})

	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		httpRequests,
		httpDuration,
		dbQueryDuration,
		outboxPublishAttempts,
		outboxDispatchRuns,
		outboxDispatchDuration,
		schedulerTaskRuns,
		schedulerTaskDuration,
		cacheHits,
		cacheMisses,
		prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Name: "worker_queue_depth",
				Help: "Current number of queued worker jobs.",
			},
			func() float64 {
				if worker == nil {
					return 0
				}
				return float64(worker.QueueDepth())
			},
		),
		prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Name: "goroutines_total",
				Help: "Current number of goroutines in the process.",
			},
			func() float64 {
				return float64(runtime.NumGoroutine())
			},
		),
	)

	return &Registry{
		registry:               registry,
		httpRequests:           httpRequests,
		httpDuration:           httpDuration,
		dbQueryDuration:        dbQueryDuration,
		outboxPublishAttempts:  outboxPublishAttempts,
		outboxDispatchRuns:     outboxDispatchRuns,
		outboxDispatchDuration: outboxDispatchDuration,
		schedulerTaskRuns:      schedulerTaskRuns,
		schedulerTaskDuration:  schedulerTaskDuration,
		cacheHits:              cacheHits,
		cacheMisses:            cacheMisses,
	}
}

func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{})
}

func (r *Registry) ObserveHTTPRequest(method, path string, status int, duration time.Duration) {
	if r == nil {
		return
	}
	if path == "" {
		path = "unknown"
	}

	r.httpRequests.WithLabelValues(method, path, strconv.Itoa(status)).Inc()
	r.httpDuration.WithLabelValues(method, path).Observe(duration.Seconds())
}

func (r *Registry) ObserveCacheLookup(hit bool) {
	if r == nil {
		return
	}
	if hit {
		r.cacheHits.Inc()
		return
	}
	r.cacheMisses.Inc()
}

func (r *Registry) ObserveDBQuery(operation, table string, duration time.Duration) {
	if r == nil {
		return
	}
	if operation == "" {
		operation = "unknown"
	}
	if table == "" {
		table = "unknown"
	}

	r.dbQueryDuration.WithLabelValues(operation, table).Observe(duration.Seconds())
}

func (r *Registry) ObserveOutboxPublishAttempt(topic, result string) {
	if r == nil {
		return
	}
	if topic == "" {
		topic = "unknown"
	}
	if result == "" {
		result = "unknown"
	}

	r.outboxPublishAttempts.WithLabelValues(topic, result).Inc()
}

func (r *Registry) ObserveOutboxDispatch(result string, duration time.Duration) {
	if r == nil {
		return
	}
	if result == "" {
		result = "unknown"
	}

	r.outboxDispatchRuns.WithLabelValues(result).Inc()
	r.outboxDispatchDuration.WithLabelValues(result).Observe(duration.Seconds())
}

func (r *Registry) ObserveSchedulerTask(task, result string, duration time.Duration) {
	if r == nil {
		return
	}
	if task == "" {
		task = "unknown"
	}
	if result == "" {
		result = "unknown"
	}

	r.schedulerTaskRuns.WithLabelValues(task, result).Inc()
	r.schedulerTaskDuration.WithLabelValues(task, result).Observe(duration.Seconds())
}
