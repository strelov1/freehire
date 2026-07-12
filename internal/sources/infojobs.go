package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
)

// infojobs adapts infojobs.net, Spain's largest job board. Boardless and multi-company: the IT
// category listing (/ofertas-trabajo/informatica-telecomunicaciones) is paged and each posting
// carries its own employer, so it stays in the source facet and takes the company from the offer.
// The pages are a server-rendered React app that embeds its data as a `window.__INITIAL_PROPS__`
// blob; the listing blob already carries every field EXCEPT the full description (the list holds a
// truncated teaser), so — like justjoin — the full body comes from a per-offer detail fetch run
// only for postings the catalogue does not already have (FetchNew).
type infojobs struct {
	http TextGetter
}

const (
	// infojobsCategory is the IT category slug; its listing is the whole IT firehose (~2.6k offers).
	infojobsCategory = "informatica-telecomunicaciones"
	infojobsListURL  = "https://www.infojobs.net/ofertas-trabajo/%s?page=%d"
	// infojobsMaxPages caps pagination as a backstop; the overview.totalElements count is the
	// primary stop.
	infojobsMaxPages = 200
)

// NewInfoJobs builds the infojobs.net adapter over the given HTTP client.
func NewInfoJobs(c TextGetter) Source { return infojobs{http: c} }

func (infojobs) Provider() string { return "infojobs" }

func (infojobs) boardless() {}

func (infojobs) aggregator() {}

// infojobsListProps is the subset of a listing page's __INITIAL_PROPS__ we read: the page of
// offers and the catalogue size used to stop pagination.
type infojobsListProps struct {
	Offers   []infojobsOffer `json:"offers"`
	Overview struct {
		TotalElements int `json:"totalElements"`
	} `json:"overview"`
}

// infojobsOffer is one listing offer. Description here is a truncated teaser (the full body lives
// on the detail page); Link is a protocol-relative URL with a tracking query. Teleworking is the
// structured work-mode label.
type infojobsOffer struct {
	Code        string `json:"code"`
	Title       string `json:"title"`
	CompanyName string `json:"companyName"`
	Link        string `json:"link"`
	City        string `json:"city"`
	PublishedAt string `json:"publishedAt"`
	Teleworking string `json:"teleworking"`
	Description string `json:"description"`
}

// infojobsDetailProps is the subset of a detail page's __INITIAL_PROPS__ we read: the offer's full
// description, which the listing truncates.
type infojobsDetailProps struct {
	Offer struct {
		Description string `json:"description"`
	} `json:"offer"`
}

// Fetch is the list-only crawl (truncated teaser description): the fallback for callers that do
// not drive hydration. FetchNew is the hydrating path used by ingest.
func (s infojobs) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	offers, err := s.crawl(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	for _, o := range offers {
		if job, ok := o.toJob(); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// FetchNew is the hydrating crawl: it pages the same listing, but fetches a posting's full
// description only for an offer the catalogue does not already have (seen reports whether the
// offer code is ingested). A seen offer refreshes liveness only (no detail request); an unseen
// offer is hydrated with its detail body; a single detail failure is isolated (logged, falling
// back to the list teaser so the offer is still ingested).
func (s infojobs) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	offers, err := s.crawl(ctx)
	if err != nil {
		return nil, err
	}
	return fetchDetails(offers, defaultDetailWorkers, func(o infojobsOffer) (Job, bool) {
		base, ok := o.toJob()
		if !ok {
			return Job{}, false
		}
		if seen(o.Code) {
			// Already ingested: refresh liveness only, never re-upsert content (that would
			// replace the full hydrated body with the list teaser and re-derive facets).
			base.SeenRefresh = true
			return base, true
		}
		body, ok := s.detail(ctx, base.URL)
		if !ok {
			log.Printf("infojobs: detail %q failed; ingesting list teaser", base.URL)
			return base, true
		}
		base.Description = sanitizeHTML(body)
		return base, true
	}), nil
}

// crawl pages the IT category listing and returns every raw offer — the shared list walk behind
// Fetch and FetchNew. It stops at the last page (an empty page or once the whole catalogue is
// collected), capped by infojobsMaxPages.
func (s infojobs) crawl(ctx context.Context) ([]infojobsOffer, error) {
	var offers []infojobsOffer
	for page := 1; page <= infojobsMaxPages; page++ {
		text, err := s.http.GetText(ctx, fmt.Sprintf(infojobsListURL, infojobsCategory, page))
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("infojobs: listing page %d: %w", page, err)
			}
			break // a later page failing ends enumeration with the offers gathered so far
		}
		var props infojobsListProps
		if err := infojobsDecodeProps(text, &props); err != nil {
			if page == 1 {
				return nil, fmt.Errorf("infojobs: listing page %d: %w", page, err)
			}
			break
		}
		if len(props.Offers) == 0 {
			break
		}
		offers = append(offers, props.Offers...)
		if props.Overview.TotalElements > 0 && len(offers) >= props.Overview.TotalElements {
			break
		}
	}
	return offers, nil
}

