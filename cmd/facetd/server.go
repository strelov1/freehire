package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/strelov1/freehire/internal/search/facetidx"
)

const (
	maxQueryBody = 1 << 20   // /facets and /distinct: a filter and a facet list
	maxBulkBody  = 256 << 20 // /write and /rebuild/push: thousands of ~7KB documents
)

// Server is facetd's HTTP face over one facetidx.Store.
//
// There is no authentication: facetd binds to localhost by default and sits on the same
// host as the API, which is its only caller (and the reindex/drain workers beside it).
// Do not expose it on a public address.
//
// Rebuild rule: /rebuild/begin opens a fresh Builder off to the side. While a rebuild is
// open (begin until commit/abort, the Build included), every /write batch is applied to the
// live index as usual AND recorded; /rebuild/commit swaps the built index in and then
// re-applies the recorded batches on top, in order, so no write that arrived during the
// rebuild is lost. A recorded write can be older than the row the rebuild read from
// Postgres; the batches are full replaces and marks, so the next drain batch for that job
// corrects it, the same window the Meilisearch rebuild has.
type Server struct {
	snapshotPath string
	walPath      string // "" = no write-ahead log; see wal.go
	m            *metrics

	store atomic.Pointer[facetidx.Store] // nil until a snapshot loaded or a rebuild committed

	mu         sync.Mutex // guards the rebuild state below and serialises /write with commit's swap
	rebuilding bool       // begin..commit/abort; /write batches are recorded
	committing bool       // Build is running off-lock; push/begin/commit are refused
	builder    *facetidx.Builder
	pending    []facetidx.Batch
	wal        *os.File // guarded by mu, like the three below
	walOff     int64    // bytes of good records in the log
	walRecs    int
	walBroken  bool // a rollback failed: refuse writes rather than log behind garbage

	snapMu      sync.Mutex // one snapshot at a time
	snapGen     uint64
	snapWritten bool
	lastWrite   atomic.Int64 // unix nanos of the last applied write, 0 = never
}

// NewServer returns a not-ready server. snapshotPath "" disables snapshots.
func NewServer(snapshotPath string) *Server {
	s := &Server{snapshotPath: snapshotPath}
	if snapshotPath != "" {
		s.walPath = snapshotPath + ".wal"
	}
	s.m = newMetrics(s)
	return s
}

// LoadSnapshot reads the snapshot file, replays the write-ahead log over it and makes the
// server ready. A missing or corrupt snapshot, or a corrupt log, leaves it not-ready and
// returns the error; the log is still opened so a rebuild's writes are recorded.
func (s *Server) LoadSnapshot() error {
	if s.snapshotPath == "" {
		return errors.New("no snapshot path configured")
	}
	ix, err := readSnapshot(s.snapshotPath)
	st, werr := s.openWAL(ix)
	if err != nil {
		return err
	}
	if werr != nil {
		return werr
	}
	s.snapMu.Lock()
	s.snapGen, s.snapWritten = ix.Generation(), true
	s.snapMu.Unlock()
	s.store.Store(st)
	return nil
}

func readSnapshot(path string) (*facetidx.Index, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return facetidx.ReadSnapshot(f)
}

// Snapshot writes the current index atomically (temp file in the same directory, then
// rename). With onlyIfChanged it does nothing when the generation is already on disk.
func (s *Server) Snapshot(onlyIfChanged bool) error {
	st := s.store.Load()
	if st == nil || s.snapshotPath == "" {
		return nil
	}
	s.snapMu.Lock()
	defer s.snapMu.Unlock()
	s.mu.Lock() // index and log position must be read together: writes log+apply under mu
	ix, walFrom := st.Current(), s.walOff
	s.mu.Unlock()
	if onlyIfChanged && s.snapWritten && s.snapGen == ix.Generation() {
		return nil
	}
	start := time.Now()
	size, err := writeFileAtomic(s.snapshotPath, ix.WriteSnapshot)
	if err != nil {
		s.m.snapshotErrors.Inc()
		return err
	}
	if err := s.rotateWAL(walFrom); err != nil {
		s.m.walErrors.Inc()
		log.Printf("facetd: WAL rotation after snapshot: %v", err)
	}
	s.snapGen, s.snapWritten = ix.Generation(), true
	s.m.snapshotSeconds.Set(time.Since(start).Seconds())
	s.m.snapshotBytes.Set(float64(size))
	s.m.snapshotLastSuccess.SetToCurrentTime()
	return nil
}

