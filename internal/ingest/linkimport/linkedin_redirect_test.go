package linkimport

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/ingest/linksource"
)

type linkedInRedirectClient struct{ final string }

func (c linkedInRedirectClient) GetHTML(ctx context.Context, raw string) (*html.Node, error) {
	root, _, err := c.GetHTMLResolved(ctx, raw)
	return root, err
}

func (c linkedInRedirectClient) GetHTMLResolved(context.Context, string) (*html.Node, string, error) {
	root, err := html.Parse(strings.NewReader(`<script type="application/ld+json">{"@type":"JobPosting","title":"Java Engineer","description":"Build Java services.","hiringOrganization":{"name":"Acme"}}</script>`))
	return root, c.final, err
}

func (linkedInRedirectClient) GetJSON(context.Context, string, any) error {
	return fmt.Errorf("unexpected JSON request")
}

func TestResolveRejectsGenericRedirectsOntoLinkedIn(t *testing.T) {
	for _, final := range []string{
		"https://www.linkedin.com/jobs/view/4490679811/",
		"https://nl.linkedin.com/jobs/view/java-engineer-4490679811/",
		"https://www.linkedin.com/jobs/search/",
	} {
		im := &Importer{reg: []linksource.Source{linksource.NewGeneric(linkedInRedirectClient{final: final})}}
		if resolved, ok, err := im.Resolve(context.Background(), "https://short.example.test/role", Board{}); err == nil || ok {
			t.Errorf("generic redirect bypassed LinkedIn application gate: %+v ok=%t err=%v", resolved, ok, err)
		}
	}
}
