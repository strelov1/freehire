package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

type beehireProber struct{}

var beehireBoardPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,127}$`)

func (beehireProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	board = strings.ToLower(strings.TrimSpace(board))
	if !beehireBoardPattern.MatchString(board) {
		return "", 0, nil
	}
	u := "https://app.beehire.com/users/getPublicCampaigns/" + url.PathEscape(board)
	var body struct {
		Campaigns []struct {
			ID string `json:"_id"`
		} `json:"campaigns"`
		User struct {
			Company string `json:"company"`
		} `json:"user"`
		Branding struct {
			Info struct {
				Name string `json:"name"`
			} `json:"companyInformation"`
		} `json:"employerBranding"`
	}
	if err := c.GetJSON(ctx, u, &body); err != nil {
		var se *sources.StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return "", 0, nil
		}
		return "", 0, err
	}
	if body.Campaigns == nil {
		return "", 0, fmt.Errorf("beehire probe: %s: listing omitted its campaigns", board)
	}
	if len(body.Campaigns) == 0 {
		return "", 0, nil
	}
	name := strings.TrimSpace(body.Branding.Info.Name)
	if name == "" {
		name = strings.TrimSpace(body.User.Company)
	}
	return name, len(body.Campaigns), nil
}

func (beehireProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	return commonCrawlCandidates(ctx, c, "app.beehire.com/career/*", recognizedBoard("beehire"))
}