func writeFileAtomic(path string, write func(io.Writer) error) (int64, error) {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return 0, err
	}
	tmp := f.Name()
	fail := func(err error) (int64, error) {
		_ = f.Close()
		_ = os.Remove(tmp)
		return 0, err
	}
	if err := write(f); err != nil {
		return fail(err)
	}
	if err := f.Sync(); err != nil {
		return fail(err)
	}
	st, err := f.Stat()
	if err != nil {
		return fail(err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return 0, err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return 0, err
	}
	return st.Size(), nil
}

// Handler is the HTTP API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /facets", s.instrument("facets", maxQueryBody, s.facets))
	mux.HandleFunc("POST /distinct", s.instrument("distinct", maxQueryBody, s.distinct))
	mux.HandleFunc("POST /write", s.instrument("write", maxBulkBody, s.write))
	mux.HandleFunc("POST /rebuild/begin", s.instrument("rebuild_begin", maxQueryBody, s.rebuildBegin))
	mux.HandleFunc("POST /rebuild/push", s.instrument("rebuild_push", maxBulkBody, s.rebuildPush))
	mux.HandleFunc("POST /rebuild/commit", s.instrument("rebuild_commit", maxQueryBody, s.rebuildCommit))
	mux.HandleFunc("POST /rebuild/abort", s.instrument("rebuild_abort", maxQueryBody, s.rebuildAbort))
	mux.HandleFunc("GET /ready", s.ready)
	mux.Handle("GET /metrics", promhttp.HandlerFor(s.m.reg, promhttp.HandlerOpts{}))
	return mux
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(c int) { w.code = c; w.ResponseWriter.WriteHeader(c) }

func (s *Server) instrument(name string, limit int64, h func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
		r.Body = http.MaxBytesReader(sw, r.Body, limit)
		h(sw, r)
		code := fmt.Sprint(sw.code)
		s.m.requests.WithLabelValues(name, code).Inc()
		s.m.latency.WithLabelValues(name, code).Observe(time.Since(start).Seconds())
	}
}

func reply(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, format string, a ...any) {
	reply(w, code, map[string]string{"error": fmt.Sprintf(format, a...)})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		fail(w, http.StatusBadRequest, "bad body: %v", err)
		return false
	}
	return true
}

func (s *Server) notReady(w http.ResponseWriter) {
	s.m.refused.WithLabelValues("not_ready").Inc()
	fail(w, http.StatusServiceUnavailable, "facetd: not ready (no index loaded)")
}

// ready answers 200 once an index is loaded.
func (s *Server) ready(w http.ResponseWriter, _ *http.Request) {
	if s.store.Load() == nil {
		fail(w, http.StatusServiceUnavailable, "not ready")
		return
	}
	reply(w, http.StatusOK, map[string]bool{"ready": true})
}

// queryErr maps an evaluator error to the wire contract: 422 for a filter or attribute the
// index cannot answer, 500 for anything else.
func (s *Server) queryErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, facetidx.ErrUnsupportedFilter):
		s.m.refused.WithLabelValues("unsupported").Inc()
		fail(w, http.StatusUnprocessableEntity, "%v", err)
	case errors.Is(err, facetidx.ErrUnknownFacet):
		s.m.refused.WithLabelValues("unknown_facet").Inc()
		fail(w, http.StatusUnprocessableEntity, "%v", err)
	default:
		fail(w, http.StatusInternalServerError, "%v", err)
	}
}

type queryReq struct {
	Filter json.RawMessage `json:"filter"`
	Facets []string        `json:"facets"`
	Attr   string          `json:"attr"`
}

