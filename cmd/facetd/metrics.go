package main

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"

	"github.com/strelov1/freehire/internal/search/facetidx"
)

// metrics is facetd's Prometheus surface, on its own registry.
type metrics struct {
	reg                 *prometheus.Registry
	requests            *prometheus.CounterVec
	latency             *prometheus.HistogramVec
	refused             *prometheus.CounterVec
	snapshotSeconds     prometheus.Gauge
	snapshotBytes       prometheus.Gauge
	snapshotLastSuccess prometheus.Gauge
	snapshotErrors      prometheus.Counter
	walBytes            prometheus.Gauge
	walRecords          prometheus.Gauge
	walReplayed         prometheus.Counter
	walErrors           prometheus.Counter
	writeBatch          prometheus.Histogram
	answerCache         *prometheus.CounterVec
}

func newMetrics(s *Server) *metrics {
	m := &metrics{
		reg:      prometheus.NewRegistry(),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "facetd_requests_total", Help: "Requests by endpoint and HTTP status."}, []string{"endpoint", "status"}),
		latency: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "facetd_request_duration_seconds", Help: "Request latency by endpoint and HTTP status.",
			Buckets: []float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 5, 30, 120}}, []string{"endpoint", "status"}),
		refused:             prometheus.NewCounterVec(prometheus.CounterOpts{Name: "facetd_refused_total", Help: "Requests refused, by reason: unsupported, unknown_facet, not_ready, abandoned (the caller gave up first)."}, []string{"reason"}),
		snapshotSeconds:     prometheus.NewGauge(prometheus.GaugeOpts{Name: "facetd_snapshot_duration_seconds", Help: "Duration of the last successful snapshot."}),
		snapshotBytes:       prometheus.NewGauge(prometheus.GaugeOpts{Name: "facetd_snapshot_bytes", Help: "Size of the last successful snapshot."}),
		snapshotLastSuccess: prometheus.NewGauge(prometheus.GaugeOpts{Name: "facetd_snapshot_last_success_timestamp_seconds", Help: "Unix time of the last successful snapshot."}),
		snapshotErrors:      prometheus.NewCounter(prometheus.CounterOpts{Name: "facetd_snapshot_errors_total", Help: "Failed snapshot writes."}),
		walBytes:            prometheus.NewGauge(prometheus.GaugeOpts{Name: "facetd_wal_bytes", Help: "Size of the write-ahead log."}),
		walRecords:          prometheus.NewGauge(prometheus.GaugeOpts{Name: "facetd_wal_records", Help: "Batches in the write-ahead log."}),
		walReplayed:         prometheus.NewCounter(prometheus.CounterOpts{Name: "facetd_wal_replayed_total", Help: "Log records replayed at start."}),
		walErrors:           prometheus.NewCounter(prometheus.CounterOpts{Name: "facetd_wal_errors_total", Help: "Write-ahead log failures: append, rollback, rotation, replay, corruption."}),
		writeBatch: prometheus.NewHistogram(prometheus.HistogramOpts{Name: "facetd_write_batch_jobs", Help: "Jobs per applied /write batch: upserts, closes, duplicates and deletes.",
			Buckets: []float64{1, 10, 50, 100, 500, 1000, 5000, 15000}}),
		answerCache: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "facetd_answer_cache_total", Help: "/facets lookups in the per-generation answer cache, by result: hit, miss."}, []string{"result"}),
	}
	cur := func(f func(ix *facetidx.Index) float64) func() float64 {
		return func() float64 {
			if st := s.store.Load(); st != nil {
				return f(st.Current())
			}
			return 0
		}
	}
	m.reg.MustRegister(m.requests, m.latency, m.refused, m.snapshotSeconds, m.snapshotBytes, m.snapshotLastSuccess, m.snapshotErrors, m.walBytes, m.walRecords, m.walReplayed, m.walErrors, m.writeBatch, m.answerCache,
		// go_* and process_*: heap, RSS and GC pauses (MemoryHigh also counts page cache).
		collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "facetd_documents", Help: "Documents in the index, closed and duplicate included."},
			cur(func(ix *facetidx.Index) float64 { t, _ := ix.Docs(); return float64(t) })),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "facetd_documents_live", Help: "Documents every answer counts."},
			cur(func(ix *facetidx.Index) float64 { _, l := ix.Docs(); return float64(l) })),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "facetd_generation", Help: "Current index generation."},
			cur(func(ix *facetidx.Index) float64 { return float64(ix.Generation()) })),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "facetd_ready", Help: "1 when an index is loaded."},
			func() float64 {
				if s.store.Load() != nil {
					return 1
				}
				return 0
			}),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "facetd_seconds_since_last_write", Help: "Seconds since the last applied /write batch; -1 before the first."},
			func() float64 {
				if t := s.lastWrite.Load(); t != 0 {
					return time.Since(time.Unix(0, t)).Seconds()
				}
				return -1
			}),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "facetd_rebuild_in_progress", Help: "1 while a rebuild is open."},
			func() float64 {
				s.mu.Lock()
				defer s.mu.Unlock()
				if s.rebuilding {
					return 1
				}
				return 0
			}),
	)
	return m
}
