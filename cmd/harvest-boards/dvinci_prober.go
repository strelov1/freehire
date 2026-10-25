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

// dvinciProber validates a d.vinci board (a careers HOST — <tenant>.dvinci-hr.com,
// <tenant>.dvinci-easy.com, or an employer's own domain on the platform) by reading the
// publication list the careers site serves at /jobPublication/list.json and counting its
// publications. It mirrors internal/ingest/sources/dvinci.go, which is unexported. The name is
// the first publication's employer; a group site files vacancies under several, and the seed's
// name stands when the list names none.
type dvinciProber struct{}

var dvinciHostPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

func (dvinciProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	board = strings.ToLower(strings.TrimSpace(board))
	if !dvinciHostPattern.MatchString(board) {
		return "", 0, nil
	}
	var doc *[]struct {
		JobOpening *struct {
			Company *struct {
				Name string `json:"name"`
			} `json:"company"`
		} `json:"jobOpening"`
	}
	if err := c.GetJSON(ctx, "https://"+board+"/jobPublication/list.json", &doc); err != nil {
		// No tenant at that host answers 404. Anything else — a refusal, a host that is not
		// d.vinci at all and serves HTML — is surfaced, not read as an absent board.
		var se *sources.StatusError
		if errors.As(err, &se) && se.Code == http.StatusNotFound {
			return "", 0, nil
		}
		return "", 0, err
	}
	if doc == nil {
		return "", 0, fmt.Errorf("dvinci probe: %s: no publication list", board)
	}
	name := ""
	for _, p := range *doc {
		if p.JobOpening != nil && p.JobOpening.Company != nil {
			if name = strings.TrimSpace(p.JobOpening.Company.Name); name != "" {
				break
			}
		}
	}
	return name, len(*doc), nil
}

// discover enumerates candidate boards from Common Crawl's index of both vendor domains.
func (dvinciProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	var out []string
	for _, pattern := range []string{"*.dvinci-hr.com", "*.dvinci-easy.com"} {
		boards, err := commonCrawlCandidates(ctx, c, pattern, recognizedBoard("dvinci"))
		if err != nil {
			return nil, err
		}
		out = append(out, boards...)
	}
	return out, nil
}
