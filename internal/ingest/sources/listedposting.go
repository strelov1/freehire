package sources

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// listedPosting is a posting a listing names before anything about it is fetched: the id it is
// stored under and the page that hydrates it. The sitemap-only adapters (werkenvoornederland,
// gemeentebanen, academictransfer) list into it, in the order they want postings hydrated.
type listedPosting struct {
	id  string
	url string
}

// hydrateListed is the body of FetchNew those adapters share. A posting the catalogue already
// holds is refreshed by identity, costing no request; up to maxNew of the rest are hydrated by
// detail, in listing order, and a posting past the bound is left for a later run. A posting whose
// detail is not ok is not yielded: nothing is stored for it yet, so a later run simply tries again.
func hydrateListed(postings []listedPosting, seen func(string) bool, maxNew, workers int, detail func(listedPosting) (Job, bool)) []Job {
	var unseen []listedPosting
	var refreshed []Job
	for _, p := range postings {
		if seen(p.id) {
			refreshed = append(refreshed, Job{ExternalID: p.id, URL: p.url, SeenRefresh: true})
			continue
		}
		if len(unseen) < maxNew {
			unseen = append(unseen, p)
		}
	}
	return append(fetchDetails(unseen, workers, detail), refreshed...)
}

// numericListing reads a flat sitemap whose posting URLs carry a numeric id (the pattern's one
// group) and returns its postings once each, highest id — the newest — first.
func numericListing(doc sitemapDoc, pattern *regexp.Regexp) []listedPosting {
	type numbered struct {
		n int64
		p listedPosting
	}
	seen := make(map[int64]bool)
	var all []numbered
	for _, u := range doc.URLs {
		loc := strings.TrimSpace(u.Loc)
		n, err := strconv.ParseInt(firstSubmatch(pattern, loc), 10, 64)
		if err != nil || n <= 0 || seen[n] {
			continue
		}
		seen[n] = true
		all = append(all, numbered{n, listedPosting{id: strconv.FormatInt(n, 10), url: loc}})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].n > all[j].n })
	out := make([]listedPosting, len(all))
	for i, a := range all {
		out[i] = a.p
	}
	return out
}
