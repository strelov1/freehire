package sources

import (
	"context"
	"errors"
	"log"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// related supplements a completed search with one public page's recommendations.
// Guest detail fragments omit this section. Recommendations never seed another page.
func (s linkedIn) related(ctx context.Context, jobs []Job, seen map[string]bool) []Job {
	var seed Job
	for _, j := range jobs {
		if !j.Unreadable && !j.Removed {
			seed = j
			break
		}
	}
	if seed.ExternalID == "" {
		return nil
	}
	root, final, err := s.get(ctx, seed.URL)
	if err != nil {
		log.Printf("linkedin: recommendations unavailable for %s: %v", seed.ExternalID, err)
		return nil
	}
	verified, err := LinkedInPosting(root, final, seed.ExternalID)
	if err != nil || verified.Removed {
		return nil
	}
	finalURL, err := url.Parse(final)
	if err != nil || finalURL.User != nil {
		return nil
	}
	validCanonical := true
	walk(root, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "link" {
			for _, rel := range strings.Fields(Attr(n, "rel")) {
				if !strings.EqualFold(rel, "canonical") {
					continue
				}
				u, err := url.Parse(Attr(n, "href"))
				if err != nil || u.User != nil || LinkedInPostingID(Attr(n, "href")) != seed.ExternalID {
					validCanonical = false
				}
			}
		}
		return true
	})
	if !validCanonical {
		return nil
	}
	var related []Job
	for _, card := range linkedInRelatedCards(root, seen) {
		if ctx.Err() != nil {
			break
		}
		j, err := s.Posting(ctx, card.URL)
		var status *StatusError
		if errors.As(err, &status) && (status.Code == 403 || status.Code == 429 || status.Code == 999) {
			log.Printf("linkedin: recommendations stopped after refusal: %v", err)
			break
		}
		if err != nil {
			card.Unreadable = true
			related = append(related, card)
			continue
		}
		if !j.Removed {
			related = append(related, j)
		}
	}
	return related
}

func linkedInRelatedCards(root *html.Node, seen map[string]bool) []Job {
	section := firstByClass(root, "similar-jobs")
	if section == nil || section.Data != "section" {
		return nil
	}
	var cards []Job
	walk(section, func(n *html.Node) bool {
		if len(cards) >= 20 {
			return false
		}
		urn := Attr(n, "data-entity-urn")
		if !strings.HasPrefix(urn, "urn:li:jobPosting:") {
			return true
		}
		id := strings.TrimPrefix(urn, "urn:li:jobPosting:")
		if id == "" || id == "0" || seen[id] || LinkedInPostingID("https://www.linkedin.com/jobs/view/"+id+"/") != id {
			return false
		}
		a := firstByClass(n, "base-card__full-link")
		if a == nil || a.Data != "a" || Attr(a, "data-tracking-control-name") != "public_jobs_similar-jobs" {
			return false
		}
		raw := Attr(a, "href")
		u, err := url.Parse(raw)
		if err != nil || u.User != nil || LinkedInPostingID(raw) != id {
			return false
		}
		title, company := firstByClass(n, "base-main-card__title"), firstByClass(n, "base-main-card__subtitle")
		if title == nil || company == nil {
			return false
		}
		j := Job{ExternalID: id, URL: "https://www.linkedin.com/jobs/view/" + id + "/", Title: strings.TrimSpace(textContent(title)), Company: strings.TrimSpace(textContent(company))}
		if j.Title != "" && j.Company != "" {
			seen[id] = true
			cards = append(cards, j)
		}
		return false
	})
	return cards
}
