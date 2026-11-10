package facetidx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// indexedAttrs is the set of attributes facetd indexes: exactly the filterable attributes of
// the Meilisearch jobs index (search.facetSettings) minus id, is_closed and is_duplicate,
// which are the id map and the closed/duplicate bitmaps. A test in internal/search/search
// holds the two lists equal.
var indexedAttrs = []string{
	"source", "company_slug",
	"work_mode", "regions", "countries", "cities", "metros", "skills", "collections",
	"is_tech", "ai_interview", "auto_apply_available", "ai_archetype", "role_type", "source_type",
	"requires_clearance", "via_recruiter", "equity", "clearance_levels", "perks", "engagement",
	"apply_questions",
	"enrichment.employment_type", "enrichment.education_level", "enrichment.seniority",
	"enrichment.category", "enrichment.domains",
	"enrichment.company_type", "enrichment.company_size", "enrichment.visa_sponsorship",
	"enrichment.salary_currency", "enrichment.salary_period",
	"enrichment.salary_min", "enrichment.salary_max", "enrichment.experience_years_min",
	"enrichment.relocation", "enrichment.english_level", "enrichment.posting_language",
	"posted_ts", "created_ts",
	"reality.class",
}

// rangeOnly are the numeric attributes our code only ever range-filters (Gte on unix
// seconds). They keep the number column alone: indexed as strings too, nearly every
// document would own a value bitmap of its own (10x the memory, and a copy of the value map
// per write batch). `=`, IN, `!=` and facets on them are refused, so the router falls back.
var rangeOnly = map[string]bool{"posted_ts": true, "created_ts": true}

// Attributes returns the attributes the index holds, besides id and the closed/duplicate markers.
func Attributes() []string { return slices.Clone(indexedAttrs) }

// posting is what one document says about one attribute, read the way Meilisearch reads it.
type posting struct {
	empty bool      // an empty array or empty string: IS EMPTY
	strs  []string  // every scalar element as a string, "" skipped
	nums  []float64 // every numeric element
}

// flattened is a document reduced to what the index keeps. An attribute is in attrs when
// the key is in the document at all (null and [] included): that is EXISTS.
type flattened struct {
	id        int64
	closed    bool
	duplicate bool
	attrs     map[string]*posting
}

// flatten reads a document's JSON: nested objects by dot path, each array element as a
// value, booleans as "true"/"false", numbers both as a string and as a number. Only the
// indexed attributes are kept, so free text and _vectors are never touched.
func flatten(raw []byte) (flattened, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		return flattened{}, fmt.Errorf("facetidx: decode document: %w", err)
	}
	idNum, ok := doc["id"].(json.Number)
	if !ok {
		return flattened{}, fmt.Errorf("facetidx: document has no numeric id")
	}
	id, err := idNum.Int64()
	if err != nil {
		return flattened{}, fmt.Errorf("facetidx: document id: %w", err)
	}
	out := flattened{id: id, attrs: make(map[string]*posting, len(indexedAttrs))}
	out.closed, _ = doc["is_closed"].(bool)
	out.duplicate, _ = doc["is_duplicate"].(bool)
	for _, attr := range indexedAttrs {
		var cur any = doc
		found := true
		for _, part := range strings.Split(attr, ".") {
			m, isMap := cur.(map[string]any)
			if !isMap {
				found = false
				break
			}
			if cur, found = m[part]; !found {
				break
			}
		}
		if !found {
			continue
		}
		p := &posting{}
		p.add(cur)
		if rangeOnly[attr] {
			p.strs = nil
		}
		out.attrs[attr] = p
	}
	return out, nil
}

func (p *posting) add(v any) {
	switch x := v.(type) {
	case nil: // null: present, not empty
	case string:
		if x == "" {
			p.empty = true
			return
		}
		p.strs = append(p.strs, x)
	case bool:
		p.strs = append(p.strs, strconv.FormatBool(x))
	case json.Number:
		f, err := x.Float64()
		if err != nil {
			return
		}
		p.nums = append(p.nums, f)
		p.strs = append(p.strs, formatNum(f))
	case []any:
		if len(x) == 0 {
			p.empty = true
		}
		for _, e := range x {
			p.add(e)
		}
	}
}

func formatNum(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }
