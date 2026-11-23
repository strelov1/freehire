package sources

import (
	"context"
	"fmt"
	"log"
	"strings"
)

// BoardCanonicalizer is an adapter that can check a candidate board against its platform before
// the catalogue stores it, and return the board id as the platform spells it. A non-nil error
// refuses the candidate (stored as rejected, with the error as the reason). An adapter without a
// transport, or one that cannot reach the platform, must return the board unchanged and nil: an
// unanswered check never blocks. boardcatalog.Inserter asks it.
type BoardCanonicalizer interface {
	CanonicalBoard(ctx context.Context, board string) (string, error)
}

var _ BoardCanonicalizer = oracle{}

// CanonicalBoard checks the board's site against the tenant's own site list. Oracle never
// rejects an unknown siteNumber: it answers with the tenant's default site, so a made-up or
// brand-named site is an alias that ingests the same jobs a second time. Only an ORA_ACTIVE
// site is accepted, and the id is rewritten to the listed spelling (CX_1, not cx_1).
func (o oracle) CanonicalBoard(ctx context.Context, board string) (string, error) {
	if o.http == nil {
		return board, nil // no transport, no check
	}
	b, ok := parseOracleBoardOK(board)
	if !ok {
		return board, nil // a malformed id is Validate's business
	}
	var resp struct {
		Items []struct {
			SiteNumber string `json:"SiteNumber"`
			StatusCode string `json:"StatusCode"`
		} `json:"items"`
		HasMore bool `json:"hasMore"`
	}
	u := fmt.Sprintf("https://%s/hcmRestApi/resources/latest/recruitingCESites?onlyData=true&limit=200", b.host)
	if err := o.http.GetJSON(ctx, u, &resp); err != nil || len(resp.Items) == 0 {
		log.Printf("oracle: %s: site list unavailable (%v); accepting %q unchecked", b.host, err, b.site)
		return board, nil
	}
	for _, s := range resp.Items {
		if strings.EqualFold(s.SiteNumber, b.site) && s.StatusCode == "ORA_ACTIVE" {
			return b.host + "/" + s.SiteNumber, nil
		}
	}
	if resp.HasMore { // shortcut: the list is cut at 200 sites, so absence proves nothing; page it if a tenant ever needs it
		log.Printf("oracle: %s: site list truncated; accepting %q unchecked", b.host, b.site)
		return board, nil
	}
	return "", fmt.Errorf("oracle: site %q is not an active site of %s (Oracle would serve its default site instead)", b.site, b.host)
}

// parseOracleBoardOK is parseOracleBoard for a caller that leaves the malformed case to Validate.
func parseOracleBoardOK(board string) (oracleBoard, bool) {
	b, err := parseOracleBoard(board)
	return b, err == nil
}
