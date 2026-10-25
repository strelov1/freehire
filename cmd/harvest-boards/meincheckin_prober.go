package main

import (
	"context"
	"errors"
	"html"
	"net/http"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// meincheckinProber validates a CHECK-IN tenant (the <board> of www.mein-check-in.de/<board>) by
// reading the start page the adapter reads — asked of www (which serves its own tenants and
// redirects the sparkasse. ones), then of vr. (whose tenants www does not know). Its menu links
// every open position and its <title> names the employer ("<name> Onlinebewerbung"); a tenant
// neither host knows is a 404. It mirrors internal/ingest/sources/meincheckin.go, which is
// unexported.
type meincheckinProber struct{}

var (
	meincheckinBoardPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	meincheckinMenuLink     = regexp.MustCompile(`class="mainmenu_position_link" href="/[^/"]+/position-(\d+)">\s*([^<]*)`)
	meincheckinTitle        = regexp.MustCompile(`<title>([^<]*?)\s*Onlinebewerbung\s*</title>`)
)

func (meincheckinProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	if !meincheckinBoardPattern.MatchString(board) {
		return "", 0, nil
	}
	var body string
	var err error
	for _, host := range []string{"https://www.mein-check-in.de/", "https://vr.mein-check-in.de/"} {
		body, err = c.GetText(ctx, host+board)
		var se *sources.StatusError
		if !errors.As(err, &se) || se.Code != http.StatusNotFound {
			break
		}
	}
	if err != nil {
		var se *sources.StatusError
		if errors.As(err, &se) && se.Code != http.StatusNotFound {
			return "", 0, err
		}
		return "", 0, nil
	}
	ids := map[string]bool{}
	for _, m := range meincheckinMenuLink.FindAllStringSubmatch(body, -1) {
		// The open application is not a vacancy; the adapter skips it too.
		if !strings.Contains(strings.ToLower(m[2]), "initiativbewerbung") {
			ids[m[1]] = true
		}
	}
	name := ""
	if m := meincheckinTitle.FindStringSubmatch(body); m != nil {
		name = strings.Join(strings.Fields(html.UnescapeString(m[1])), " ")
	}
	return name, len(ids), nil
}

// discover enumerates candidate tenants from Common Crawl's index of *.mein-check-in.de.
func (meincheckinProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	return commonCrawlCandidates(ctx, c, "*.mein-check-in.de", recognizedBoard("meincheckin"))
}
