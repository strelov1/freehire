package main

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// greythrProber validates a greytHR Hire career site (specsmakershrm.greythr.com) by reading the
// listing the adapter reads (internal/ingest/sources/greythr.go, unexported), which carries every
// published opening in one response, and names the employer from the site's own company details.
// A label no tenant holds, and a tenant without the Hire module, answer the listing with a 404.
type greythrProber struct{}

var greythrBoardPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*\.greythr\.com$`)

func (greythrProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	if !greythrBoardPattern.MatchString(board) {
		return "", 0, nil
	}
	base := "https://" + board + "/hire/api/career/"
	var listing struct {
		Data *[]struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := c.PostJSON(ctx, base+"published_jobs/", map[string]any{}, &listing); err != nil {
		var se *sources.StatusError
		if errors.As(err, &se) && se.Code == http.StatusNotFound {
			return "", 0, nil
		}
		return "", 0, err
	}
	if listing.Data == nil {
		return "", 0, errors.New("greythr probe: " + board + ": answer carries no data")
	}
	n := len(*listing.Data)
	if n == 0 {
		return "", 0, nil
	}
	var company struct {
		Name string `json:"company_name"`
	}
	if err := c.GetJSON(ctx, base+"get_company_details/", &company); err != nil {
		return "", n, nil // the name is best effort; the board is live either way
	}
	return strings.TrimSpace(company.Name), n, nil
}
