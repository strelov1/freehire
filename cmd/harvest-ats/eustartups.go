package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/strelov1/freehire/internal/platform/scraplingfetch"
)

// The EU-Startups company directory (www.eu-startups.com/directory, a WordPress Business
// Directory Plugin site behind Cloudflare): ~33k European startups, 11 per listing page. A
// listing carries only the company's card link; the website is on the card, so a full run is
// one request per listing page plus one per card. Fetched through scrapling-fetch, whose http
// mode passes the site's Cloudflare without a browser.
const eusBase = "https://www.eu-startups.com/directory/"

// eusRate is the request pace against eu-startups.com: one a second, ~10 h for a full run. The
// spike ran sequentially at 1.5 s and was never refused; eusWorkers only hides the latency.
const (
	eusRate    = time.Second
	eusWorkers = 4
)

// maxEUSPageBytes caps one directory page; listing and card pages run 300–400 KB.
const maxEUSPageBytes = 8 << 20

var (
	eusListingRe = regexp.MustCompile(`<div class="listing-title">\s*<h3><a href="https://www\.eu-startups\.com/directory/([^"/?#]+)/"`)
	eusTitleRe   = regexp.MustCompile(`<title>([^<]*?)\s*\|\s*EU-Startups</title>`)
	eusWebsiteRe = regexp.MustCompile(`wpbdp-field-website[^>]*>\s*<span class="field-label">[^<]*</span>\s*<div class="value">([^<]*)</div>`)
)

// parseEUSListing returns the card slugs on one directory listing page, in page order. A page
// past the last one still answers 200, with no listings — that empty result ends the walk.
func parseEUSListing(page string) []string {
	var slugs []string
	for _, m := range eusListingRe.FindAllStringSubmatch(page, -1) {
		slugs = append(slugs, m[1])
	}
	return slugs
}

// parseEUSCard reads the company name and website off a directory card. The website is the
// card's free-text field, so a bare "acme.com" gets a scheme; a card without one has Website "".
func parseEUSCard(page string) companySite {
	var site companySite
	if m := eusTitleRe.FindStringSubmatch(page); m != nil {
		site.Name = strings.TrimSpace(html.UnescapeString(m[1]))
	}
	if m := eusWebsiteRe.FindStringSubmatch(page); m != nil {
		w := strings.TrimSpace(html.UnescapeString(m[1]))
		if w != "" && !strings.HasPrefix(w, "http://") && !strings.HasPrefix(w, "https://") {
			w = "https://" + w
		}
		site.Website = w
	}
	return site
}

// eusRecord is one line of the resumable state file: either a listing page's slugs or one
// card's result. A card is recorded even without a website, so a re-run does not fetch it again;
// a failed fetch is not recorded, so a re-run retries it.
type eusRecord struct {
	Page    int      `json:"page,omitempty"`
	Slugs   []string `json:"slugs,omitempty"`
	Card    string   `json:"card,omitempty"`
	Name    string   `json:"name,omitempty"`
	Website string   `json:"website,omitempty"`
}

// eusState is what a previous run left in the state file.
type eusState struct {
	pages map[int][]string
	cards map[string]companySite
}

// loadEUSState reads the state file; a missing file is an empty state, and a torn last line (a
// run killed mid-write) is skipped rather than failing the resume.
func loadEUSState(r io.Reader) (eusState, error) {
	st := eusState{pages: map[int][]string{}, cards: map[string]companySite{}}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for sc.Scan() {
		var rec eusRecord
		if json.Unmarshal(sc.Bytes(), &rec) != nil {
			continue
		}
		switch {
		case rec.Page > 0:
			st.pages[rec.Page] = rec.Slugs
		case rec.Card != "":
			st.cards[rec.Card] = companySite{Name: rec.Name, Website: rec.Website}
		}
	}
	return st, sc.Err()
}

