package main

import (
	"fmt"
	"strings"
)

// parseTenants parses the --tenants flag: comma-separated "key:value" pairs, mirroring a
// board file's tenants map (e.g. "Arvato_Systems:Arvato Systems"). Empty input is an empty
// map, not an error — most boards have none.
func parseTenants(s string) (map[string]string, error) {
	if s == "" {
		return nil, nil
	}
	out := make(map[string]string)
	for _, pair := range strings.Split(s, ",") {
		key, value, ok := strings.Cut(pair, ":")
		if !ok {
			return nil, fmt.Errorf("add-board: --tenants pair %q is not key:value", pair)
		}
		out[key] = value
	}
	return out, nil
}
