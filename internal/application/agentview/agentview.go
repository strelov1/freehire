// Package agentview records the job postings served to agents — over the hosted MCP
// servers, OJCP and the agent search endpoint — as a JSON Lines log, and aggregates
// that log into per-day unique views. It is the agent-side twin of viewlog: nginx's
// access log cannot see these, because every MCP call is a POST to one URL and a search
// answer is a list. See AGENTS.md.
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

// ViewerKey is the dedup identity: the account when the caller is signed in, otherwise
// the IP and User-Agent pair the nginx rollup dedups on.
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
// middleware. fasthttp's RequestCtx answers Value from its user values, and the adaptor
// passes that same context to the MCP SDK, so Record finds the scope in a tool handler
// without any parameter threading.
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
