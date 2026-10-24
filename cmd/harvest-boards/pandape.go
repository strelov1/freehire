package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/ingest/atsboard"
	"github.com/strelov1/freehire/internal/ingest/sources"
)

// A Pandapé board is a career-site tenant label (<tenant>.<domain>), the key sources.pandape
// lists by. Each deployment is its own provider with its own tenant namespace: InfoJobs Brasil's
// (pandape, <tenant>.pandape.infojobs.com.br) and Computrabajo's Latin-American one
// (pandapelatam, <tenant>.pandape.computrabajo.com).
const (
	// pandapeSitemapIndex is the sitemap robots.txt names: a gzip index of gzip shards, each
	// up to 1,000 vacancy URLs (115 shards, ~113.6k URLs, 3,347 tenant labels on 2026-10-23).
	// It is the only public list that spans tenants. It is NOT the catalogue — a tenant's
	// own listing carries more than its sitemap entries (braspress: 54 against 20) — so it
	// serves discovery only. It names Brazilian tenants only: the Latin-American domain's
	// robots.txt points at this same file.
	pandapeSitemapIndex = "https://www.pandape.com.br/pandapeindexofertas.xml.gz"
	// pandapeLatamCrawlPattern is the Common Crawl query for the Latin-American domain, which
	// publishes no sitemap of its own: every tenant subdomain, no path glob.
	pandapeLatamCrawlPattern = "*.pandape.computrabajo.com"
)

// pandapeProber probes and discovers one Pandapé deployment's career sites.
type pandapeProber struct {
	provider string // the sources provider, which atsboard names for the domain's URLs
	host     string // the career-site domain
	latam    bool   // discover from Common Crawl rather than the Brazilian sitemap
}

// probeClient is the client the Latin-American domain is probed through: its edge answers
// 403 to the project's plain "freehire/0.1" string and serves the browser-compatible form of
// the same identity, which is what the adapter crawls with. nil keeps the default egress.
func (p pandapeProber) probeClient() *sources.Client {
	if p.latam {
		return sources.NewCompatibleUAClient()
	}
	return nil
}

var (
	pandapeBrazilProber = pandapeProber{provider: "pandape", host: "pandape.infojobs.com.br"}
	pandapeLatamProber  = pandapeProber{provider: "pandapelatam", host: "pandape.computrabajo.com", latam: true}
)

// maxRequestsPerSecond holds the harvest to the crawl's own pace: every tenant and the sitemap
// are one origin, which answers a burst with 403 (see sources.PandapeRequestInterval).
func (pandapeProber) maxRequestsPerSecond() float64 {
	return float64(time.Second) / float64(sources.PandapeRequestInterval)
}

// pandapeTitle is the career site's title: "<employer> - Trabalhe Conosco | Pandapé" in
// Brazil, "<employer> - Trabaja con nosotros | Pandapé" elsewhere.
var pandapeTitle = regexp.MustCompile(`^(.+?) - (?:Trabalhe Conosco|Trabaja con nosotros) \| Pandap`)

// probe reads the career site's vacancy list page (/Vacancies): the employer from its title and
// the open-vacancy count from its "subTotal" counter. The list page, not the home page: a
// tenant on the platform's richer home template renders its vacancies only behind a link, and
// its home carries no counter (avos-tech: none on /, 1,241 on /Vacancies). An unknown tenant
// answers 404 ("A Página de Carreiras desta empresa não está mais disponível no Pandapé") and a
// site with no vacancy counts 0; both are a skip. Any other failure is returned, so probeAll
// counts it. A tenant stating more vacancies than the pager can list is logged and skipped.
func (p pandapeProber) probe(ctx context.Context, c httpClient, tenant string) (string, int, error) {
	root, err := c.GetHTML(ctx, "https://"+tenant+"."+p.host+"/Vacancies")
	if err != nil {
		// Only the platform's 404 says the tenant is gone. Anything else — the burst 403
		// above all — is a probe that did not happen, and is surfaced as a failure rather
		// than counted as an absent board.
		var se *sources.StatusError
		if errors.As(err, &se) && se.Code == http.StatusNotFound {
			return "", 0, nil
		}
		return "", 0, err
	}
	n := pandapeSubtotal(root)
	if n == 0 {
		return "", 0, nil
	}
	if n > sources.PandapeListingWindow {
		// The crawl could only ever fail on it (see sources.PandapeListingWindow).
		log.Printf("pandape probe: %s states %d vacancies, past the pager's %d window; skipping", tenant, n, sources.PandapeListingWindow)
		return "", 0, nil
	}
	name := tenant
	if m := pandapeTitle.FindStringSubmatch(pageTitle(root)); m != nil {
		name = strings.TrimSpace(m[1])
	}
	return name, n, nil
}

