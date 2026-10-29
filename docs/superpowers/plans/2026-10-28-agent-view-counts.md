# Agent view counts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Count job postings served to agents over MCP, OJCP and `/api/v1/agent/jobs/search` by writing a JSON Lines event log that `cmd/rollup-views` aggregates into `job_daily_views.agent_uniques` and `jobs.view_count`.

**Architecture:** A new package `internal/application/agentview` owns the event, the per-day file writer, the request-scope context helpers and the parser/aggregator. Fiber middleware on each agent route stores a scope in fasthttp user values; call sites call `agentview.Record(ctx, kind, ids)`, a no-op without a scope. `cmd/rollup-views` gains a second pass over the log directory.

**Tech Stack:** Go, Fiber v2 (fasthttp), modelcontextprotocol/go-sdk v1.8.0, PostgreSQL, sqlc.

Spec: `docs/superpowers/specs/2026-10-28-agent-view-counts-design.md`.

## Global Constraints

- English only in code, comments, docs and commits.
- `AGENT_VIEW_LOG_DIR` unset ⇒ nothing is written and the rollup pass is skipped.
- One file per UTC day: `agent-views-YYYY-MM-DD.jsonl`. The rollup reads only days before today (UTC).
- Dedup key is `(viewer, job_id, day)`; `kind` and `surface` never split the count.
- `uniques` and `page_uniques` must not move from this feature; only `agent_uniques` and `jobs.view_count`.
- `GET /api/v1/jobs/<slug>`, `/api/v1/jobs/search`, `/api/v1/jobs` and the in-app assistant are never logged.
- Recording never blocks a response: a full buffer drops the event.
- Before each commit with Go: `gofmt -l .` prints nothing, `go vet ./...`, `go test ./...`. Before push: `go vet -tags=integration ./...`.
- Commit messages via `git commit -F <file>` (no backticks in `-m`).

---

### Task 1: The `agentview` package

**Files:**
- Create: `internal/application/agentview/agentview.go` (Event, Kind, viewer key, file naming, scope/context, Record)
- Create: `internal/application/agentview/writer.go` (Writer)
- Create: `internal/application/agentview/aggregate.go` (Aggregate, ListFiles)
- Create: `internal/application/agentview/agentview_test.go`, `writer_test.go`, `aggregate_test.go`
- Create: `internal/application/agentview/AGENTS.md`
- Modify: `internal/platform/arch/layering/blocks.go` — add `"agentview"` to the `application` package list next to `"viewlog"`.

**Interfaces — Produces:**
```go
type Kind string
const KindSearch Kind = "search"; const KindDetail Kind = "detail"
type Event struct { TS time.Time `json:"ts"`; Kind Kind `json:"kind"`; Surface string `json:"surface"`; Viewer string `json:"viewer"`; JobIDs []int64 `json:"job_ids"` }
type Sink interface { Write(Event) }
func ViewerKey(userID int64, ip, ua string) string          // "user:<id>" or "ip:<ip>|ua:<ua>"
func FileName(day time.Time) string                         // agent-views-YYYY-MM-DD.jsonl (UTC)
func Attach(set func(key, value any), sink Sink, surface, viewer string) // for c.Context().SetUserValue
func With(ctx context.Context, sink Sink, surface, viewer string) context.Context // tests / std contexts
func Record(ctx context.Context, kind Kind, ids []int64)
func NewWriter(dir string) *Writer   // nil when dir == ""; (*Writer)(nil) is a valid no-op Sink
func (w *Writer) Write(Event); func (w *Writer) Close(); func (w *Writer) Dropped() int64
type DayFile struct { Path string; Day time.Time }
func ListFiles(dir string, now time.Time) ([]DayFile, error) // days < today UTC, ascending; missing dir => nil, nil
type Stats struct { Lines, Bad int }
func Aggregate(r io.Reader) (map[time.Time]map[int64]int, Stats, error) // day -> job id -> uniques
```
Surfaces (constants in agentview.go): `SurfaceMCP = "mcp"`, `SurfaceMCPAccount = "mcp_account"`, `SurfaceOJCP = "ojcp"`, `SurfaceAgentAPI = "agent_api"`.

- [ ] **Step 1: Write the failing tests**

`agentview_test.go`:
```go
package agentview

import (
	"context"
	"testing"
	"time"
)

type sliceSink struct{ got []Event }

func (s *sliceSink) Write(e Event) { s.got = append(s.got, e) }

func TestRecordCarriesTheScopeAndTheIDs(t *testing.T) {
	sink := &sliceSink{}
	ctx := With(context.Background(), sink, SurfaceMCP, ViewerKey(0, "203.0.113.7", "Claude-User"))
	Record(ctx, KindDetail, []int64{42})
	if len(sink.got) != 1 {
		t.Fatalf("events = %d, want 1", len(sink.got))
	}
	e := sink.got[0]
	if e.Kind != KindDetail || e.Surface != SurfaceMCP || e.Viewer != "ip:203.0.113.7|ua:Claude-User" || len(e.JobIDs) != 1 || e.JobIDs[0] != 42 {
		t.Errorf("event = %+v", e)
	}
	if e.TS.IsZero() || e.TS.Location() != time.UTC {
		t.Errorf("ts = %v, want a UTC timestamp", e.TS)
	}
}

func TestRecordOfNoIDsWritesNothing(t *testing.T) {
	sink := &sliceSink{}
	Record(With(context.Background(), sink, SurfaceMCP, "v"), KindSearch, nil)
	if len(sink.got) != 0 {
		t.Errorf("events = %d, want 0 for an empty result", len(sink.got))
	}
}

func TestASignedInViewerIsKeyedByAccount(t *testing.T) {
	if got := ViewerKey(7, "203.0.113.7", "ua"); got != "user:7" {
		t.Errorf("ViewerKey = %q, want user:7", got)
	}
}

func TestAttachStoresAScopeRecordCanRead(t *testing.T) {
	// Fiber hands us SetUserValue, not a context; a map stands in for fasthttp's store.
	sink := &sliceSink{}
	store := map[any]any{}
	Attach(func(k, v any) { store[k] = v }, sink, SurfaceOJCP, "v")
	ctx := mapContext{context.Background(), store}
	Record(ctx, KindSearch, []int64{1, 2})
	if len(sink.got) != 1 || sink.got[0].Surface != SurfaceOJCP {
		t.Fatalf("events = %+v", sink.got)
	}
}

type mapContext struct {
	context.Context
	m map[any]any
}

func (c mapContext) Value(k any) any { return c.m[k] }

func TestFileNameIsTheEventsUTCDay(t *testing.T) {
	ts := time.Date(2026, 10, 28, 23, 30, 0, 0, time.FixedZone("x", -5*3600)) // 04:30 UTC on the 29th
	if got := FileName(ts); got != "agent-views-2026-10-29.jsonl" {
		t.Errorf("FileName = %q", got)
	}
}
```