// runEUStartups walks the EU-Startups directory into the same unmatched {name,website} JSON
// that `resolve` consumes, resuming from (and appending to) statePath.
func runEUStartups(slugFile, statePath string) int {
	existing, err := readSlugSet(slugFile)
	if err != nil {
		log.Printf("harvest-ats: read slug set: %v", err)
		return 1
	}
	get, err := eusFetcher()
	if err != nil {
		log.Printf("harvest-ats: %v", err)
		return 1
	}

	st := eusState{pages: map[int][]string{}, cards: map[string]companySite{}}
	if f, err := os.Open(statePath); err == nil {
		st, err = loadEUSState(f)
		f.Close()
		if err != nil {
			log.Printf("harvest-ats: read state: %v", err)
			return 1
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		log.Printf("harvest-ats: open state: %v", err)
		return 1
	}
	out, err := os.OpenFile(statePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("harvest-ats: open state: %v", err)
		return 1
	}
	defer out.Close()
	var mu sync.Mutex
	enc := json.NewEncoder(out)
	save := func(rec eusRecord) {
		mu.Lock()
		defer mu.Unlock()
		if err := enc.Encode(rec); err != nil {
			log.Printf("harvest-ats: write state: %v", err)
		}
	}
	log.Printf("harvest-ats: eustartups: resuming with %d pages, %d cards", len(st.pages), len(st.cards))

	var slugs []string
	for page := 1; ; page++ {
		found, done := st.pages[page]
		if !done {
			body, err := get(fmt.Sprintf("%spage/%d/", eusBase, page))
			if err != nil {
				log.Printf("harvest-ats: eustartups: listing page %d: %v (re-run to resume)", page, err)
				return 1
			}
			if found = parseEUSListing(body); len(found) == 0 {
				break
			}
			save(eusRecord{Page: page, Slugs: found})
			if page%100 == 0 {
				log.Printf("harvest-ats: eustartups: listing page %d", page)
			}
		}
		slugs = append(slugs, found...)
	}
	log.Printf("harvest-ats: eustartups: %d cards listed, %d already read", len(slugs), len(st.cards))

	var (
		jobs   = make(chan string)
		wg     sync.WaitGroup
		failed int
	)
	for i := 0; i < eusWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for slug := range jobs {
				body, err := get(eusBase + slug + "/")
				if err != nil {
					log.Printf("harvest-ats: eustartups: card %s: %v", slug, err)
					mu.Lock()
					failed++
					mu.Unlock()
					continue
				}
				site := parseEUSCard(body)
				save(eusRecord{Card: slug, Name: site.Name, Website: site.Website})
				mu.Lock()
				st.cards[slug] = site
				if n := len(st.cards); n%500 == 0 {
					log.Printf("harvest-ats: eustartups: %d/%d cards", n, len(slugs))
				}
				mu.Unlock()
			}
		}()
	}
	queued := map[string]bool{}
	for _, s := range slugs {
		mu.Lock()
		_, done := st.cards[s]
		mu.Unlock()
		if !done && !queued[s] {
			queued[s] = true
			jobs <- s
		}
	}
	close(jobs)
	wg.Wait()

	var sites []companySite
	for _, s := range st.cards {
		if s.Website != "" {
			sites = append(sites, s)
		}
	}
	unmatched := dedupeByWebsite(filterUnmatched(sites, existing))
	log.Printf("harvest-ats: eustartups: %d cards, %d with a website, %d unmatched, %d failed (re-run to retry)", len(st.cards), len(sites), len(unmatched), failed)
	if err := json.NewEncoder(os.Stdout).Encode(unmatched); err != nil {
		log.Printf("harvest-ats: encode: %v", err)
		return 1
	}
	return 0
}

// eusFetcher returns a paced GET over scrapling-fetch (SCRAPLING_FETCH_URL/TOKEN) that yields
// the body of a 200 answer and an error for anything else.
func eusFetcher() (func(string) (string, error), error) {
	tr, err := scraplingfetch.New(scraplingfetch.Config{
		URL:   os.Getenv("SCRAPLING_FETCH_URL"),
		Token: os.Getenv("SCRAPLING_FETCH_TOKEN"),
	}, scraplingfetch.ModeHTTP, false, 2*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("eustartups needs scrapling-fetch: %w", err)
	}
	client := &http.Client{Transport: tr}
	limiter := rate.NewLimiter(rate.Every(eusRate), 1)
	return func(u string) (string, error) {
		ctx := context.Background()
		if err := limiter.Wait(ctx); err != nil {
			return "", err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return "", err
		}
		resp, err := client.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("status %d", resp.StatusCode)
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, maxEUSPageBytes))
		return string(b), err
	}, nil
}
