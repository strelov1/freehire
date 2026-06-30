package sources

import (
	"context"
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/normalize"
	"golang.org/x/net/html"
)

// telegramcareers adapts Telegram's official jobs page (telegram.org/jobs), a single,
// hand-maintained page (boardless, one company) rather than an ATS. The page server-renders
// each role as an <h3> title followed by its description markup inside #dev_page_content, so
// the adapter splits on the headings — no per-role URL, date, or location is published, so
// the dedup id is the title slug and the URL is the shared jobs page.
type telegramcareers struct {
	http HTMLGetter
}

const telegramJobsURL = "https://telegram.org/jobs"

// NewTelegramCareers builds the Telegram careers-page adapter over the given HTTP client.
func NewTelegramCareers(c HTMLGetter) Source { return telegramcareers{http: c} }

func (telegramcareers) Provider() string { return "telegramcareers" }

// telegramcareers serves a single company, so its config entry carries no board.
func (telegramcareers) boardless() {}

func (t telegramcareers) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	root, err := t.http.GetHTML(ctx, telegramJobsURL)
	if err != nil {
		return nil, fmt.Errorf("telegramcareers: fetch jobs page: %w", err)
	}
	content := elementByID(root, "dev_page_content")
	if content == nil {
		return nil, fmt.Errorf("telegramcareers: jobs content not found")
	}

	// Walk the content's children in document order: an <h3> opens a new role and every
	// node after it (until the next <h3>) is that role's description body.
	var jobs []Job
	var title string
	var body strings.Builder
	flush := func() {
		if title == "" {
			return
		}
		jobs = append(jobs, Job{
			ExternalID:  normalize.Slug(title),
			URL:         telegramJobsURL,
			Title:       title,
			Company:     e.Company,
			Description: sanitizeHTML(body.String()),
			// The page states no location; applications route to @jobs_bot. Leave location
			// empty and the remote flag false rather than guessing.
		})
	}
	for c := content.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "h3" {
			flush()
			title = textContent(c)
			body.Reset()
			continue
		}
		if title != "" {
			_ = html.Render(&body, c)
		}
	}
	flush()
	return jobs, nil
}

// elementByID returns the first element node whose id attribute equals id, or nil.
func elementByID(root *html.Node, id string) *html.Node {
	var found *html.Node
	walk(root, func(n *html.Node) bool {
		if found != nil {
			return false
		}
		if n.Type == html.ElementNode && attr(n, "id") == id {
			found = n
			return false
		}
		return true
	})
	return found
}