`writer_test.go`:
```go
package agentview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAnUnsetDirectoryIsANoOpWriter(t *testing.T) {
	w := NewWriter("")
	if w != nil {
		t.Fatal("NewWriter(\"\") must be nil")
	}
	w.Write(Event{}) // must not panic
	w.Close()
}

func TestEachEventIsOneLineInItsDaysFile(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(dir)
	day1 := time.Date(2026, 10, 27, 23, 59, 0, 0, time.UTC)
	day2 := time.Date(2026, 10, 28, 0, 1, 0, 0, time.UTC)
	w.Write(Event{TS: day1, Kind: KindSearch, Surface: SurfaceMCP, Viewer: "v", JobIDs: []int64{1, 2}})
	w.Write(Event{TS: day2, Kind: KindDetail, Surface: SurfaceMCP, Viewer: "v", JobIDs: []int64{1}})
	w.Close()

	for name, want := range map[string]int{"agent-views-2026-10-27.jsonl": 1, "agent-views-2026-10-28.jsonl": 1} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if n := strings.Count(string(b), "\n"); n != want {
			t.Errorf("%s: %d lines, want %d", name, n, want)
		}
	}
}

func TestAFullBufferDropsInsteadOfBlocking(t *testing.T) {
	w := newWriter(t.TempDir(), 1)
	w.pause() // the drain goroutine holds off, so the buffer fills
	done := make(chan struct{})
	go func() {
		for range 10 {
			w.Write(Event{TS: time.Now().UTC(), JobIDs: []int64{1}})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Write blocked on a full buffer")
	}
	if w.Dropped() == 0 {
		t.Error("Dropped() = 0, want the overflow counted")
	}
	w.resume()
	w.Close()
}
```

`aggregate_test.go`:
```go
package agentview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestASearchThenADetailByOneViewerIsOneView(t *testing.T) {
	log := `{"ts":"2026-10-27T10:00:00Z","kind":"search","surface":"mcp","viewer":"a","job_ids":[1,2]}
{"ts":"2026-10-27T10:05:00Z","kind":"detail","surface":"mcp","viewer":"a","job_ids":[1]}
{"ts":"2026-10-27T11:00:00Z","kind":"detail","surface":"ojcp","viewer":"b","job_ids":[1]}
{"ts":"2026-10-28T00:00:01Z","kind":"detail","surface":"mcp","viewer":"a","job_ids":[1]}
`
	got, stats, err := Aggregate(strings.NewReader(log))
	if err != nil {
		t.Fatal(err)
	}
	d27 := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	d28 := d27.AddDate(0, 0, 1)
	if got[d27][1] != 2 || got[d27][2] != 1 || got[d28][1] != 1 {
		t.Errorf("counts = %v", got)
	}
	if stats.Lines != 4 || stats.Bad != 0 {
		t.Errorf("stats = %+v", stats)
	}
}

func TestAnUnreadableLineIsCountedAndSkipped(t *testing.T) {
	_, stats, err := Aggregate(strings.NewReader("not json\n{\"ts\":\"2026-10-27T10:00:00Z\",\"viewer\":\"a\",\"job_ids\":[1]}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Lines != 2 || stats.Bad != 1 {
		t.Errorf("stats = %+v", stats)
	}
}

func TestListFilesSkipsTodayAndStrangers(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"agent-views-2026-10-26.jsonl", "agent-views-2026-10-27.jsonl", "agent-views-2026-10-28.jsonl", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	files, err := ListFiles(dir, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || filepath.Base(files[0].Path) != "agent-views-2026-10-26.jsonl" || filepath.Base(files[1].Path) != "agent-views-2026-10-27.jsonl" {
		t.Errorf("files = %+v", files)
	}
}

func TestListFilesOfAMissingDirectoryIsEmpty(t *testing.T) {
	files, err := ListFiles(filepath.Join(t.TempDir(), "absent"), time.Now())
	if err != nil || len(files) != 0 {
		t.Errorf("files = %v, err = %v", files, err)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/application/agentview/`
Expected: FAIL — package does not compile (undefined: Record, NewWriter, …).

- [ ] **Step 3: Implement**

