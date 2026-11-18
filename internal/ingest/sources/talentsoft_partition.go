package sources

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// talentsoftSlice names a filtered slice in an error message, or "" for the whole site.
func talentsoftSlice(filters url.Values) string {
	if len(filters) == 0 {
		return ""
	}
	return " (" + filters.Encode() + ")"
}

var (
	// talentsoftFacetCount is the count a facet value's label ends in ("CDI (3382)").
	talentsoftFacetCount = regexp.MustCompile(`\((\d+)\)\s*$`)
	talentsoftFacetID    = regexp.MustCompile(`^[0-9]+$`)
)

// talentsoftDim is one search dimension a slice can be split along: the facet_<name> URL
// parameter and its values with their counts. Counts read from the facet panel are the slice's
// own; counts read from the search form are the whole site's.
type talentsoftDim struct {
	name        string
	values      []talentsoftDimValue
	conditional bool
}

type talentsoftDimValue struct {
	id    string
	count int
}

// talentsoftSplitSlack is how far a split's values may fall short of the slice they split: a
// posting filed under no value of a field counts in the total and under none of its values.
// What a split leaves out is not read, and the walk's own 90% floor bounds the whole loss.
func talentsoftSplitSlack(total int) int { return max(2, total*5/100) }

// talentsoftPartition picks the dimension to split a slice along. Two sources offer them. The
// facet panel links each value as ?changefacet=1&facet_<dim>=<id> with the slice's own count.
// A site without one still takes the same parameters, which its
// search form names only by field: the contract select is facet_JobDescription_Contract, and a
// geographic checkbox is facet_Country, facet_Region or facet_Department by its depth in the
// tree (its aria-labelledby lists its ancestors). Form counts are the whole site's, so they can
// vouch for a split only at the top of the walk; deeper, a form split is checked after it is
// walked.
//
// Of the dimensions whose counts cover the slice (or that cannot be checked yet), the one whose
// largest value is smallest is taken, which keeps the next level of splitting shallow; checked
// ones come first.
func talentsoftPartition(root *html.Node, filters url.Values, total int, top bool) (talentsoftDim, bool) {
	var best talentsoftDim
	bestChecked, bestMax, found := false, 0, false
	for _, d := range talentsoftDims(root) {
		if filters.Has("facet_"+d.name) || len(d.values) == 0 {
			continue
		}
		sum, largest := 0, 0
		for _, v := range d.values {
			sum += v.count
			largest = max(largest, v.count)
		}
		checked := d.conditional || top
		if checked && (sum > total || total-sum > talentsoftSplitSlack(total)) {
			continue
		}
		switch {
		case !found, checked && !bestChecked:
		case checked != bestChecked, largest > bestMax, largest == bestMax && d.name >= best.name:
			continue
		}
		best, bestChecked, bestMax, found = d, checked, largest, true
	}
	return best, found
}

// talentsoftDims reads every dimension the page offers: the facet panel's links, then the
// search form's contract select and geographic checkboxes.
func talentsoftDims(root *html.Node) []talentsoftDim {
	byName := map[string]*talentsoftDim{}
	var order []string
	add := func(name, id string, count int, conditional bool) {
		d, ok := byName[name]
		if !ok {
			d = &talentsoftDim{name: name, conditional: conditional}
			byName[name] = d
			order = append(order, name)
		}
		for _, v := range d.values {
			if v.id == id {
				return
			}
		}
		d.values = append(d.values, talentsoftDimValue{id: id, count: count})
	}
	count := func(label string) (int, bool) {
		m := talentsoftFacetCount.FindStringSubmatch(collapseSpace(label))
		if m == nil {
			return 0, false
		}
		n, err := strconv.Atoi(m[1])
		return n, err == nil
	}
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		switch {
		case n.Data == "a":
			u, err := url.Parse(Attr(n, "href"))
			if err != nil {
				return false
			}
			c, ok := count(textContent(n))
			if !ok {
				return false
			}
			for k, v := range u.Query() {
				dim, isFacet := strings.CutPrefix(k, "facet_")
				if isFacet && len(v) == 1 && talentsoftFacetID.MatchString(v[0]) {
					add(dim, v[0], c, true)
				}
			}
			return false
		case n.Data == "select" && strings.HasSuffix(Attr(n, "name"), "EngineCriteriaCollection$Contract"):
			for o := n.FirstChild; o != nil; o = o.NextSibling {
				if o.Type != html.ElementNode || o.Data != "option" || !talentsoftFacetID.MatchString(Attr(o, "value")) || Attr(o, "value") == "0" {
					continue
				}
				if c, ok := count(textContent(o)); ok {
					add("JobDescription_Contract", Attr(o, "value"), c, false)
				}
			}
			return false
		case n.Data == "input" && Attr(n, "type") == "checkbox" && strings.Contains(Attr(n, "name"), "GeographicalAreaCollection$"):
			id := Attr(n, "value")
			c, ok := count(Attr(n, "aria-label"))
			level := len(strings.Fields(Attr(n, "aria-labelledby")))
			if !ok || !talentsoftFacetID.MatchString(id) || level < 1 || level > len(talentsoftGeoLevels) {
				return false
			}
			add(talentsoftGeoLevels[level-1], id, c, false)
			return false
		}
		return true
	})
	out := make([]talentsoftDim, 0, len(order))
	for _, name := range order {
		out = append(out, *byName[name])
	}
	return out
}

// talentsoftGeoLevels names the URL parameter of a geographic checkbox by its depth in the
// search form's tree, as the site's own "show the search URL" spells them.
var talentsoftGeoLevels = []string{"Country", "Region", "Department"}
