package atsboard

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	bullhornHost = regexp.MustCompile(`^public-rest([0-9]+)\.bullhornstaffing\.com$`)
	bullhornPath = regexp.MustCompile(`^/rest-services/([A-Za-z0-9]+)/(?:entity/JobOrder/[0-9]+|search/JobOrder)/?$`)
)

func recognizeBullhorn(u *url.URL, host, src, _, _ string) (source, board, canonical string, ok bool) {
	pod := bullhornHost.FindStringSubmatch(host)
	path := bullhornPath.FindStringSubmatch(u.Path)
	if pod == nil || path == nil || u.User != nil || u.Port() != "" || !strings.EqualFold(u.Hostname(), host) {
		return "", "", "", false
	}
	u.Host = host
	u.Path = "/rest-services/" + path[1] + "/search/JobOrder"
	u.RawQuery = url.Values{"query": {"(isOpen:1)"}, "fields": {"id"}}.Encode()
	u.RawPath, u.Fragment = "", ""
	return src, pod[1] + ":" + path[1], u.String(), true
}
