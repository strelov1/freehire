package linksource

import (
	"context"
	"os"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// erecruiterCapture serves the Echo Investment eRecruiter board as captured live on 2026-10-29
// (three list pages, 31 offers) and the detail pages of the offers it holds, recording which
// offers had their detail read.
type erecruiterCapture struct {
	t       *testing.T
	details []string
	// lastPageOnly serves the capture's one-offer third page as the whole board.
	lastPageOnly bool
}

func (c *erecruiterCapture) read(name string) string {
	b, err := os.ReadFile("../sources/testdata/erecruiter/" + name)
	if err != nil {
		c.t.Fatal(err)
	}
	return string(b)
}

func (c *erecruiterCapture) GetText(_ context.Context, url string) (string, error) {
	if c.lastPageOnly {
		if strings.HasSuffix(url, "pn=1") {
			return c.read("echo_list_p3.jsonp"), nil
		}
		return `({"htm":""})`, nil
	}
	for _, p := range []string{"1", "2", "3"} {
		if strings.HasSuffix(url, "pn="+p) {
			return c.read("echo_list_p" + p + ".jsonp"), nil
		}
	}
	return `({"htm":""})`, nil
}

func (c *erecruiterCapture) GetHTML(_ context.Context, url string) (*html.Node, error) {
	for _, oid := range []string{"5015393", "5011939"} {
		if strings.Contains(url, "oid="+oid+"&") {
			c.details = append(c.details, oid)
			return html.Parse(strings.NewReader(c.read("echo_offer_" + oid + ".html")))
		}
	}
	c.details = append(c.details, url)
	return html.Parse(strings.NewReader("<html><body></body></html>"))
}

// An eRecruiter link names its offer only in the query (Offer.aspx?oid=<id>&cfg=<board>…), and
// every offer on a board shares that path. The resolver must hydrate the offer the oid names and
// return it — not the board's first row, and not a body-less refresh of some other offer.
func TestResolveOnBoardResolvesAnErecruiterOfferByItsOid(t *testing.T) {
	const board = "4D3DD27FCA3D40C79E01BEE9745902B7"
	capture := &erecruiterCapture{t: t}
	reg := map[string]sources.Source{"erecruiter": sources.NewErecruiter(capture)}

	raw := "https://skk.erecruiter.pl/Offer.aspx?oid=5011939&cfg=" + board + "&ejoId=623916&ejorId=322876&comId=20002054"
	job, ok, err := ResolveOnBoard(context.Background(), reg, "erecruiter", board, raw)
	if err != nil || !ok {
		t.Fatalf("ResolveOnBoard = (ok %v, err %v), want the linked offer", ok, err)
	}
	if job.ExternalID != board+":5011939" || job.SeenRefresh || job.Description == "" {
		t.Errorf("resolved %s (refresh %v, body %d bytes), want the hydrated offer 5011939",
			job.ExternalID, job.SeenRefresh, len(job.Description))
	}
	if job.Title != "Inżynier Budowy/Inżynierka Budowy" {
		t.Errorf("title = %q", job.Title)
	}
	if len(capture.details) != 1 || capture.details[0] != "5011939" {
		t.Errorf("details read = %v, want only the linked offer", capture.details)
	}
}

// When the offer the oid names is gone, the link resolves to nothing — never to another offer that
// happens to share the Offer.aspx path, even when it is the only other offer on the board.
func TestResolveOnBoardDoesNotSubstituteAnotherErecruiterOffer(t *testing.T) {
	const board = "4D3DD27FCA3D40C79E01BEE9745902B7"
	capture := &erecruiterCapture{t: t, lastPageOnly: true}
	reg := map[string]sources.Source{"erecruiter": sources.NewErecruiter(capture)}

	// The board now lists only offer 4987752; the link names 5011050, closed since it was shared.
	raw := "https://skk.erecruiter.pl/Offer.aspx?oid=5011050&cfg=" + board + "&ejoId=623224&ejorId=322754&comId=20002054"
	job, ok, err := ResolveOnBoard(context.Background(), reg, "erecruiter", board, raw)
	if err != nil {
		t.Fatalf("ResolveOnBoard: %v", err)
	}
	if ok {
		t.Errorf("resolved %s, want nothing: the linked offer could not be read", job.ExternalID)
	}
}

// Another query value that happens to equal a listed offer's id does not make that offer the
// answer: its own URL names a different oid than the link does. 5011050 is listed, but its detail
// answers with no posting; 5011939 is listed and readable.
func TestResolveOnBoardIgnoresAQueryValueThatContradictsTheOid(t *testing.T) {
	const board = "4D3DD27FCA3D40C79E01BEE9745902B7"
	capture := &erecruiterCapture{t: t}
	reg := map[string]sources.Source{"erecruiter": sources.NewErecruiter(capture)}

	raw := "https://skk.erecruiter.pl/Offer.aspx?oid=5011050&cfg=" + board + "&ref=5011939"
	job, ok, err := ResolveOnBoard(context.Background(), reg, "erecruiter", board, raw)
	if err != nil {
		t.Fatalf("ResolveOnBoard: %v", err)
	}
	if ok {
		t.Errorf("resolved %s, want nothing: the link names oid 5011050", job.ExternalID)
	}
}
