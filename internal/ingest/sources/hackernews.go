package sources

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// hackernews adapts Hacker News' monthly "Ask HN: Who is hiring?" threads through the Algolia
// HN Search API, which serves the whole thread — every top-level comment with its HTML — as
// one keyless JSON document. Each top-level comment is one employer's post; replies are
// discussion and are not read.
//
// # Boardless, and what a crawl reads
//
// There is no board: the "whoishiring" account posts one hiring thread a month, and a crawl
// reads the two newest (this month's, still filling, and last month's, still live in the
// first weeks of the new one). The thread search is filtered by that account's own author tag
// rather than by free text, because a free-text query is ranked and lets an unrelated recent
// story outrank the thread; the account also posts the sibling "Who wants to be hired?" and
// "Freelancer?" threads, so the title is checked against hackernewsHiringTitle. Both threads
// are read whole and any failure fails the crawl, so the provider is a fullCatalog: a post
// from a thread that has aged out of the window is genuinely no longer offered, and the
// source-scoped sweep closes it.
//
// # The post format is a convention, not a schema
//
// The thread's own instructions ask for "Company | Role | Location | ..." on the first line,
// and most posts follow it, with extra segments for commitment, salary, remote policy and an
// apply link. The first paragraph (up to the first <p>) is read as that header: the first
// pipe segment is the employer and the second the role; a post with fewer than two segments
// names no employer this catalogue could file it under and is dropped, and so is one whose
// first segment opens with a seniority word AND a role noun together ("Senior Software
// Engineer, Frontend") — there every segment is shifted by one, and filing it would invent
// an employer out of a job title. That pairing is deliberately conservative and does not
// catch every role-first header (a plain, unqualified role like "Backend Engineer" still
// slips through), because a broader trigger risks dropping a real employer whose name
// happens to contain a role word ("Lead Bank", "Chief Industries") — see hackernewsSeniority.
// The location is the first later segment that is not a commitment word, a URL or a salary.
// The whole comment, header included, is the body. The apply link is the first anchor in the
// post, falling back to the comment's own permalink.
type hackernews struct {
	http JSONGetter
}

// NewHackerNews builds the adapter over the given HTTP client.
func NewHackerNews(c JSONGetter) Source { return hackernews{http: c} }

func (hackernews) Provider() string { return "hackernews" }

// One global thread, no per-tenant board.
func (hackernews) boardless() {}

// Every post names its own employer.
func (hackernews) aggregator() {}

// Both threads are read whole every run and a failed read fails the crawl.
func (hackernews) fullCatalog() {}

const (
	// hackernewsThreadSearchURL lists the whoishiring account's newest stories. Ten covers the
	// two hiring threads wanted plus their sibling threads.
	hackernewsThreadSearchURL = "https://hn.algolia.com/api/v1/search_by_date?tags=story,author_whoishiring&hitsPerPage=10"
	// hackernewsItemURL is one story with its full comment tree.
	hackernewsItemURL = "https://hn.algolia.com/api/v1/items/%d"
	// hackernewsPermalink is a comment's own page, the fallback link for a post without one.
	hackernewsPermalink = "https://news.ycombinator.com/item?id=%d"
	// hackernewsThreads is how many of the newest hiring threads a crawl reads.
	hackernewsThreads = 2
)

var (
	hackernewsHiringTitle = regexp.MustCompile(`(?i)who is hiring`)
	// hackernewsParagraph splits a comment's HTML into paragraphs. HN emits an unclosed <p>
	// between paragraphs and nothing before the first one.
	hackernewsParagraph = regexp.MustCompile(`(?i)<p\b[^>]*>`)
	// Stops before whitespace or closing punctuation, so a URL wrapped in parens or
	// followed by a comma ("...(https://acme.example/jobs)") does not swallow the
	// delimiter into the match and leave it dangling in the stripped text.
	hackernewsURL = regexp.MustCompile(`https?://[^\s)\]}>,]+`)
	// hackernewsCommitment is a header segment that names a commitment rather than a place.
	hackernewsCommitment = regexp.MustCompile(`(?i)^(full[ -]?time|part[ -]?time|contract(or)?|intern(ship)?s?|permanent|freelance)$`)
	// hackernewsSeniority opens a segment that is a ROLE, not an employer: "Senior Software
	// Engineer, Frontend", "Founding Engineer", "Head of Platform". Paired with
	// hackernewsRoleNoun below, never alone — plenty of real employers open with one of these
	// words ("Lead Bank", "Chief Industries") and only the pair is evidence.
	hackernewsSeniority = regexp.MustCompile(`(?i)^(sr|jr|senior|junior|staff|principal|lead|founding|head|director|vp|chief)\b`)
	// hackernewsRoleNoun is the other half of that evidence: the word a role is built around.
	hackernewsRoleNoun = regexp.MustCompile(`(?i)\b(engineer|engineering|developer|designer|scientist|analyst|architect|manager|programmer|devops|sre|researcher)\b`)
)

