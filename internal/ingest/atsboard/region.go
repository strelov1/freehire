package atsboard

import (
	"net/url"
	"strings"
)

// Region returns a URL's explicit data residency where the adapter needs it in
// addition to the board id. Lever EU tenants do not exist on the default API.
func Region(raw string) string {
	u, err := url.Parse(raw)
	if err == nil && (strings.EqualFold(u.Hostname(), "jobs.eu.lever.co") || strings.EqualFold(u.Hostname(), "api.eu.lever.co")) {
		return "eu"
	}
	return ""
}