// parseFilter turns the wire filter into the shapes facetidx accepts: null -> nil, a
// string, []string, [][]string, or []any of string / []string when the two are mixed.
func parseFilter(raw json.RawMessage) (any, error) {
	var v any
	if len(raw) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("%w: %v", facetidx.ErrUnsupportedFilter, err)
	}
	switch t := v.(type) {
	case nil, string:
		return t, nil
	case []any:
		strs, groups := 0, 0
		out := make([]any, len(t))
		for i, e := range t {
			switch e := e.(type) {
			case string:
				strs++
				out[i] = e
			case []any:
				g := make([]string, len(e))
				for j, x := range e {
					s, ok := x.(string)
					if !ok {
						return nil, fmt.Errorf("%w: group element %T", facetidx.ErrUnsupportedFilter, x)
					}
					g[j] = s
				}
				groups++
				out[i] = g
			default:
				return nil, fmt.Errorf("%w: element %T", facetidx.ErrUnsupportedFilter, e)
			}
		}
		switch {
		case groups == 0:
			ss := make([]string, len(out))
			for i, o := range out {
				ss[i] = o.(string)
			}
			return ss, nil
		case strs == 0:
			gg := make([][]string, len(out))
			for i, o := range out {
				gg[i] = o.([]string)
			}
			return gg, nil
		}
		return out, nil
	}
	return nil, fmt.Errorf("%w: filter shape %T", facetidx.ErrUnsupportedFilter, v)
}

func (s *Server) facets(w http.ResponseWriter, r *http.Request) {
	var req queryReq
	if !decode(w, r, &req) {
		return
	}
	st := s.store.Load()
	if st == nil {
		s.notReady(w)
		return
	}
	f, err := parseFilter(req.Filter)
	if err != nil {
		s.queryErr(w, err)
		return
	}
	ans, err := st.Current().Facets(f, req.Facets)
	if err != nil {
		s.queryErr(w, err)
		return
	}
	stats := make(map[string]map[string]float64, len(ans.Stats))
	for k, v := range ans.Stats {
		stats[k] = map[string]float64{"min": v.Min, "max": v.Max}
	}
	reply(w, http.StatusOK, map[string]any{"total": ans.Total, "distribution": ans.Distribution, "stats": stats})
}

func (s *Server) distinct(w http.ResponseWriter, r *http.Request) {
	var req queryReq
	if !decode(w, r, &req) {
		return
	}
	st := s.store.Load()
	if st == nil {
		s.notReady(w)
		return
	}
	f, err := parseFilter(req.Filter)
	if err != nil {
		s.queryErr(w, err)
		return
	}
	n, err := st.Current().Distinct(f, req.Attr)
	if err != nil {
		s.queryErr(w, err)
		return
	}
	reply(w, http.StatusOK, map[string]int64{"count": n})
}

type writeReq struct {
	Upserts   []json.RawMessage `json:"upserts"`
	Close     []int64           `json:"close"`
	Duplicate []int64           `json:"duplicate"`
	Delete    []int64           `json:"delete"`
}

func (s *Server) write(w http.ResponseWriter, r *http.Request) {
	var req writeReq
	if !decode(w, r, &req) {
		return
	}
	b := req.batch()
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.store.Load()
	if st == nil && !s.rebuilding {
		s.notReady(w)
		return
	}
	recLen, err := s.logWrite(req) // durable before it is applied or acknowledged
	if err != nil {
		fail(w, http.StatusInternalServerError, "facetd: write-ahead log: %v", err)
		return
	}
	var gen uint64
	if st != nil {
		if gen, err = st.Apply(b); err != nil {
			s.walUndo(recLen)
			fail(w, http.StatusBadRequest, "%v", err)
			return
		}
	}
	if s.rebuilding {
		s.pending = append(s.pending, b)
	}
	s.lastWrite.Store(time.Now().UnixNano())
	reply(w, http.StatusOK, map[string]uint64{"generation": gen})
}

func (s *Server) rebuildBegin(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.committing {
		fail(w, http.StatusConflict, "a rebuild commit is running")
		return
	}
	s.builder, s.pending, s.rebuilding = facetidx.NewBuilder(), nil, true
	reply(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) rebuildPush(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Docs []json.RawMessage `json:"docs"`
	}
	if !decode(w, r, &req) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.builder == nil {
		fail(w, http.StatusConflict, "no rebuild open")
		return
	}
	for _, d := range req.Docs {
		if err := s.builder.Add([]byte(d)); err != nil {
			fail(w, http.StatusBadRequest, "%v", err)
			return
		}
	}
	reply(w, http.StatusOK, map[string]int{"added": len(req.Docs)})
}

