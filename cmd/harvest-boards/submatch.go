package main

import "regexp"

// firstSubmatch returns a pattern's first capture group in s, "" when it does not match. It
// mirrors the unexported helper of the same name in internal/ingest/sources, which this package
// cannot reach; it lives in its own file rather than in one prober's, like pageTitle, because
// it belongs to no single platform.
func firstSubmatch(pattern *regexp.Regexp, s string) string {
	if m := pattern.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}
