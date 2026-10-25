package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// talentstormProber validates a talentstorm career site (the <board> of advades.talentstorm.de)
// by reading the offer list the adapter reads, which carries every published offer. It mirrors
// internal/ingest/sources/talentstorm.go, which is unexported. The API names no company; the
// employer is the label of the workplace an offer marks as headquarters (advades GmbH), which
// 2026-10-25's sites mostly carry. The API answers an unknown label 401, the same as a request
// naming no Origin, so a 401 is surfaced as a probe failure rather than skipped: a run where the
// API stopped honouring the Origin would otherwise read as every candidate being absent.
type talentstormProber struct{}

var talentstormBoardPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// talentstormOpenApplicationType is the offer type of an open application, which the adapter
// never yields and so is not counted.
const talentstormOpenApplicationType = 6

func (talentstormProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	if !talentstormBoardPattern.MatchString(board) {
		return "", 0, nil
	}
	var offers *[]struct {
		IsPublished bool `json:"isPublished"`
		IDJobtype   int  `json:"idJobtype"`
		Locations   []struct {
			Location struct {
				IsHeadquarter bool   `json:"isHeadquarter"`
				Label         string `json:"label"`
			} `json:"location"`
		} `json:"jobofferLocations"`
	}
	u := "https://api.talentstorm.de/api/v1/joboffers/basic/" + board + "?pagination=false"
	if err := c.GetJSONWithHeaders(ctx, u, map[string]string{"Origin": "https://" + board + ".talentstorm.de"}, &offers); err != nil {
		var se *sources.StatusError
		if errors.As(err, &se) && se.Code == http.StatusNotFound {
			return "", 0, nil
		}
		return "", 0, err
	}
	if offers == nil {
		return "", 0, fmt.Errorf("talentstorm probe: %s: answer carries no offer list", board)
	}
	n, name := 0, ""
	for _, o := range *offers {
		if !o.IsPublished || o.IDJobtype == talentstormOpenApplicationType {
			continue
		}
		n++
		for _, l := range o.Locations {
			if name == "" && l.Location.IsHeadquarter {
				name = strings.Join(strings.Fields(l.Location.Label), " ")
			}
		}
	}
	return name, n, nil
}

// discover enumerates candidate sites from Common Crawl's index of *.talentstorm.de.
func (talentstormProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	return commonCrawlCandidates(ctx, c, "*.talentstorm.de", recognizedBoard("talentstorm"))
}
