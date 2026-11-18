package sources

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/platform/htmltext"
)

// halyk adapts Halyk Bank's career site (halykbank.kz/about/career/vacancies), a
// single-company source with no per-tenant board id (boardless). One server-rendered
// listing, then one HTML page per vacancy. There is no API and no JobPosting markup.
//
// The number in /vacancies-inner/N is a position in the current list, not a vacancy id, and
// the apply form's data-id is the same on every page. A stored URL therefore goes stale the
// moment the list shifts, which is why this adapter is not a HydratingSource: SeenRefresh
// would keep touching a link that now belongs to a different posting. Every crawl refetches
// the page and writes the URL it just opened.
//
// Identity is the title and the city, both on the card, so the id is known before the
// detail fetch and a reshuffle of the numbers does not mint a new job. A shared title
// and city additionally needs a distinct readable body; see providers/halyk.md for the
// identity transitions when that collision appears or disappears.
//
// The listing states its own total ("Всего: N вакансий"). The crawl fails unless the
// parsed cards equal that total, which is what earns fullBoardListing. A detail page
// this crawl could not read is an unreadable marker; a 404 is dropped, the same split
// every other link-only adapter makes.
type halyk struct {
	http HTMLGetter
}

const halykListURL = "https://halykbank.kz/about/career/vacancies"

var (
	halykPathPattern  = regexp.MustCompile(`vacancies-inner/\d+(?:[/?#]|$)`)
	halykTotalPattern = regexp.MustCompile(`Всего:\s*(\d+)`)
	halykBase, _      = url.Parse(halykListURL)
)

// NewHalyk builds the Halyk Bank adapter over the given HTTP client.
func NewHalyk(c HTMLGetter) Source { return halyk{http: c} }

func (halyk) Provider() string { return "halyk" }

func (halyk) boardless() {}

func (halyk) fullBoardListing() {}

func (s halyk) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	cards, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, card := range cards {
		counts[card.id]++
	}
	jobs := fetchDetails(cards, defaultDetailWorkers, func(card halykCard) (Job, bool) {
		job, ok := s.detail(ctx, e, card)
		if !ok {
			log.Printf("halyk: vacancy %s: gone", card.url)
		}
		return job, ok
	})
	if len(cards) > 0 && len(jobs) == 0 {
		return nil, fmt.Errorf("halyk: listed %d vacancies and read none of their pages", len(cards))
	}
	seen := map[string]bool{}
	for i := range jobs {
		job := &jobs[i]
		if counts[job.ExternalID] < 2 {
			continue
		}
		body := strings.ToLower(halykText(htmltext.ToTextFull(job.Description)))
		if job.Unreadable || body == "" {
			return nil, fmt.Errorf("halyk: shared title/city identity %s has an unreadable detail", job.ExternalID)
		}
		sum := sha256.Sum256([]byte(body))
		job.ExternalID += ":" + hex.EncodeToString(sum[:8])
		if seen[job.ExternalID] {
			return nil, fmt.Errorf("halyk: ambiguous shared title %q in %q", job.Title, job.Location)
		}
		seen[job.ExternalID] = true
	}
	return jobs, nil
}

type halykCard struct {
	id, url, title, city string
}

func (s halyk) list(ctx context.Context) ([]halykCard, error) {
	root, err := s.http.GetHTML(ctx, halykListURL)
	if err != nil {
		return nil, fmt.Errorf("halyk: listing: %w", err)
	}
	total, err := halykStatedTotal(textContent(root))
	if err != nil {
		return nil, err
	}
	var cards []halykCard
	var walkErr error
	walk(root, func(n *html.Node) bool {
		if walkErr != nil || n.Type != html.ElementNode || n.Data != "a" {
			return walkErr == nil
		}
		href := Attr(n, "href")
		if !halykPathPattern.MatchString(href) {
			return true
		}
		title := halykBoldText(n, "text-lg")
		if title == "" {
			return false
		}
		city := halykCity(n)
		id := halykExternalID(title, city)
		link, err := halykAbsURL(href)
		if err != nil {
			walkErr = err
			return false
		}
		cards = append(cards, halykCard{id: id, url: link, title: title, city: city})
		return false
	})
	if walkErr != nil {
		return nil, walkErr
	}
	if len(cards) != total {
		return nil, fmt.Errorf("halyk: listing states %d vacancies, parsed %d", total, len(cards))
	}
	return cards, nil
}

func (s halyk) detail(ctx context.Context, e CompanyEntry, card halykCard) (Job, bool) {
	root, err := s.http.GetHTML(ctx, card.url)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(card.id, card.url, e.Company), true
		}
		return Job{}, false
	}
	title := firstNonEmpty(halykBoldText(root, "text-3xl"), card.title)
	body := halykBody(root)
	if body == "" {
		return unreadableDetail(card.id, card.url, e.Company), true
	}
	return Job{
		ExternalID:  card.id,
		Title:       title,
		Company:     e.Company,
		Location:    card.city,
		URL:         card.url,
		Description: body,
	}, true
}

func halykStatedTotal(text string) (int, error) {
	m := halykTotalPattern.FindStringSubmatch(text)
	if m == nil {
		return 0, fmt.Errorf("halyk: listing has no stated total")
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, fmt.Errorf("halyk: stated total %q", m[1])
	}
	return n, nil
}

func halykAbsURL(href string) (string, error) {
	u, err := url.Parse(href)
	if err != nil {
		return "", fmt.Errorf("halyk: vacancy url %q: %w", href, err)
	}
	return halykBase.ResolveReference(u).String(), nil
}

// halykExternalID is the stable identity. The path number is not: it moves when the list
// grows or shrinks. Fetch qualifies identities shared by multiple live cards.
func halykExternalID(title, city string) string {
	key := strings.ToLower(halykText(title)) + "\n" + strings.ToLower(halykText(city))
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:8])
}

func halykText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func halykBoldText(root *html.Node, sizeClass string) string {
	var text string
	walk(root, func(n *html.Node) bool {
		if text != "" {
			return false
		}
		if n.Type == html.ElementNode && HasClass(n, sizeClass) && HasClass(n, "font-bold") {
			text = halykText(textContent(n))
			return false
		}
		return true
	})
	return text
}

func halykCity(card *html.Node) string {
	var city string
	walk(card, func(n *html.Node) bool {
		if city != "" {
			return false
		}
		if n.Type == html.ElementNode && HasClass(n, "flex-x") {
			city = halykText(textContent(n))
			return false
		}
		return true
	})
	return city
}

func halykBody(root *html.Node) string {
	var parts []string
	walk(root, func(n *html.Node) bool {
		if n.Type == html.ElementNode && HasClass(n, "vacanices-inner-cie-item") {
			if body := strings.TrimSpace(sanitizeHTML(innerHTML(n))); body != "" {
				parts = append(parts, body)
			}
			return false
		}
		return true
	})
	return strings.Join(parts, "\n")
}
