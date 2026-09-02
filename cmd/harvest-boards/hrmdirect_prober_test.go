package main

import (
	"context"
	"testing"
)

func TestHRMDirectProbe(t *testing.T) {
	p := hrmdirectProber{}
	listing := `<html><body><div class="reqResult"><table class="reqResultTable">
<tr><td class="posTitle"><a href="job-opening.php?req=100&amp;req_loc=11&amp;&amp;#job">Server</a></td></tr>
<tr><td class="posTitle"><a href="job-opening.php?req=100&amp;req_loc=12&amp;&amp;#job">Server</a></td></tr>
<tr><td class="posTitle"><a href="job-opening.php?req=100&amp;req_loc=12&amp;&amp;#job">Server (apply)</a></td></tr>
</table>
<a href="job-openings.php?search=true&amp;dept=7">Engineering</a>
<a href="job-opening.php?req=100">Broken</a>
</div></body></html>`
	getter := fakeGetter{
		"https://acme.hrmdirect.com/employment/job-openings.php?search=true": listing,
		// A tenant with no openings still serves the page, with the empty-result notice.
		"https://empty.hrmdirect.com/employment/job-openings.php?search=true": `<html><body>
<div class="reqResult"><div id="noResultsMsg"><p>Select options from the menus above.</p></div></div>
</body></html>`,
	}
	// Live: two postings. One requisition open in two locations is two rows; a posting linked
	// twice counts once; and neither the department filter nor a req without req_loc is one.
	if name, n, err := p.probe(context.Background(), getter, "acme"); err != nil || name != "" || n != 2 {
		t.Errorf("live: got (%q,%d,%v), want (\"\",2,nil)", name, n, err)
	}
	if _, n, err := p.probe(context.Background(), getter, "empty"); err != nil || n != 0 {
		t.Errorf("empty: got n=%d err=%v, want 0,nil", n, err)
	}
	// A tenant that does not exist is a skip, never a fatal error.
	if _, n, err := p.probe(context.Background(), getter, "gone"); err != nil || n != 0 {
		t.Errorf("gone: got n=%d err=%v, want 0,nil", n, err)
	}
}
