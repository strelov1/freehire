package main

import "fmt"

// render writes the gauge set for one run's drift list.
func render(drift []string) string {
	return gauge(
		"freehire_search_settings_drift_count",
		"Sortable/filterable attributes and embedders this binary may request that the live Meilisearch indexes have not yet declared. 0 means the live settings match what this binary expects.",
		len(drift),
	)
}

// gauge writes one HELP/TYPE/value trio. Both are required because the textfile
// collector SKIPS a file it cannot parse: a malformed payload is not a loud failure,
// it is a metric that quietly stops existing — which reads exactly like a healthy
// silence.
func gauge(name, help string, value int) string {
	return fmt.Sprintf("# HELP %s %s\n# TYPE %s gauge\n%s %d\n", name, help, name, name, value)
}
