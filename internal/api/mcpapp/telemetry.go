package mcpapp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var mcpClientCalls = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "freehire_mcp_client_calls_total", Help: "MCP calls by surface and recognized client family, including rejected calls.",
}, []string{"surface", "client"})

var mcpResponseBytes = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name: "freehire_mcp_response_bytes", Help: "Serialized MCP tool result size, excluding the JSON-RPC envelope; no payloads retained.",
	Buckets: []float64{1024, 4096, 16384, 65536, 262144, 1048576, 2097152, 4194304, 8388608, 16777216, 33554432, 67108864},
}, []string{"surface", "tool"})

var mcpSearchQuality = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "freehire_mcp_search_quality_total", Help: "Successful job search calls by surface and quality signal (one call may have multiple signals).",
}, []string{"surface", "signal"})

var mcpProblemReports = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "freehire_mcp_problem_reports_total", Help: "Accepted problem report calls by surface and fixed category; repeated reports are not unique issues.",
}, []string{"surface", "category"})

type callMetadataKey struct{}
type callState struct {
	ran bool
	err error
}
type callMetadata struct {
	client         string
	conversation   string
	state          callState
	reportCategory string
}

func metadataForCall(req *mcp.CallToolRequest) *callMetadata {
	out := &callMetadata{}
	meta := req.Params.GetMeta()
	var clientName, sessionName, userAgent string
	if info, ok := meta["io.modelcontextprotocol/clientInfo"].(map[string]any); ok {
		clientName, _ = info["name"].(string)
	}
	if req.Session != nil {
		if init := req.Session.InitializeParams(); init != nil && init.ClientInfo != nil {
			sessionName = init.ClientInfo.Name
		}
	}
	if req.Extra != nil {
		userAgent = req.Extra.Header.Get("User-Agent")
	}
	out.client = clientFamily(clientName, sessionName, userAgent)
	// A conversation is caller-supplied correlation, never an authorization boundary.
	// UUID-only input and hashing keep arbitrary text and raw identifiers out of logs.
	if analytics, ok := meta["freehire.me/analytics"].(map[string]any); ok {
		value, _ := analytics["conversation_id"].(string)
		if id, err := uuid.Parse(value); err == nil && len(value) == 36 && id != uuid.Nil {
			digest := sha256.Sum256([]byte(id.String()))
			out.conversation = hex.EncodeToString(digest[:16])
		}
	}
	if req.Params.Name == "report_problem" {
		var report struct {
			Category string `json:"category"`
		}
		if json.Unmarshal(req.Params.Arguments, &report) == nil {
			switch report.Category {
			case "tool_error", "missing_capability", "other":
				out.reportCategory = report.Category
			}
		}
	}
	return out
}

var clientFamilies = []string{"chatgpt", "claude", "cursor", "codex", "vscode", "zed"}

func clientFamily(names ...string) string {
	fallback := "unknown"
	for _, name := range names {
		if name == "" {
			continue
		}
		fallback = "other"
		if len(name) > 256 {
			continue
		}
		words := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		})
		for _, word := range words {
			// Return the literal, never word: that shares memory with the header, which
			// fasthttp reuses for the next request, and this value becomes a metric label.
			if i := slices.Index(clientFamilies, word); i >= 0 {
				return clientFamilies[i]
			}
		}
	}
	return fallback
}

func observeSearchQuality(surface, name string, result *mcp.CallToolResult) []any {
	if name != "search_jobs" && name != "search" {
		return nil
	}
	var payload []byte
	if result.StructuredContent != nil {
		payload, _ = json.Marshal(result.StructuredContent)
	} else if len(result.Content) == 1 {
		if text, ok := result.Content[0].(*mcp.TextContent); ok {
			payload = []byte(text.Text)
		}
	}
	var summary struct {
		Jobs          []json.RawMessage `json:"jobs"`
		Results       []json.RawMessage `json:"results"`
		IgnoredParams []string          `json:"ignored_params"`
		Semantic      string            `json:"semantic"`
	}
	if json.Unmarshal(payload, &summary) != nil {
		return nil
	}
	// Missing result arrays are unknown, not an empty search.
	if summary.Jobs == nil && summary.Results == nil {
		return nil
	}
	returned := len(summary.Jobs) + len(summary.Results)
	mcpSearchQuality.WithLabelValues(surface, "all").Inc()
	if returned == 0 {
		mcpSearchQuality.WithLabelValues(surface, "empty").Inc()
	}
	ignored := len(summary.IgnoredParams) > 0
	if ignored {
		mcpSearchQuality.WithLabelValues(surface, "ignored_filters").Inc()
	}
	semantic := "not_requested"
	switch summary.Semantic {
	case "applied", "unavailable":
		semantic = summary.Semantic
	}
	if semantic == "unavailable" {
		mcpSearchQuality.WithLabelValues(surface, "semantic_fallback").Inc()
	}
	if semantic != "not_requested" {
		mcpSearchQuality.WithLabelValues(surface, "semantic_requested").Inc()
	}
	attrs := []any{"returned_results", returned, "ignored_filters", ignored, "semantic", semantic}
	if ignored {
		// ignored_params entries read param=value; only the name may reach a log, and an
		// entry without the "=" is not that shape, so it is not trusted to be a name.
		var names []string
		for _, entry := range summary.IgnoredParams {
			if name, _, ok := strings.Cut(entry, "="); ok {
				names = append(names, name)
			}
		}
		attrs = append(attrs, "ignored", strings.Join(loggableNames(names), ","))
	}
	return attrs
}

const (
	maxLoggedNames   = 5
	maxLoggedNameLen = 40
)

var (
	// The SDK's schema rejection is text: "validating /properties/limit: type: ..." for a
	// wrong type, "unexpected additional properties [\"a\" \"b\"]" for an unknown argument.
	// Both carry the argument name and, for a type error, the caller's value, so only the
	// name is cut out of them.
	identifier         = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	rejectedProperty   = regexp.MustCompile(`/properties/([A-Za-z0-9_.-]+)`)
	rejectedUnexpected = regexp.MustCompile(`unexpected additional properties \[([^\]]*)\]`)
	rejectedQuoted     = regexp.MustCompile(`"([A-Za-z0-9_.-]+)"`)
)

// badArgNames returns the names of the arguments an SDK schema rejection blames — names
// only, never values, which can carry personal text.
func badArgNames(result *mcp.CallToolResult) []string {
	if result == nil || !result.IsError || len(result.Content) != 1 {
		return nil
	}
	content, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		return nil
	}
	text := content.Text[:min(len(content.Text), 2048)]
	var names []string
	if m := rejectedProperty.FindStringSubmatch(text); m != nil {
		names = append(names, m[1])
	}
	if m := rejectedUnexpected.FindStringSubmatch(text); m != nil {
		for _, q := range rejectedQuoted.FindAllStringSubmatch(m[1], -1) {
			names = append(names, q[1])
		}
	}
	return loggableNames(names)
}

// loggableNames keeps the first few distinct names that look like identifiers, each
// bounded, so a caller cannot put free text or an unbounded list into a log line.
func loggableNames(names []string) []string {
	var out []string
	for _, name := range names {
		if len(out) == maxLoggedNames {
			break
		}
		if name == "" || len(name) > maxLoggedNameLen || slices.Contains(out, name) || !identifier.MatchString(name) {
			continue
		}
		out = append(out, name)
	}
	// The SDK names rejected properties in map order; sorted, a log line is stable.
	slices.Sort(out)
	return out
}