`agentview.go`:
```go
// Package agentview records the job postings served to agents — over the hosted MCP
// servers, OJCP and the agent search endpoint — as a JSON Lines log, and aggregates
// that log into per-day unique views. It is the agent-side twin of viewlog: nginx's
// access log cannot see these, because every MCP call is a POST to one URL and a
// search answer is a list. See AGENTS.md.
package agentview

import (
	"context"
	"strconv"
	"time"
)

// Kind says how a posting reached the agent. It is kept in the log for analysis; the
// rollup counts both kinds as one view.
type Kind string

const (
	KindSearch Kind = "search"
	KindDetail Kind = "detail"
)

// The surfaces an event can come from.
const (
	SurfaceMCP        = "mcp"
	SurfaceMCPAccount = "mcp_account"
	SurfaceOJCP       = "ojcp"
	SurfaceAgentAPI   = "agent_api"
)

// Event is one served response: the postings one agent call returned.
type Event struct {
	TS      time.Time `json:"ts"`
	Kind    Kind      `json:"kind"`
	Surface string    `json:"surface"`
	Viewer  string    `json:"viewer"`
	JobIDs  []int64   `json:"job_ids"`
}

// Sink receives events. *Writer is the production one.
type Sink interface{ Write(Event) }

// ViewerKey is the dedup identity: the account when the caller is signed in,
// otherwise the IP and User-Agent pair the nginx rollup dedups on.
func ViewerKey(userID int64, ip, ua string) string {
	if userID > 0 {
		return "user:" + strconv.FormatInt(userID, 10)
	}
	return "ip:" + ip + "|ua:" + ua
}

const filePrefix, fileSuffix, dayLayout = "agent-views-", ".jsonl", "2006-01-02"

// FileName is the log file an event at ts belongs to, by its UTC day.
func FileName(ts time.Time) string {
	return filePrefix + ts.UTC().Format(dayLayout) + fileSuffix
}

type scopeKey struct{}

type scope struct {
	sink    Sink
	surface string
	viewer  string
}

// Attach stores a request's scope through set — c.Context().SetUserValue in a Fiber
// middleware. fasthttp's RequestCtx answers Value from its user values, and the
// adaptor passes that same context to the MCP SDK, so Record finds the scope in a
// tool handler without any parameter threading.
func Attach(set func(key, value any), sink Sink, surface, viewer string) {
	set(scopeKey{}, scope{sink: sink, surface: surface, viewer: viewer})
}

// With returns ctx carrying a scope, for callers that hold a plain context.
func With(ctx context.Context, sink Sink, surface, viewer string) context.Context {
	return context.WithValue(ctx, scopeKey{}, scope{sink: sink, surface: surface, viewer: viewer})
}

// Record logs that ids were served to the agent ctx belongs to. Without a scope it does
// nothing: that is what keeps the in-app assistant and every human route out of the log.
func Record(ctx context.Context, kind Kind, ids []int64) {
	s, ok := ctx.Value(scopeKey{}).(scope)
	if !ok || s.sink == nil || len(ids) == 0 {
		return
	}
	s.sink.Write(Event{TS: time.Now().UTC(), Kind: kind, Surface: s.surface, Viewer: s.viewer, JobIDs: ids})
}
```

Note on `s.sink == nil`: a nil `*Writer` stored in the interface is non-nil as an interface, so `Writer.Write` must itself be nil-receiver safe (below).

`writer.go`:
```go
package agentview

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

// bufferSize bounds how many events may wait for the disk. A full buffer drops the
// newest event: a lost view costs a number, a blocked request costs a caller.
const bufferSize = 4096

// Writer appends events to one file per UTC day under dir. One goroutine owns the
// files; handlers only send on a channel. Each event is a single write(2) of one line
// to an O_APPEND file, which Linux appends atomically for regular files, so the two
// blue/green server instances can share a day's file without interleaving lines.
type Writer struct {
	dir     string
	ch      chan Event
	dropped atomic.Int64
	done    chan struct{}
	gate    sync.Mutex // held by pause() in tests to stall the drain
}

// NewWriter starts a writer under dir, or returns nil (a no-op Sink) when dir is empty.
func NewWriter(dir string) *Writer {
	if dir == "" {
		return nil
	}
	return newWriter(dir, bufferSize)
}

func newWriter(dir string, size int) *Writer {
	w := &Writer{dir: dir, ch: make(chan Event, size), done: make(chan struct{})}
	go w.run()
	return w
}

// Write queues e, or drops it when the buffer is full. Safe on a nil Writer.
func (w *Writer) Write(e Event) {
	if w == nil {
		return
	}
	select {
	case w.ch <- e:
	default:
		w.dropped.Add(1)
	}
}

// Dropped is how many events a full buffer has cost since start.
func (w *Writer) Dropped() int64 {
	if w == nil {
		return 0
	}
	return w.dropped.Load()
}

// Close flushes the queued events and stops the writer. Safe on a nil Writer.
func (w *Writer) Close() {
	if w == nil {
		return
	}
	close(w.ch)
	<-w.done
}

func (w *Writer) pause()  { w.gate.Lock() }
func (w *Writer) resume() { w.gate.Unlock() }

func (w *Writer) run() {
	defer close(w.done)
	var (
		f    *os.File
		name string
	)
	defer func() {
		if f != nil {
			f.Close()
		}
	}()
	for e := range w.ch {
		w.gate.Lock()
		w.gate.Unlock()

		line, err := json.Marshal(e)
		if err != nil {
			continue
		}
		if n := FileName(e.TS); n != name {
			if f != nil {
				f.Close()
				f = nil
			}
			name = n
			f, err = os.OpenFile(filepath.Join(w.dir, n), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
			if err != nil {
				log.Printf("agentview: open %s: %v", n, err)
				name = "" // retry the open on the next event
				continue
			}
		}
		if _, err := f.Write(append(line, '\n')); err != nil {
			log.Printf("agentview: write %s: %v", name, err)
		}
	}
}
```

`aggregate.go`:
```go
package agentview

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DayFile is one day's log file.
type DayFile struct {
	Path string
	Day  time.Time
}

// ListFiles returns the day files under dir for days before now's UTC day, oldest
// first. Today's file is still being written, so it is never offered. A missing dir is
// an empty answer: where the log is not configured there is nothing to roll up.
func ListFiles(dir string, now time.Time) ([]DayFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	today := now.UTC().Truncate(24 * time.Hour)
	var out []DayFile
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasPrefix(n, filePrefix) || !strings.HasSuffix(n, fileSuffix) {
			continue
		}
		day, err := time.Parse(dayLayout, strings.TrimSuffix(strings.TrimPrefix(n, filePrefix), fileSuffix))
		if err != nil || !day.Before(today) {
			continue
		}
		out = append(out, DayFile{Path: filepath.Join(dir, n), Day: day})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Day.Before(out[j].Day) })
	return out, nil
}

// Stats reports what Aggregate read: every non-empty line, and how many of them it
// could not use.
type Stats struct{ Lines, Bad int }

// Aggregate counts unique viewers per (UTC day, job). A viewer seeing a job in a search
// and then reading it is ONE view: kind and surface are not in the dedup key.
func Aggregate(r io.Reader) (map[time.Time]map[int64]int, Stats, error) {
	counts := map[time.Time]map[int64]int{}
	seen := map[string]struct{}{}
	var stats Stats
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		stats.Lines++
		var e Event
		if err := json.Unmarshal(line, &e); err != nil || e.TS.IsZero() || e.Viewer == "" {
			stats.Bad++
			continue
		}
		day := e.TS.UTC().Truncate(24 * time.Hour)
		for _, id := range e.JobIDs {
			key := e.Viewer + "\x00" + strconv.FormatInt(id, 10) + "\x00" + day.Format(dayLayout)
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			if counts[day] == nil {
				counts[day] = map[int64]int{}
			}
			counts[day][id]++
		}
	}
	return counts, stats, sc.Err()
}
```

