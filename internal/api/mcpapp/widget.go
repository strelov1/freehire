package mcpapp

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The widget is the page an MCP Apps host (ChatGPT, Claude) draws a tool result in, instead
// of the model retelling it as prose. One self-contained HTML file, embedded in the binary,
// rendering both shapes it is handed: a search_jobs list and one get_job posting.
//
// It lives beside wire.go rather than in web/ on purpose. It reads exactly the fields those
// types publish, and a test holds it to them (widget_fields_test.go); a frontend build would
// also make this binary depend on a JS build, which is a way to ship a stale page with every
// check green. See openspec/changes/archive/2026-10-20-chatgpt-app-widgets/design.md
// and the requirements in openspec/specs/chatgpt-app-widgets/spec.md.

//go:embed widget/jobs.html
var widgetPage []byte

// widgetMIMEType is the MCP Apps profile. A host renders a ui:// resource only under it.
const widgetMIMEType = "text/html;profile=mcp-app"

// widgetResourceURI names the page by its content. A host caches a UI resource by URI, so a
// page edited under an unchanged URI is a page nobody sees until the cache lets go; deriving
// it from the bytes means nobody has to remember to bump it.
var widgetResourceURI = widgetURI(widgetPage)

func widgetURI(page []byte) string {
	sum := sha256.Sum256(page)
	return "ui://freehire/jobs-" + hex.EncodeToString(sum[:6]) + ".html"
}

// widgetMeta is the resource's _meta: the CSP the host enforces on the page. Logos come from
// our own proxy; nothing else is admitted, because the page makes no request of its own —
// every value it draws arrives in the tool result.
//
// Declared on both the listing and the read: hosts differ in which one they consult, and the
// specification describes both.
func widgetMeta() mcp.Meta {
	return mcp.Meta{"ui": map[string]any{
		"csp":           map[string]any{"resourceDomains": []string{"https://logo.freehire.me"}},
		"prefersBorder": true,
	}}
}

// rendersInWidget is the _meta a tool carries when the page can draw its result. Only
// search_jobs and get_job: a host draws the widget for every tool that names it.
//
// Both spellings, as the official ext-apps server SDK writes them: the nested ui.resourceUri
// is the standard, and the flat "ui/resourceUri" is what hosts read before it was.
func rendersInWidget() mcp.Meta {
	return mcp.Meta{
		"ui":             map[string]any{"resourceUri": widgetResourceURI},
		"ui/resourceUri": widgetResourceURI,
	}
}

func addWidget(server *mcp.Server) {
	read := func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
			URI:      req.Params.URI,
			MIMEType: widgetMIMEType,
			Text:     string(widgetPage),
			Meta:     widgetMeta(),
		}}}, nil
	}
	server.AddResource(&mcp.Resource{
		URI:         widgetResourceURI,
		Name:        "jobs",
		Title:       "freehire job cards",
		Description: "Draws job search results and one posting as cards.",
		MIMEType:    widgetMIMEType,
		Meta:        widgetMeta(),
	}, read)
	// A host keeps the tool list it read at connect time, so after a deploy it still asks for
	// the previous page's URI; answering "not found" there left an empty frame. Any earlier
	// jobs page gets the current one.
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "ui://freehire/jobs-{version}.html",
		Name:        "jobs-previous",
		Title:       "freehire job cards",
		MIMEType:    widgetMIMEType,
		Meta:        widgetMeta(),
	}, read)
}
