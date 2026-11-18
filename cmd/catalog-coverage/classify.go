package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/ingest/atsboard"
	"github.com/strelov1/freehire/internal/job/jobhash"
)

// The buckets a sampled posting lands in, from best to worst news for the catalogue.
const (
	bucketHave           = "have"            // we hold an open posting of this role at this company
	bucketKnownBoard     = "known_board"     // its ATS board is in our catalogue, the posting is not: a crawl gap
	bucketNewBoard       = "new_board"       // an ATS board we recognise but do not crawl: harvest it
	bucketKnownCompany   = "known_company"   // no recognisable board, but we hold other roles at the company
	bucketUnknownCompany = "unknown_company" // nothing of this employer in the catalogue
	bucketUnusable       = "unusable"        // no company or no title to match on
)

var buckets = []string{bucketHave, bucketKnownBoard, bucketNewBoard, bucketKnownCompany, bucketUnknownCompany, bucketUnusable}

// sample is one posting from an outside catalogue. Ref names that catalogue, so one file can
// carry several and the summary splits by it.
type sample struct {
	Ref, Name, Title, URL string
}

// verdict is a sample's bucket plus the board its URL resolved to, if any.
type verdict struct {
	Bucket, Provider, Board string
}

// catalogue answers the questions a sample is classified by. The open postings of the
// sample's employers are loaded up front (roles, companies); a board is asked about one at a
// time.
type catalogue struct {
	roles     map[string]bool // jobhash.RoleKey over the folded company key
	companies map[string]bool // folded company keys holding any open posting
	board     func(provider, board string) (boardPostings, error)
}

// boardPostings is what the catalogue holds on one ATS board.
type boardPostings struct {
	known bool
	ids   []string        // native posting ids: external_id with its "<board>:" namespace cut off
	roles map[string]bool // jobhash.RoleKey of each open title, under boardRoleScope
}

// boardRoleScope stands in for the company in a RoleKey taken on one board: the board already
// names the employer, and the sample's spelling of it ("Anduril" against our "Anduril
// Industries") is exactly what must not decide the match.
const boardRoleScope = "board"

// minIDLen keeps a short native id from matching by accident somewhere inside a URL.
const minIDLen = 4

// holds reports whether the board's open postings include the sampled one: the apply link
// carries one of its native ids, or one of its titles is the same role.
func (b boardPostings) holds(s sample) bool {
	link := strings.ToLower(s.URL)
	for _, id := range b.ids {
		if len(id) >= minIDLen && containsToken(link, strings.ToLower(id)) {
			return true
		}
	}
	role := jobhash.RoleKey(boardRoleScope, s.Title)
	return role != "" && b.roles[role]
}

// containsToken reports whether id occurs in link with no letter or digit on either side, so
// a held posting 123456 does not claim a link to 1234567.
func containsToken(link, id string) bool {
	for from := 0; ; {
		i := strings.Index(link[from:], id)
		if i < 0 {
			return false
		}
		start := from + i
		end := start + len(id)
		if !isAlnum(link, start-1) && !isAlnum(link, end) {
			return true
		}
		from = start + 1
	}
}

func isAlnum(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return false
	}
	c := s[i]
	return c >= 'a' && c <= 'z' || c >= '0' && c <= '9'
}

func classify(s sample, c catalogue) (verdict, error) {
	key := normalize.CompanyKey(s.Name)
	role := jobhash.RoleKey(key, s.Title)
	if key == "" || role == "" {
		return verdict{Bucket: bucketUnusable}, nil
	}
	if c.roles[role] {
		return verdict{Bucket: bucketHave}, nil
	}
	if provider, board, _, ok := atsboard.Recognize(s.URL); ok {
		b, err := c.board(provider, board)
		if err != nil {
			return verdict{}, err
		}
		v := verdict{Bucket: bucketNewBoard, Provider: provider, Board: board}
		switch {
		case b.holds(s):
			v.Bucket = bucketHave
		case b.known:
			v.Bucket = bucketKnownBoard
		}
		return v, nil
	}
	if c.companies[key] {
		return verdict{Bucket: bucketKnownCompany}, nil
	}
	return verdict{Bucket: bucketUnknownCompany}, nil
}

// parseSamples reads a CSV with a header naming at least name, title and url, in any order;
// ref is optional and other columns are ignored. It shares name/url with
// cmd/seed-from-inventory, so the same file feeds both.
func parseSamples(r io.Reader) ([]sample, error) {
	cr := csv.NewReader(r)
	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	col := make(map[string]int, len(header))
	for i, h := range header {
		col[h] = i
	}
	for _, need := range []string{"name", "title", "url"} {
		if _, ok := col[need]; !ok {
			return nil, fmt.Errorf("missing required column %q", need)
		}
	}
	refIdx, hasRef := col["ref"]

	var out []sample
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("parse row: %w", err)
		}
		s := sample{Name: rec[col["name"]], Title: rec[col["title"]], URL: rec[col["url"]]}
		if hasRef {
			s.Ref = rec[refIdx]
		}
		out = append(out, s)
	}
}
