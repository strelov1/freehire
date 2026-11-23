package atsboard

import (
	"net/url"
	"strings"
)

// Region returns a URL's explicit data residency where the adapter needs it in
// addition to the board id. Lever EU and MokaHR R1 tenants use separate APIs.
func Region(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	switch strings.ToLower(u.Hostname()) {
	case "jobs.eu.lever.co", "api.eu.lever.co":
		return "eu"
	case "hire-r1.mokahr.com":
		return "r1"
	default:
		return ""
	}
}
