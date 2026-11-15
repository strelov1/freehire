package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// kekaProber validates a Keka career portal (the <board> of minfy.keka.com/careers) by reading
// the listing the adapter reads (internal/ingest/sources/keka.go, unexported), which carries
// every open posting in one response.
// Keka serves every label (wildcard DNS), and a label with no careers portal is neither a DNS
// failure nor a 404: the API redirects to an HTML page under HTTP 200, "Invalid Tenant" for a
// label no organisation holds, "Forbidden Access" for an organisation without Keka's hiring
// module. The listing is therefore read as text, so those pages (a skip) can be told apart from
// an answer the prober does not understand (surfaced).
// The employer name comes from the portal's careerportalinfo, so a seed's company claim can be
// checked against it: a guessed label can land on a different employer, and that check turns
// the collision into a rejection.
type kekaProber struct{}

var kekaBoardPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// kekaNoPortal holds stable fragments of the pages Keka redirects a label with no careers portal
// to: /careers/Content/TenantNotFound.html and /careers/Content/403.html.
var kekaNoPortal = []string{"<title>Invalid Tenant</title>", "<title>Forbidden Access</title>"}

func (kekaProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	if !kekaBoardPattern.MatchString(board) {
		return "", 0, nil
	}
	base := "https://" + board + ".keka.com/careers"
	body, err := c.GetText(ctx, base+"/api/jobs/default/active")
	if err != nil {
		return "", 0, err
	}
	for _, page := range kekaNoPortal {
		if strings.Contains(body, page) {
			return "", 0, nil
		}
	}
	var postings []struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &postings); err != nil {
		return "", 0, fmt.Errorf("keka probe: %s: unreadable listing: %w", board, err)
	}
	if len(postings) == 0 {
		return "", 0, nil
	}
	// Best effort, as in the adapter: a portal whose name cannot be read is still live.
	var info struct {
		Name string `json:"name"`
	}
	if err := c.GetJSON(ctx, base+"/api/organization/default/careerportalinfo", &info); err != nil {
		return "", len(postings), nil
	}
	return strings.TrimSpace(info.Name), len(postings), nil
}
