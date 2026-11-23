package boardresolve

import (
	"context"
	"net/url"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/atsboard"
)

func (r *Resolver) resolveAppOne(ctx context.Context, rawURL string) (source, board, canonical string, ok bool) {
	u, err := url.Parse(rawURL)
	if err != nil || r.appOnePortal == nil || u.Scheme != "https" || u.User != nil ||
		!strings.EqualFold(u.Host, "apply.appone.com") {
		return "", "", "", false
	}
	id, found := strings.CutPrefix(strings.TrimSuffix(u.Path, "/"), "/job/")
	if !found {
		return "", "", "", false
	}
	source, board, canonical, ok = atsboard.Recognize(r.appOnePortal(ctx, id))
	if !ok || source != "appone" {
		return "", "", "", false
	}
	return source, board, canonical, true
}