func (s *Server) rebuildAbort(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.committing {
		fail(w, http.StatusConflict, "a rebuild commit is running")
		return
	}
	s.builder, s.pending, s.rebuilding = nil, nil, false
	reply(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) rebuildCommit(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	if s.builder == nil || s.committing {
		s.mu.Unlock()
		fail(w, http.StatusConflict, "no rebuild open")
		return
	}
	b := s.builder
	s.builder, s.committing = nil, true
	s.mu.Unlock()

	ix := b.Build() // off-lock: /write keeps being applied and recorded meanwhile

	s.mu.Lock()
	var gen uint64
	if st := s.store.Load(); st != nil {
		gen = st.Replace(ix)
	} else {
		s.store.Store(facetidx.NewStore(ix))
		gen = ix.Generation()
	}
	st := s.store.Load()
	replayed := len(s.pending)
	for _, p := range s.pending {
		g, err := st.Apply(p)
		if err != nil {
			log.Printf("facetd: replaying a write recorded during the rebuild failed: %v", err)
			continue
		}
		gen = g
	}
	s.pending, s.rebuilding, s.committing = nil, false, false
	s.mu.Unlock()

	if err := s.Snapshot(false); err != nil {
		log.Printf("facetd: snapshot after rebuild: %v", err)
	}
	total, live := st.Current().Docs()
	log.Printf("facetd: rebuild committed generation=%d docs=%d live=%d replayed_batches=%d", gen, total, live, replayed)
	reply(w, http.StatusOK, map[string]uint64{"generation": gen, "documents": total})
}

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
}

func newMetrics(s *Server) *metrics {
	m := &metrics{
		reg:      prometheus.NewRegistry(),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "facetd_requests_total", Help: "Requests by endpoint and HTTP status."}, []string{"endpoint", "status"}),
		latency: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "facetd_request_duration_seconds", Help: "Request latency by endpoint and HTTP status.",
			Buckets: []float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 5, 30, 120}}, []string{"endpoint", "status"}),
		refused:             prometheus.NewCounterVec(prometheus.CounterOpts{Name: "facetd_refused_total", Help: "Requests refused, by reason: unsupported, unknown_facet, not_ready."}, []string{"reason"}),
		snapshotSeconds:     prometheus.NewGauge(prometheus.GaugeOpts{Name: "facetd_snapshot_duration_seconds", Help: "Duration of the last successful snapshot."}),
		snapshotBytes:       prometheus.NewGauge(prometheus.GaugeOpts{Name: "facetd_snapshot_bytes", Help: "Size of the last successful snapshot."}),
		snapshotLastSuccess: prometheus.NewGauge(prometheus.GaugeOpts{Name: "facetd_snapshot_last_success_timestamp_seconds", Help: "Unix time of the last successful snapshot."}),
		snapshotErrors:      prometheus.NewCounter(prometheus.CounterOpts{Name: "facetd_snapshot_errors_total", Help: "Failed snapshot writes."}),
		walBytes:            prometheus.NewGauge(prometheus.GaugeOpts{Name: "facetd_wal_bytes", Help: "Size of the write-ahead log."}),
		walRecords:          prometheus.NewGauge(prometheus.GaugeOpts{Name: "facetd_wal_records", Help: "Batches in the write-ahead log."}),
		walReplayed:         prometheus.NewCounter(prometheus.CounterOpts{Name: "facetd_wal_replayed_total", Help: "Log records replayed at start."}),
		walErrors:           prometheus.NewCounter(prometheus.CounterOpts{Name: "facetd_wal_errors_total", Help: "Write-ahead log failures: append, rollback, rotation, replay, corruption."}),
	}
	cur := func(f func(ix *facetidx.Index) float64) func() float64 {
		return func() float64 {
			if st := s.store.Load(); st != nil {
				return f(st.Current())
			}
			return 0
		}
	}
	m.reg.MustRegister(m.requests, m.latency, m.refused, m.snapshotSeconds, m.snapshotBytes, m.snapshotLastSuccess, m.snapshotErrors, m.walBytes, m.walRecords, m.walReplayed, m.walErrors,
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