// detail fetches an offer's detail page and returns its full description, ok=false on a failed
// request or a page with no parseable body — so the caller falls back to the list teaser.
func (s infojobs) detail(ctx context.Context, jobURL string) (string, bool) {
	text, err := s.http.GetText(ctx, jobURL)
	if err != nil {
		return "", false
	}
	var props infojobsDetailProps
	if err := infojobsDecodeProps(text, &props); err != nil {
		return "", false
	}
	if strings.TrimSpace(props.Offer.Description) == "" {
		return "", false
	}
	return props.Offer.Description, true
}

// infojobsPropsMarker matches the `window.__INITIAL_PROPS__ = JSON.parse(` assignment; the JSON
// string literal that follows is the (double-encoded) props blob.
var infojobsPropsMarker = regexp.MustCompile(`window\.__INITIAL_PROPS__\s*=\s*JSON\.parse\(`)

// infojobsDecodeProps extracts the __INITIAL_PROPS__ blob from a page and unmarshals it into v.
// The blob is double-encoded — a JSON string literal (produced for JSON.parse) whose content is
// itself the JSON document — so it is decoded in two steps: a json.Decoder reads the leading
// string token (correctly honouring the `\"` escapes and stopping at the true closing quote,
// which a lazy regex could not), then its unescaped content is unmarshalled into v.
func infojobsDecodeProps(pageText string, v any) error {
	loc := infojobsPropsMarker.FindStringIndex(pageText)
	if loc == nil {
		return errors.New("infojobs: __INITIAL_PROPS__ not found")
	}
	dec := json.NewDecoder(strings.NewReader(pageText[loc[1]:]))
	var inner string
	if err := dec.Decode(&inner); err != nil {
		return fmt.Errorf("infojobs: decode props string: %w", err)
	}
	if err := json.Unmarshal([]byte(inner), v); err != nil {
		return fmt.Errorf("infojobs: unmarshal props: %w", err)
	}
	return nil
}

// infojobsWorkMode maps the structured teleworking label to freehire's work-mode vocabulary.
func infojobsWorkMode(teleworking string) string {
	switch strings.TrimSpace(teleworking) {
	case "Solo teletrabajo":
		return "remote"
	case "Híbrido":
		return "hybrid"
	case "Presencial":
		return "onsite"
	default:
		return ""
	}
}

// toJob maps a listing offer to a Job, returning ok=false for an unusable offer (no code to key
// on, no link to build the URL, or no company). The URL is the offer's protocol-relative link
// with its tracking query stripped and the scheme restored.
func (o infojobsOffer) toJob() (Job, bool) {
	if o.Code == "" || o.Link == "" || o.CompanyName == "" {
		return Job{}, false
	}
	mode := infojobsWorkMode(o.Teleworking)
	return Job{
		ExternalID:  o.Code,
		URL:         "https:" + trimURLSuffix(o.Link),
		Title:       o.Title,
		Company:     o.CompanyName,
		Location:    o.City,
		Description: sanitizeHTML(o.Description),
		Remote:      mode == "remote",
		WorkMode:    mode,
		PostedAt:    parseRFC3339(o.PublishedAt),
	}, true
}