// hackernewsSearch is the story search response; only the id and title are read.
type hackernewsSearch struct {
	Hits []struct {
		ObjectID string `json:"objectID"`
		Title    string `json:"title"`
	} `json:"hits"`
}

// hackernewsItem is a story or a comment as the items API serves it. Text is null on a deleted
// comment, and the tree recurses through Children.
type hackernewsItem struct {
	ID        int64            `json:"id"`
	Title     string           `json:"title"`
	Text      string           `json:"text"`
	CreatedAt string           `json:"created_at"`
	Children  []hackernewsItem `json:"children"`
}

// threads finds the newest hiring threads' ids, newest first.
func (s hackernews) threads(ctx context.Context) ([]int64, error) {
	var resp hackernewsSearch
	if err := s.http.GetJSON(ctx, hackernewsThreadSearchURL, &resp); err != nil {
		return nil, fmt.Errorf("hackernews: thread search: %w", err)
	}
	var ids []int64
	for _, h := range resp.Hits {
		if !hackernewsHiringTitle.MatchString(h.Title) {
			continue
		}
		id, err := strconv.ParseInt(h.ObjectID, 10, 64)
		if err != nil || id == 0 {
			continue
		}
		ids = append(ids, id)
		if len(ids) == hackernewsThreads {
			break
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("hackernews: no \"Who is hiring?\" thread among the newest %d whoishiring stories", len(resp.Hits))
	}
	if len(ids) < hackernewsThreads {
		// Not an error — a fresh deploy racing the new month's post, or the account simply
		// not having posted a second thread yet, are both legitimate. But it's a real
		// coverage reduction on a fullCatalog source (a post from an unread thread closes on
		// the next sweep as if withdrawn), so it's worth a line distinguishing it from the
		// intended two-thread read rather than passing silently.
		log.Printf("hackernews: found only %d of %d hiring threads this run", len(ids), hackernewsThreads)
	}
	return ids, nil
}

func (s hackernews) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	ids, err := s.threads(ctx)
	if err != nil {
		return nil, err
	}
	// The (up to two) thread fetches are independent ~500KB requests; run them concurrently
	// rather than doubling the crawl's wall-clock latency for no correctness reason. Any
	// single failure still fails the whole crawl (fullCatalog), so the result only needs
	// collecting once every goroutine has finished.
	threadsData := make([]hackernewsItem, len(ids))
	errs := make([]error, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id int64) {
			defer wg.Done()
			if err := s.http.GetJSON(ctx, fmt.Sprintf(hackernewsItemURL, id), &threadsData[i]); err != nil {
				// fullCatalog: a thread that could not be read must fail the crawl, never shrink it.
				errs[i] = fmt.Errorf("hackernews: thread %d: %w", id, err)
			}
		}(i, id)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}

	var jobs []Job
	for _, thread := range threadsData {
		for _, c := range thread.Children {
			if job, ok := c.toJob(); ok {
				jobs = append(jobs, job)
			}
		}
	}
	return jobs, nil
}

// hackernewsHeader is a post's parsed first line.
type hackernewsHeader struct {
	Company, Title, Location string
	Remote                   bool
}

