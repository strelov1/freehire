package mcpapp

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// A host caches a UI resource by its URI, so a changed page under an unchanged URI is a
// page nobody sees. The URI is derived from the bytes so that cannot happen.
func TestTheWidgetURIFollowsItsBytes(t *testing.T) {
	a, b := widgetURI([]byte("<p>one</p>")), widgetURI([]byte("<p>two</p>"))
	if a == b {
		t.Fatalf("two different pages share the URI %q", a)
	}
	for _, uri := range []string{a, b, widgetResourceURI} {
		if !strings.HasPrefix(uri, "ui://freehire/jobs-") || !strings.HasSuffix(uri, ".html") {
			t.Errorf("URI %q is not ui://freehire/jobs-<hash>.html", uri)
		}
	}
	if widgetResourceURI != widgetURI(widgetPage) {
		t.Errorf("the served URI %q is not derived from the embedded page", widgetResourceURI)
	}
}

// metaUI reads the `ui` block out of a _meta map, the way a host does: through JSON.
func metaUI(t *testing.T, meta mcp.Meta) map[string]any {
	t.Helper()
	raw, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal _meta: %v", err)
	}
	var decoded struct {
		UI map[string]any `json:"ui"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal _meta: %v", err)
	}
	return decoded.UI
}

// The CSP is what the host enforces on the page. It must admit the logo proxy, and
// nothing that would let the page talk to anyone: the widget makes no request of its own.
func assertWidgetCSP(t *testing.T, where string, meta mcp.Meta) {
	t.Helper()
	ui := metaUI(t, meta)
	csp, _ := ui["csp"].(map[string]any)
	if csp == nil {
		t.Fatalf("%s: no _meta.ui.csp", where)
	}
	if got := csp["resourceDomains"]; !slices.Equal(toStrings(got), []string{"https://logo.freehire.me"}) {
		t.Errorf("%s: resourceDomains = %v, want only the logo proxy", where, got)
	}
	for _, key := range []string{"connectDomains", "frameDomains"} {
		if v, ok := csp[key]; ok && len(toStrings(v)) > 0 {
			t.Errorf("%s: %s = %v, want none — the page makes no request", where, key, v)
		}
	}
}

func toStrings(v any) []string {
	items, _ := v.([]any)
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, _ := item.(string)
		out = append(out, s)
	}
	return out
}

func TestTheWidgetIsServedAsAnMCPAppsResource(t *testing.T) {
	session := connect(t, readerThatFinds(t))
	ctx := context.Background()

	listed, err := session.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("resources/list: %v", err)
	}
	if len(listed.Resources) != 1 {
		t.Fatalf("listed %d resources, want exactly the widget", len(listed.Resources))
	}
	res := listed.Resources[0]
	if res.URI != widgetResourceURI || res.MIMEType != widgetMIMEType {
		t.Errorf("listed %q as %q, want %q as %q", res.URI, res.MIMEType, widgetResourceURI, widgetMIMEType)
	}
	assertWidgetCSP(t, "resources/list", res.Meta)

	read, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: res.URI})
	if err != nil {
		t.Fatalf("resources/read: %v", err)
	}
	if len(read.Contents) != 1 {
		t.Fatalf("read %d contents, want one page", len(read.Contents))
	}
	page := read.Contents[0]
	if page.MIMEType != widgetMIMEType || page.Text != string(widgetPage) {
		t.Errorf("read %q (%d bytes), want the embedded page as %q", page.MIMEType, len(page.Text), widgetMIMEType)
	}
	assertWidgetCSP(t, "resources/read", page.Meta)

	// A host that listed tools before a deploy still asks for the previous page's URI.
	const previous = "ui://freehire/jobs-000000000000.html"
	stale, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: previous})
	if err != nil {
		t.Fatalf("resources/read of a previous URI: %v", err)
	}
	if len(stale.Contents) != 1 || stale.Contents[0].Text != string(widgetPage) || stale.Contents[0].URI != previous {
		t.Errorf("a previous URI did not get the current page")
	}
}

// Only the tools whose result the page knows how to draw may point at it: a host renders
// the widget for every tool that names it, and a page handed a salary table it cannot
// read would show an empty frame where the model's answer should be.
func TestOnlyTheRenderToolsNameTheWidget(t *testing.T) {
	session := connect(t, readerThatFinds(t))
	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	renders := map[string]bool{toolSearchJobs: true, toolGetJob: true}
	for _, tool := range listed.Tools {
		uri, _ := metaUI(t, tool.Meta)["resourceUri"].(string)
		// The flat key is the one hosts read before the nested form was standardised; the
		// official ext-apps server SDK writes both, and so do we.
		if legacy, _ := tool.Meta["ui/resourceUri"].(string); legacy != uri {
			t.Errorf("%s: ui/resourceUri = %q, but ui.resourceUri = %q", tool.Name, legacy, uri)
		}
		switch {
		case renders[tool.Name] && uri != widgetResourceURI:
			t.Errorf("%s: resourceUri = %q, want %q", tool.Name, uri, widgetResourceURI)
		case !renders[tool.Name] && uri != "":
			t.Errorf("%s: names the widget, but the page cannot draw its result", tool.Name)
		}
	}
}

// The page's half of the handshake is a contract with every host, and a strict host
// validates it: a request it rejects leaves the page without the theme or the ability to
// open links. These are the field names of McpUiInitializeRequest in the ext-apps
// specification (src/spec.types.ts), which a fake host in a manual check never validated.
func TestTheWidgetIntroducesItselfTheWayTheSpecificationNamesIt(t *testing.T) {
	page := string(widgetPage)
	for _, want := range []string{`"ui/initialize"`, `protocolVersion: "2026-01-26"`, "appInfo:", "appCapabilities:"} {
		if !strings.Contains(page, want) {
			t.Errorf("the page's ui/initialize does not carry %s", want)
		}
	}
	if strings.Contains(page, "clientInfo") {
		t.Error("the page sends clientInfo; McpUiInitializeRequest names it appInfo")
	}
}
