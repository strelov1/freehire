package sources

import (
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// Some live adverts no longer publish JobPosting JSON-LD. Keep the listing facts and read
// only the visible advert, after checking that the page belongs to the requested posting.
func mynaviVisibleDetail(root *html.Node, base Job) (Job, bool, error) {
	if metaProperty(root, "og:url") != base.URL {
		return Job{}, false, fmt.Errorf("page carries no matching posting identity")
	}
	description := firstByClass(root, "jobPointArea__wrap-jobDescription")
	if description == nil || collapseSpace(textContent(description)) == "" {
		return Job{}, false, fmt.Errorf("page carries no posting description")
	}
	var body strings.Builder
	body.WriteString(innerHTML(description))
	if requirements := firstByID(root, "jobInfo2"); requirements != nil {
		body.WriteString("<h2>" + textContent(requirements) + "</h2>")
		for n := requirements.NextSibling; n != nil; n = n.NextSibling {
			if n.Type == html.ElementNode && n.Data == "h2" {
				break
			}
			if n.Type == html.ElementNode {
				body.WriteString(innerHTML(n))
			}
		}
	}
	if table := firstByClass(root, "jobOfferTable"); table != nil {
		body.WriteString("<table>" + innerHTML(table) + "</table>")
	}
	base.Description = sanitizeHTML(body.String())
	return base, true, nil
}