// hackernewsParseHeader reads the pipe-delimited first paragraph of a post. ok is false for a
// post with no second segment: nothing then separates the employer from the role.
func hackernewsParseHeader(text string) (hackernewsHeader, bool) {
	head := text
	if loc := hackernewsParagraph.FindStringIndex(text); loc != nil {
		head = text[:loc[0]]
	}
	plain := strings.TrimSpace(textFromHTML(head))
	parts := strings.Split(plain, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	if len(parts) < 2 || parts[0] == "" {
		return hackernewsHeader{}, false
	}
	// A post that opens with the ROLE has shifted every segment by one: the role lands in the
	// employer's place and the location in the role's. Filing it writes a job title as a
	// company, which is worse than not filing it at all — it invents an employer, and the
	// company_slug that dedup and the company page are keyed on is then a role. Nothing here
	// can recover the real employer (a post in this shape names it only in prose, if at all),
	// so the post is dropped, the same answer a post with no second segment already gets.
	if hackernewsSeniority.MatchString(parts[0]) && hackernewsRoleNoun.MatchString(parts[0]) {
		return hackernewsHeader{}, false
	}
	// A bare URL as the employer segment (a malformed/reordered post) names no real employer,
	// the same as an all-URL title already gets dropped below.
	company := strings.TrimSpace(hackernewsURL.ReplaceAllString(parts[0], ""))
	if company == "" {
		return hackernewsHeader{}, false
	}
	title := strings.TrimSpace(hackernewsURL.ReplaceAllString(parts[1], ""))
	if title == "" {
		return hackernewsHeader{}, false
	}
	h := hackernewsHeader{
		Company: company,
		Title:   title,
		Remote:  isRemote(strings.Join(parts[1:], " | ")),
	}
	for _, seg := range parts[2:] {
		seg = strings.TrimSpace(hackernewsURL.ReplaceAllString(seg, ""))
		if seg == "" || hackernewsCommitment.MatchString(seg) || hackernewsStartsWithCurrency(seg) {
			continue
		}
		h.Location = seg
		break
	}
	return h, true
}

// hackernewsStartsWithCurrency reports whether seg opens with $, €, or £. Decodes the first
// RUNE rather than slicing the first byte: € and £ are multi-byte in UTF-8, and a byte slice
// cuts into the middle of one, so it never matches the symbol it was meant to catch.
func hackernewsStartsWithCurrency(seg string) bool {
	r, _ := utf8.DecodeRuneInString(seg)
	return strings.ContainsRune("$€£", r)
}

// hackernewsLink is the first absolute http(s) anchor in a post, or "".
func hackernewsLink(fragment string) string {
	root, err := html.Parse(strings.NewReader(fragment))
	if err != nil {
		return ""
	}
	link := ""
	walk(root, func(n *html.Node) bool {
		if link != "" {
			return false
		}
		if n.Type == html.ElementNode && n.Data == "a" {
			if href := strings.TrimSpace(attr(n, "href")); strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
				link = href
				return false
			}
		}
		return true
	})
	return link
}

// toJob maps one top-level comment to a Job. ok is false for a deleted or empty comment and
// for one whose header names no employer and role.
func (c hackernewsItem) toJob() (Job, bool) {
	text := strings.TrimSpace(c.Text)
	if c.ID == 0 || text == "" {
		return Job{}, false
	}
	h, ok := hackernewsParseHeader(text)
	if !ok {
		return Job{}, false
	}
	link := hackernewsLink(text)
	if link == "" {
		link = fmt.Sprintf(hackernewsPermalink, c.ID)
	}
	// WorkMode is left unset on purpose: h.Remote comes from a free-text scan of the whole
	// header tail (title, commitment, salary — not just Location), which is exactly the
	// "location heuristic" source.go's Job.WorkMode contract says must never reach this
	// field. The pipeline gives a set WorkMode precedence over its own location/description
	// dictionary, so setting it here from this heuristic would override that dictionary's
	// more careful resolution with a cruder guess — worse than leaving it empty.
	return Job{
		ExternalID:  strconv.FormatInt(c.ID, 10),
		URL:         link,
		Title:       h.Title,
		Company:     h.Company,
		Location:    h.Location,
		Description: sanitizeHTML(text),
		Remote:      h.Remote,
		PostedAt:    parseRFC3339(c.CreatedAt),
	}, true
}