`AGENTS.md` (short — the package's contract):
```markdown
# internal/application/agentview

Records the job postings served to AGENTS — the hosted MCP servers (`/mcp`,
`/mcp/account`), OJCP (`/ojcp/v1/*`, `/ojcp/mcp`) and `/api/v1/agent/jobs/search` — as
a JSON Lines log, and aggregates it into per-day unique views. The agent-side twin of
[viewlog](../viewlog/AGENTS.md): nginx's access log cannot see these, since every MCP
call is a POST to one URL and a search answer is a list.

- **Scope, not flags.** A Fiber middleware on each agent route calls `Attach` with
  `c.Context().SetUserValue`; fasthttp's `RequestCtx.Value` reads user values and the
  adaptor hands that context to the MCP SDK, so `Record` finds the scope inside a tool.
  `Record` without a scope does nothing — that is what keeps the in-app assistant,
  which shares `search_jobs`/`get_job` with `/mcp/account`, out of the log.
- **Never log what nginx already counts.** `GET /api/v1/jobs/<slug>` is in `uniques`
  already; recording it here would count one read twice.
- **One file per UTC day** (`agent-views-YYYY-MM-DD.jsonl`), named by each event's own
  timestamp. No logrotate; the rollup reads only finished days.
- **Two kinds, one count.** `kind` (`search`/`detail`) stays in the log for analysis;
  `Aggregate` dedups on `(viewer, job, day)` alone.
- **Dropping beats blocking.** A full buffer drops the event and counts it.
- `cmd/rollup-views` applies the counts to `job_daily_views.agent_uniques` and
  `jobs.view_count`; `uniques` and `page_uniques` never move from here.
```

`blocks.go`: in the `"application"` list, change `"userjob", "viewlog",` to `"userjob", "viewlog", "agentview",` (keep whatever ordering style the list uses — if it is alphabetical, insert `"agentview"` at the front).

- [ ] **Step 4: Run tests**

Run: `go test ./internal/application/agentview/ ./internal/platform/arch/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -l . ; go vet ./internal/application/agentview/
git add internal/application/agentview internal/platform/arch/layering/blocks.go
printf 'Add agentview: log job postings served to agents\n' > /tmp/m && git commit -F /tmp/m
```

---

### Task 2: Schema and query

**Files:**
- Create: `migrations/0212_agent_daily_views.sql`
- Modify: `internal/platform/db/queries/viewlog.sql` (new query; refresh the `ApplyDailyView` comment's claim that `view_count` reads from `uniques` alone)
- Generated: `internal/platform/db/*` via `make sqlc`

**Interfaces — Produces:** `db.ApplyDailyAgentViewParams{Day pgtype.Date; JobID int64; Delta int32}` and `(*db.Queries).ApplyDailyAgentView(ctx, []db.ApplyDailyAgentViewParams) *db.ApplyDailyAgentViewBatchResults`.

- [ ] **Step 1: Migration**

```sql
-- Agent views: postings served to agents over MCP, OJCP and /api/v1/agent/jobs/search,
-- counted by cmd/rollup-views from internal/application/agentview's log. Kept apart from
-- `uniques` (people and API reads, from nginx) and `page_uniques` (people only) because
-- jobs.view_count now sums both sources, and once summed nothing else could tell an
-- agent's view from a person's.
ALTER TABLE job_daily_views ADD COLUMN agent_uniques integer NOT NULL DEFAULT 0;
```

- [ ] **Step 2: Query** (append to `viewlog.sql`)

```sql
-- name: ApplyDailyAgentView :batchexec
-- Apply one (day, job) agent-view count additively: upsert job_daily_views.agent_uniques
-- and add the same delta to jobs.view_count. `uniques` is written as 0 on insert (it is
-- NOT NULL with no default) and never touched on conflict, so the nginx figures and
-- GET /api/v1/stats/catalog keep their meaning.
WITH ins AS (
    INSERT INTO job_daily_views (day, job_id, uniques, agent_uniques)
    VALUES (sqlc.arg('day'), sqlc.arg('job_id'), 0, sqlc.arg('delta'))
    ON CONFLICT (day, job_id)
        DO UPDATE SET agent_uniques = job_daily_views.agent_uniques + EXCLUDED.agent_uniques
)
UPDATE jobs SET view_count = view_count + sqlc.arg('delta')
WHERE id = sqlc.arg('job_id');
```

In the `ApplyDailyView` comment, replace "`jobs.view_count and GET /api/v1/stats/catalog both read from it`" with "`GET /api/v1/stats/catalog reads it, and jobs.view_count accrues it (plus ApplyDailyAgentView's agent views)`".

- [ ] **Step 3: Generate and lint**

Run: `make sqlc && pnpm check:sql && go build ./...`
Expected: success; `internal/platform/db/viewlog.sql.go` gains `ApplyDailyAgentView`.

- [ ] **Step 4: Commit**

```bash
git add migrations/0212_agent_daily_views.sql internal/platform/db
printf 'Add job_daily_views.agent_uniques\n' > /tmp/m && git commit -F /tmp/m
```

---

### Task 3: The rollup pass

**Files:**
- Create: `cmd/rollup-views/agent.go`
- Modify: `cmd/rollup-views/main.go` (run both passes; stop the early return when nginx has no files)
- Test: `cmd/rollup-views/agent_test.go` (unit) and `cmd/rollup-views/agent_integration_test.go` (`//go:build integration`)

**Interfaces — Consumes:** `agentview.ListFiles`, `agentview.Aggregate`, `db.ApplyDailyAgentView`, existing `IsViewLogFileProcessed`/`MarkViewLogFileProcessed`, `pgerr.IsDeadlock`.
**Produces:** `processAgent(ctx, pool, dir string, now time.Time) (nFiles, nViews int, err error)` and `buildAgentParams(map[time.Time]map[int64]int) ([]db.ApplyDailyAgentViewParams, int)`.

- [ ] **Step 1: Failing unit test** (`agent_test.go`)

```go
package main

import (
	"testing"
	"time"
)

func TestAgentParamsAreOrderedByJobThenDay(t *testing.T) {
	// The same lock-order rule buildParams documents: a random map order inside one long
	// transaction is a deadlock waiting for an ingest writer.
	d1 := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	d2 := d1.AddDate(0, 0, 1)
	params, total := buildAgentParams(map[time.Time]map[int64]int{
		d2: {9: 1, 3: 2},
		d1: {9: 4},
	})
	if total != 7 {
		t.Errorf("total = %d, want 7", total)
	}
	want := []struct {
		id  int64
		day time.Time
	}{{3, d2}, {9, d1}, {9, d2}}
	if len(params) != len(want) {
		t.Fatalf("params = %d, want %d", len(params), len(want))
	}
	for i, w := range want {
		if params[i].JobID != w.id || !params[i].Day.Time.Equal(w.day) {
			t.Errorf("params[%d] = %d/%v, want %d/%v", i, params[i].JobID, params[i].Day.Time, w.id, w.day)
		}
	}
}
```

- [ ] **Step 2: Run** — `go test ./cmd/rollup-views/` → FAIL (undefined: buildAgentParams).

- [ ] **Step 3: Implement `agent.go`**

```go
package main

import (
	"context"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/application/agentview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

// agentLogRetention is how long a day's agent-view file is kept after the run that
// applied it — the retention nginx's own logs have.
const agentLogRetention = 14 * 24 * time.Hour

// processAgent applies every finished, not-yet-applied day of the agent-view log, then
// removes the files past retention. The cursor is the same content-signature table the
// nginx pass uses, so a re-run applies nothing twice.
func processAgent(ctx context.Context, pool *pgxpool.Pool, dir string, now time.Time) (nFiles, nViews int, err error) {
	files, err := agentview.ListFiles(dir, now)
	if err != nil {
		return 0, 0, err
	}
	q := db.New(pool)
	for _, f := range files {
		counts, sig, err := aggregateAgentFile(f.Path)
		if err != nil {
			return nFiles, nViews, err
		}
		done, err := q.IsViewLogFileProcessed(ctx, sig)
		if err != nil {
			return nFiles, nViews, err
		}
		if done {
			continue
		}
		applied, err := applyAgentWithRetry(ctx, pool, q, f.Path, counts, sig)
		if err != nil {
			return nFiles, nViews, err
		}
		nFiles++
		nViews += applied
	}
	for _, f := range files {
		if now.Sub(f.Day) > agentLogRetention {
			if err := os.Remove(f.Path); err != nil {
				log.Printf("rollup-views: remove %s: %v", filepath.Base(f.Path), err)
			}
		}
	}
	return nFiles, nViews, nil
}

// aggregateAgentFile reads one day file and its cursor signature in one pass. A file
// with lines and not one readable line fails the run: a day that parses as nothing would
// otherwise count nothing in silence — the trap viewlog/AGENTS.md names for nginx.
func aggregateAgentFile(path string) (map[time.Time]map[int64]int, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	h := fnv.New64a()
	counts, stats, err := agentview.Aggregate(io.TeeReader(f, h))
	if err != nil {
		return nil, 0, err
	}
	if stats.Lines > 0 && stats.Bad == stats.Lines {
		return nil, 0, fmt.Errorf("%s: none of %d lines could be read", filepath.Base(path), stats.Lines)
	}
	if stats.Bad > 0 {
		log.Printf("rollup-views: %s: skipped %d unreadable line(s) of %d", filepath.Base(path), stats.Bad, stats.Lines)
	}
	return counts, int64(h.Sum64()), nil
}

// applyAgentWithRetry is applyWithRetry's twin for the agent pass; see its comment for
// why a deadlock is retried rather than fatal.
func applyAgentWithRetry(ctx context.Context, pool *pgxpool.Pool, q *db.Queries, path string, counts map[time.Time]map[int64]int, sig int64) (int, error) {
	var err error
	for attempt := 1; attempt <= deadlockRetries; attempt++ {
		var applied int
		applied, err = applyAgentFile(ctx, pool, q, path, counts, sig)
		if err == nil {
			return applied, nil
		}
		if !pgerr.IsDeadlock(err) {
			return 0, err
		}
		log.Printf("rollup-views: %s: deadlock on attempt %d/%d, retrying: %v", filepath.Base(path), attempt, deadlockRetries, err)
		delay := time.Duration(attempt) * 2 * time.Second
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(delay + time.Duration(rand.Int64N(int64(time.Second)))):
		}
	}
	return 0, fmt.Errorf("apply %s: still deadlocking after %d attempts: %w", filepath.Base(path), deadlockRetries, err)
}

// applyAgentFile applies one file's counts and marks it processed in one transaction.
func applyAgentFile(ctx context.Context, pool *pgxpool.Pool, q *db.Queries, path string, counts map[time.Time]map[int64]int, sig int64) (int, error) {
	params, total := buildAgentParams(counts)
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	qtx := q.WithTx(tx)
	if len(params) > 0 {
		br := qtx.ApplyDailyAgentView(ctx, params)
		var batchErr error
		br.Exec(func(_ int, e error) {
			if e != nil && batchErr == nil {
				batchErr = e
			}
		})
		if cerr := br.Close(); cerr != nil && batchErr == nil {
			batchErr = cerr
		}
		if batchErr != nil {
			return 0, batchErr
		}
	}
	if err := qtx.MarkViewLogFileProcessed(ctx, db.MarkViewLogFileProcessedParams{
		Signature: sig, Filename: filepath.Base(path),
	}); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return total, nil
}

// buildAgentParams flattens counts into the batch in ascending (job, day) order — the
// lock-order rule buildParams documents — and reports the views it carries.
func buildAgentParams(counts map[time.Time]map[int64]int) ([]db.ApplyDailyAgentViewParams, int) {
	var params []db.ApplyDailyAgentViewParams
	total := 0
	for day, perJob := range counts {
		for id, n := range perJob {
			params = append(params, db.ApplyDailyAgentViewParams{
				Day: pgtype.Date{Time: day, Valid: true}, JobID: id, Delta: int32(n),
			})
			total += n
		}
	}
	sort.Slice(params, func(i, j int) bool {
		if params[i].JobID != params[j].JobID {
			return params[i].JobID < params[j].JobID
		}
		return params[i].Day.Time.Before(params[j].Day.Time)
	})
	return params, total
}
```

If the generated `ApplyDailyAgentViewParams` field name is not `Delta` (check `internal/platform/db/viewlog.sql.go`), use the generated name.

- [ ] **Step 4: Wire into `run()` in `main.go`**

Replace the block from `files, err := viewlog.RotatedFiles(dir, base)` through the final `return 0` with:

```go
	files, err := viewlog.RotatedFiles(dir, base)
	if err != nil {
		log.Printf("list logs in %s: %v", dir, err)
		return 1
	}
	if !*backfill {
		files = uncompressed(files)
	}
	if len(files) == 0 {
		log.Printf("rollup-views: no rotated logs to process in %s", dir)
	} else {
		nFiles, nViews, err := process(ctx, pool, files)
		if err != nil {
			log.Printf("process: %v", err)
			return 1
		}
		log.Printf("rollup-views: processed %d file(s), applied %d view(s)", nFiles, nViews)
	}

	// The agent pass is independent of nginx: no rotated access log is not a reason to
	// skip the agent days. Unset, the log is not configured and there is nothing to read.
	if agentDir := os.Getenv("AGENT_VIEW_LOG_DIR"); agentDir != "" {
		nFiles, nViews, err := processAgent(ctx, pool, agentDir, time.Now())
		if err != nil {
			log.Printf("agent views: %v", err)
			return 1
		}
		log.Printf("rollup-views: agent views: processed %d file(s), applied %d view(s)", nFiles, nViews)
	}
	return 0
```

Update the package doc comment at the top of `main.go`: add a paragraph — "A second pass reads internal/application/agentview's day files under AGENT_VIEW_LOG_DIR (postings served to agents over MCP, OJCP and the agent search) and applies them to job_daily_views.agent_uniques and jobs.view_count."

- [ ] **Step 5: Integration test** (`agent_integration_test.go`, `//go:build integration`)

Find how other `cmd/*` integration tests get a migrated database (`grep -rln "go:build integration" cmd/ | head`, then reuse that helper — e.g. a `testdb`/`dbtest` package under `internal/platform`). The test:

1. Creates a job row (reuse the helper other tests use to insert a job) and reads its `view_count` (0).
2. Writes `agent-views-2026-10-26.jsonl` into `t.TempDir()` with two lines: viewer `a` search `[id]`, viewer `a` detail `[id]`, and one line viewer `b` detail `[id]`; also writes `agent-views-<today>.jsonl` with viewer `c` detail `[id]`.
3. Calls `processAgent(ctx, pool, dir, time.Now())` → `nFiles == 1`, `nViews == 2`.
4. Asserts `job_daily_views` for `(2026-10-26, id)` has `agent_uniques = 2`, `uniques = 0`, `page_uniques = 0`, and `jobs.view_count = 2` (today's file was not read).
5. Calls `processAgent` again → `nFiles == 0`, `view_count` still 2.

- [ ] **Step 6: Run**

Run: `go test ./cmd/rollup-views/ && go vet -tags=integration ./cmd/rollup-views/ && go test -tags=integration ./cmd/rollup-views/`
Expected: PASS (integration needs Docker).

- [ ] **Step 7: Commit**

```bash
gofmt -l . ; go vet ./...
git add cmd/rollup-views
printf 'Roll up agent views into job_daily_views and view_count\n' > /tmp/m && git commit -F /tmp/m
```

---

### Task 4: Record at the agent call sites

**Files:**
- Create: `internal/api/handler/agent_views.go` (the middleware)
- Modify: `internal/api/handler/handler.go` (`Config.AgentViews agentview.Sink`; pass it to the mounts)
- Modify: `internal/api/handler/mcpapp.go` (`register` adds middleware; `SearchJobs`, `JobDetail` record)
- Modify: `internal/api/handler/mcpapp_account.go` (middleware after `mcpAccountAuthenticate`)
- Modify: `internal/api/handler/ojcp.go` (`register` adds middleware on `/ojcp/v1/search`, `/ojcp/v1/jobs/:slug`, `/ojcp/mcp`; `SearchJobs`, `JobDetail` record)
- Modify: `internal/api/handler/assistant_tools.go` (`search_jobs`, `get_job` record — a no-op in the in-app assistant)
- Modify: `internal/api/handler/agent_search.go` and the route that mounts `AgentSearchJobs` (middleware + record)
- Modify: `internal/platform/config/config.go` (`AgentViewLogDir` from `AGENT_VIEW_LOG_DIR`), `cmd/server/main.go` (build the writer, pass it, `Close` on shutdown)
- Test: `internal/api/handler/agent_views_test.go`

**Interfaces — Consumes:** `agentview.Attach`, `agentview.Record`, `agentview.ViewerKey`, `agentview.Sink`, surface constants, `mcpapp.AccountUserIDHeader`, `auth.UserID(c)`.
**Produces:** `func agentViews(sink agentview.Sink, surface string) fiber.Handler`.

- [ ] **Step 1: Failing tests** (`agent_views_test.go`)

```go
package handler

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"

	"github.com/strelov1/freehire/internal/api/mcpapp"
	"github.com/strelov1/freehire/internal/application/agentview"
	"github.com/strelov1/freehire/internal/search/search"
)

type recordingSink struct {
	mu  sync.Mutex
	got []agentview.Event
}

func (s *recordingSink) Write(e agentview.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.got = append(s.got, e)
}

func TestAnMCPSearchThroughTheMountedRouteIsRecorded(t *testing.T) {
	// The scope crosses Fiber -> fasthttp user values -> adaptor -> MCP SDK -> tool. No
	// unit test on SearchJobs can prove that chain; this one goes through all of it.
	sink := &recordingSink{}
	fake := &fakeSearcher{res: search.Result{Hits: []search.JobDocument{{ID: 7}, {ID: 9}}}}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.All("/mcp", agentViews(sink, agentview.SurfaceMCP),
		adaptor.HTTPHandler(mcpapp.Handler(mcpappUnder(fake, nil, fakeOJCPStore{}), "")))

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search_jobs","arguments":{"query":"go"}}}`
	req := httptest.NewRequestWithContext(t.Context(), fiber.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("User-Agent", "Claude-User")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if len(sink.got) != 1 {
		t.Fatalf("events = %d, want 1", len(sink.got))
	}
	e := sink.got[0]
	if e.Kind != agentview.KindSearch || e.Surface != agentview.SurfaceMCP || len(e.JobIDs) != 2 || e.JobIDs[0] != 7 {
		t.Errorf("event = %+v", e)
	}
	if !strings.HasSuffix(e.Viewer, "|ua:Claude-User") {
		t.Errorf("viewer = %q, want the User-Agent in the key", e.Viewer)
	}
}
```

Before writing these, open `mcpapp_test.go`/the `fakeSearcher` definition and adjust field names (`res`, `search.Result`, `search.JobDocument{ID: …}`) to the real ones; a stateless `tools/call` without `initialize` is what `Stateless: true` accepts — if the SDK refuses it, send the `initialize` body from `TestTheMountedRouteSpeaksMCPOverHTTP` first in the same test.

Add one test per remaining surface in the same file, each asserting one event with the right `kind`/`surface`:
- OJCP REST detail: `app.Get("/ojcp/v1/jobs/:slug", agentViews(sink, agentview.SurfaceOJCP), h.OJCPJobDetail)` with an `ojcpHandlers` whose fake store returns a published job row (copy the store setup from `ojcp_test.go`'s detail test) → `KindDetail`, `JobIDs == [row.ID]`.
- Account viewer key: a request through `agentViews(sink, agentview.SurfaceMCPAccount)` carrying header `mcpapp.AccountUserIDHeader: 42` and a handler that calls `agentview.Record(c.Context(), agentview.KindDetail, []int64{1})` → `Viewer == "user:42"`.

- [ ] **Step 2: Run** — `go test ./internal/api/handler/ -run 'Agent|MCPSearchThroughTheMounted|SameSearchWithout'` → FAIL (undefined: agentViews).

- [ ] **Step 3: Middleware** (`agent_views.go`)

```go
package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/mcpapp"
	"github.com/strelov1/freehire/internal/application/agentview"
	"github.com/strelov1/freehire/internal/identity/auth"
)

// agentViews marks a request as an agent's, so the postings its handler serves are
// recorded (internal/application/agentview). The scope lives in fasthttp's user values,
// which RequestCtx.Value exposes as a context — the same context the MCP adaptor hands
// the SDK — so Fiber handlers and MCP tools reach it alike. A nil sink records nothing.
//
// The viewer is the account when one is known — the /mcp/account header that
// mcpAccountAuthenticate has already overwritten with a verified id, or a signed-in
// caller elsewhere — and otherwise the client IP (c.IP(), already resolved from the
// trusted proxy's X-Real-IP) with the User-Agent.
func agentViews(sink agentview.Sink, surface string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if sink == nil {
			return c.Next()
		}
		userID := auth.UserID(c)
		if userID == 0 {
			userID, _ = strconv.ParseInt(c.Get(mcpapp.AccountUserIDHeader), 10, 64)
		}
		agentview.Attach(c.Context().SetUserValue, sink, surface,
			agentview.ViewerKey(userID, c.IP(), c.Get(fiber.HeaderUserAgent)))
		return c.Next()
	}
}
```

Check: `auth.UserID(c)`'s real signature in `internal/identity/auth/middleware.go:168` — adapt the call if it differs. Check `c.Context().SetUserValue`'s signature matches `func(key, value any)`; if its key parameter is typed `any` but the method value does not convert, wrap it: `func(k, v any) { c.Context().SetUserValue(k, v) }`.

**Nil-interface trap:** `Config.AgentViews` is typed `agentview.Sink`. In `cmd/server`, assign it only when the writer is non-nil (`if w := agentview.NewWriter(dir); w != nil { cfg.AgentViews = w }`) so an unset directory leaves a true nil interface and the middleware short-circuits.

- [ ] **Step 4: Mount the middleware**

- `mcpapp.go` `register`: `api.All("/mcp", chatAgentLimiter(mw.public), agentViews(h.agentViews, agentview.SurfaceMCP), adaptor.HTTPHandler(...))`. Add field `agentViews agentview.Sink` to `mcpappHandlers`, set in `Register` (`mcpappH.agentViews = cfg.AgentViews`).
- `mcpapp_account.go` `mountMCPAccount`: add a `sink agentview.Sink` parameter; insert `agentViews(sink, agentview.SurfaceMCPAccount)` after `mcpAccountAuthenticate(...)`. Update the call in `handler.go:1065` to pass `cfg.AgentViews`.
- `ojcp.go` `register`: `views := agentViews(h.agentViews, agentview.SurfaceOJCP)` and add it after `limit` on `/ojcp/v1/search`, `/ojcp/v1/jobs/:slug` and `/ojcp/mcp` only (not employers/manifest/keys). Field + `Register` assignment as for mcpapp.
- `/agent/jobs/search`: find its route (`grep -n "AgentSearchJobs" internal/api/handler/*.go`) and add `agentViews(<sink>, agentview.SurfaceAgentAPI)` before the handler; give `searchHandlers` an `agentViews` field set in `Register`.

- [ ] **Step 5: Record at the call sites**

- `mcpapp.go` `SearchJobs`, after the hits loop:
  ```go
  	agentview.Record(ctx, agentview.KindSearch, hitIDs(res.Hits))
  ```
  `JobDetail`, after `publishedToAgents` passes and before returning:
  ```go
  	agentview.Record(ctx, agentview.KindDetail, []int64{row.ID})
  ```
- `ojcp.go` `SearchJobs` / `JobDetail`: the same two lines at the same points.
- `assistant_tools.go` `search_jobs` Run: `agentview.Record(ctx, agentview.KindSearch, hitIDs(res.Hits))` after the search succeeds; `get_job` Run: `agentview.Record(ctx, agentview.KindDetail, []int64{job.ID})` after `GetJobBySlug` succeeds.
- `agent_search.go` `AgentSearchJobs`: `agentview.Record(c.Context(), agentview.KindSearch, hitIDs(res.Hits))` after `runJobSearch` succeeds.
- Add the shared helper to `agent_views.go`:
  ```go
  // hitIDs is the job ids of a search answer, in order.
  func hitIDs(hits []search.JobDocument) []int64 {
  	ids := make([]int64, len(hits))
  	for i := range hits {
  		ids[i] = hits[i].ID
  	}
  	return ids
  }
  ```
  If `semanticSearch.search` returns a different hit type, adapt `hitIDs`' parameter to it.

`GET /api/v1/jobs/:slug` (`jobs.go`) is deliberately NOT touched: nginx already counts it.

- [ ] **Step 6: Server wiring**

`internal/platform/config/config.go`: add `AgentViewLogDir string` to the settings struct with a doc comment ("AgentViewLogDir is where the server writes internal/application/agentview's day files. Unset, nothing is written."), read from `os.Getenv("AGENT_VIEW_LOG_DIR")` beside the other plain reads (e.g. `MetricsPort`).

`cmd/server/main.go`, before `handler.Register`:
```go
	// Postings served to agents, logged for cmd/rollup-views (internal/application/agentview).
	var agentViews agentview.Sink
	if w := agentview.NewWriter(cfg.AgentViewLogDir); w != nil {
		defer w.Close()
		agentViews = w
	}
```
and `AgentViews: agentViews,` in the `handler.Config` literal. In `handler.Config` add:
```go
	// AgentViews receives the postings served to agents (internal/application/agentview).
	// Optional: nil records nothing.
	AgentViews agentview.Sink
```
Check that `defer w.Close()` runs on the server's real shutdown path (if `main` exits via `os.Exit` or a `run()` helper, place the `Close` where other shutdown cleanups live).

- [ ] **Step 7: Run everything**

Run: `go build ./... && go test ./internal/api/handler/ ./internal/application/... ./cmd/... && go vet ./... && go vet -tags=integration ./...`
Expected: PASS. The `vet -tags=integration` line matters here: `mountMCPAccount`'s signature changed and integration tests may call it.

- [ ] **Step 8: Commit**

```bash
gofmt -l . ; golangci-lint run ./internal/api/handler/... ./internal/application/agentview/... ./cmd/rollup-views/...
git add internal/api/handler internal/platform/config cmd/server
printf 'Record job postings served to agents\n\nMCP, OJCP and the agent search endpoint now log which postings they\nserved, for the view rollup. The in-app assistant and REST job detail\nare left out: the first is a person, the second nginx counts already.\n' > /tmp/m && git commit -F /tmp/m
```

---

### Task 5: Docs and the PR

**Files:**
- Modify: `AGENTS.md` (root) — in the worker list, extend the view-count story: add a bullet for `rollup-views` if none exists, or append to the **View counts** convention: "`job_daily_views.agent_uniques` counts postings served to agents (MCP, OJCP, `/agent/jobs/search`, via `internal/application/agentview`); `jobs.view_count` = `uniques` + `agent_uniques`. Never rank or publish a human figure on either." Add the module-table row: `| **Agent views** (postings served to agents, the day-file log, the scope that keeps people out) | [internal/application/agentview/AGENTS.md](internal/application/agentview/AGENTS.md) |`.
- Modify: `internal/application/viewlog/AGENTS.md` — one line under Shape: "`jobs.view_count` also accrues agent views from `internal/application/agentview`; this package's `uniques` does not."

- [ ] **Step 1:** Edit the two files as above.
- [ ] **Step 2:** `pnpm check:links` → all resolve.
- [ ] **Step 3:** Commit (`printf 'Document agent view counts\n' > /tmp/m && git commit -F /tmp/m`).
- [ ] **Step 4:** Run `/simplify` over the branch's Go changes, then push and open the PR (`gh pr create`), body ending with the deploy steps from the spec. Read CodeRabbit's review (`gh api repos/<owner>/<repo>/pulls/<n>/comments`, `gh pr view <n> --json reviews`), fix valid findings.

### Deploy (after merge, in `freehire-ops`)

1. Create `/var/lib/freehire/agent-views` (or the path ops conventions use), owned by the server's user, readable by the `rollup-views` unit's user.
2. Set `AGENT_VIEW_LOG_DIR` for both `freehire` server units and `freehire-rollup-views.service`; copy to the host, `systemctl daemon-reload`, run `release.sh freehire` (it migrates 0212 itself).
3. Next day after 02:30 UTC: `select day, sum(agent_uniques) from job_daily_views where day >= current_date - 2 group by 1;` shows yesterday > 0.