// pandapeSubtotal reads the career site's <span id="subTotal">N</span>, 0 when it has none.
func pandapeSubtotal(n *html.Node) int {
	if n.Type == html.ElementNode && sources.Attr(n, "id") == "subTotal" && n.FirstChild != nil {
		v, _ := strconv.Atoi(strings.TrimSpace(n.FirstChild.Data))
		return v
	}
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		if v := pandapeSubtotal(ch); v != 0 {
			return v
		}
	}
	return 0
}

type pandapeURLSet struct {
	Locs []string `xml:"url>loc"`
}

type pandapeSitemapSet struct {
	Locs []string `xml:"sitemap>loc"`
}

// discover lists candidate tenants. The Brazilian deployment's are read from its sitemap
// (discoverSitemap); the Latin-American one has none, so its come from Common Crawl's index of
// the domain. Recognition goes through atsboard either way, so the platform's own hosts (www,
// the confidential-employer buckets) never become candidates.
func (p pandapeProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	if p.latam {
		return commonCrawlCandidates(ctx, c, pandapeLatamCrawlPattern, p.tenantOf)
	}
	return p.discoverSitemap(ctx, c)
}

// tenantOf is the tenant label a vacancy URL names on this deployment.
func (p pandapeProber) tenantOf(u string) (string, bool) {
	src, board, _, ok := atsboard.Recognize(strings.TrimSpace(u))
	if !ok || src != p.provider {
		return "", false
	}
	return board, true
}

// discoverSitemap reads the sitemap index and every shard, collecting each vacancy URL's tenant
// in first-seen order. The host-less entries some shards carry ("<loc>/Detail/3552612</loc>")
// name no tenant. A shard that fails is logged and skipped: a short candidate list only means
// fewer probes, never a wrong board.
func (p pandapeProber) discoverSitemap(ctx context.Context, c httpClient) ([]string, error) {
	var index pandapeSitemapSet
	if err := pandapeGzipXML(ctx, c, pandapeSitemapIndex, &index); err != nil {
		return nil, fmt.Errorf("pandape discover: sitemap index: %w", err)
	}
	if len(index.Locs) == 0 {
		return nil, fmt.Errorf("pandape discover: sitemap index lists no shard")
	}
	var tenants []string
	seen := make(map[string]bool)
	for _, shard := range index.Locs {
		var set pandapeURLSet
		if err := pandapeGzipXML(ctx, c, strings.TrimSpace(shard), &set); err != nil {
			log.Printf("pandape discover: %s: %v (skipping shard)", shard, err)
			continue
		}
		for _, loc := range set.Locs {
			board, ok := p.tenantOf(loc)
			if !ok || seen[board] {
				continue
			}
			seen[board] = true
			tenants = append(tenants, board)
		}
	}
	return tenants, nil
}

// pandapeGzipXML fetches a .xml.gz FILE — served as application/x-gzip, not as a gzip
// Content-Encoding, so the body arrives compressed — and decodes the XML inside it.
func pandapeGzipXML(ctx context.Context, c httpClient, u string, v any) error {
	raw, err := c.GetText(ctx, u)
	if err != nil {
		return err
	}
	zr, err := gzip.NewReader(bytes.NewReader([]byte(raw)))
	if err != nil {
		return fmt.Errorf("not gzip: %w", err)
	}
	body, err := io.ReadAll(zr)
	if err != nil {
		return err
	}
	return xml.Unmarshal(body, v)
}
