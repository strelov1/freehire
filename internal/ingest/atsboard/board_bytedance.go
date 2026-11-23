package atsboard

import (
	"net/url"
	"regexp"
	"strings"
)

var byteDancePosition = regexp.MustCompile(`^/(?:campus|experienced)/position/[0-9]+/detail/?$`)

func recognizeByteDance(u *url.URL, host, src, _, apex string) (source, board, canonical string, ok bool) {
	if !strings.EqualFold(u.Hostname(), apex) || u.User != nil || u.Port() != "" ||
		!byteDancePosition.MatchString(u.Path) {
		return "", "", "", false
	}
	return src, host, "https://" + host, true
}
