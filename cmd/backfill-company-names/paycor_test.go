package main

import (
	"context"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/dict/companyname"
	"github.com/strelov1/freehire/internal/platform/db"
)

const paycorBoard = "0123456789abcdef0123456789abcdef"

func paycorResolverFor(rows []string, reply string) (companyname.Resolver, *fakeModel) {
	m := &fakeModel{reply: reply}
	return newPaycorResolver(nil, postingTextResolver{q: &fakeDescriptions{rows: rows}, model: m, source: "paycor", vendor: "Paycor"}), m
}

func TestPaycorResolverFromPostings(t *testing.T) {
	posting := "<p>Join Lakeside Dental Group as a hygienist. Lakeside Dental Group offers benefits.</p>"
	cases := []struct {
		name, reply string
		rows        []string
		want        string
	}{
		{"named in text", `{"employer": "Lakeside Dental Group"}`, []string{posting}, "Lakeside Dental Group"},
		{"not verbatim", `{"employer": "Lakeside Dental"}`, []string{"We hire. Lakeside Group."}, ""},
		{"vendor", `{"employer": "Paycor"}`, []string{"Apply through Paycor today."}, ""},
		{"vendor inside a name", `{"employer": "Paycor Holdings"}`, []string{"Paycor Holdings is hiring."}, ""},
		{"generic", `{"employer": "Our client"}`, []string{"Our client is hiring."}, ""},
		{"control character", "{\"employer\": \"Acme\\u0007 Co\"}", []string{"Acme\u0007 Co is hiring."}, ""},
		{"too long", `{"employer": "` + strings.Repeat("Acme ", 30) + `"}`, []string{strings.Repeat("Acme ", 30)}, ""},
		{"no postings", `{"employer": "Acme"}`, nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, m := paycorResolverFor(c.rows, c.reply)
			got, err := r.Name(context.Background(), paycorBoard)
			if err != nil || got != c.want {
				t.Fatalf("Name = %q, %v; want %q", got, err, c.want)
			}
			if len(c.rows) == 0 && m.calls != 0 {
				t.Errorf("model called %d times for a board without postings", m.calls)
			}
		})
	}
}

func TestPaycorResolverTitleFirst(t *testing.T) {
	title := stubResolver{byBoard: map[string]string{paycorBoard: "Titled Corp"}}
	m := &fakeModel{reply: `{"employer": "Other"}`}
	r := newPaycorResolver(title, postingTextResolver{q: &fakeDescriptions{rows: []string{"Other is hiring"}}, model: m, source: "paycor", vendor: "Paycor"})
	if got, _ := r.Name(context.Background(), paycorBoard); got != "Titled Corp" || m.calls != 0 {
		t.Errorf("Name = %q after %d model calls; want the title's name, no model call", got, m.calls)
	}

	// A title naming only the vendor falls through to the postings.
	title = stubResolver{byBoard: map[string]string{paycorBoard: "Paycor"}}
	r = newPaycorResolver(title, postingTextResolver{q: &fakeDescriptions{rows: []string{"Other is hiring"}}, model: m, source: "paycor", vendor: "Paycor"})
	if got, _ := r.Name(context.Background(), paycorBoard); got != "Other" {
		t.Errorf("Name = %q, want Other from the postings", got)
	}
}

// A board id that is not a 32-hex clientId reaches neither a URL nor the model.
func TestPaycorResolverRejectsBadClientID(t *testing.T) {
	for _, board := range []string{"", "abc", paycorBoard + "x", paycorBoard + "/../x", "evil.example.com#" + paycorBoard[:15]} {
		r, m := paycorResolverFor([]string{"Acme is hiring"}, `{"employer": "Acme"}`)
		if got, err := r.Name(context.Background(), board); got != "" || err != nil || m.calls != 0 {
			t.Errorf("Name(%q) = %q, %v after %d calls; want empty", board, got, err, m.calls)
		}
	}
}

// The pass renames a Paycor board together with its postings and aliases the old slug.
func TestPaycorPlaceholderPassRenamesBoardAndJobs(t *testing.T) {
	r, _ := paycorResolverFor([]string{"Lakeside Dental Group is hiring"}, `{"employer": "Lakeside Dental Group"}`)
	rows := []db.ListPlaceholderCompanyBoardsRow{{Provider: "paycor", Board: paycorBoard, Company: paycorBoard}}
	renames, unresolved := resolvePlaceholders(context.Background(), rows, companyname.Registry{"paycor": r})
	if len(renames) != 1 || unresolved != 0 {
		t.Fatalf("renames = %+v, unresolved = %d", renames, unresolved)
	}
	f := &fakePlaceholderWriter{boardRows: 1}
	if _, _, err := writePlaceholderRename(context.Background(), f, renames[0]); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 3 || !strings.HasPrefix(f.calls[0], "jobs paycor ") || !strings.HasPrefix(f.calls[1], "board paycor/") || !strings.Contains(f.calls[2], "board_name") {
		t.Errorf("calls = %q, want jobs, board, alias", f.calls)
	}
}
