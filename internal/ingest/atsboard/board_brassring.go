package atsboard

import (
	"net/url"
	"regexp"
	"strings"
)

const brassringCareerPath = "/TGnewUI/Search/home/HomeWithPreLoad"

var brassringBoardID = regexp.MustCompile(`^[0-9]+$`)

func recognizeBrassring(u *url.URL, host, src, mode, apex string) (source, board, canonical string, ok bool) {
	if !strings.HasSuffix(host, "."+apex) || u.User != nil || u.Port() != "" || !strings.EqualFold(u.Hostname(), host) ||
		!strings.EqualFold(strings.TrimSuffix(u.Path, "/"), brassringCareerPath) {
		return "", "", "", false
	}
	queryValues, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", "", "", false
	}
	ids := make(map[string]string, 2)
	for key, values := range queryValues {
		key = strings.ToLower(key)
		if key != "partnerid" && key != "siteid" {
			continue
		}
		if len(values) != 1 || ids[key] != "" || !brassringBoardID.MatchString(values[0]) {
			return "", "", "", false
		}
		ids[key] = values[0]
	}
	if len(ids) != 2 {
		return "", "", "", false
	}
	query := url.Values{"partnerid": {ids["partnerid"]}, "siteid": {ids["siteid"]}}.Encode()
	u.Host, u.Path, u.RawPath, u.RawQuery, u.Fragment = host, brassringCareerPath, "", query, ""
	return src, host + "?" + query, u.String(), true
}
