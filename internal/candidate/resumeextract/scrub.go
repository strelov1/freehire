package resumeextract

import (
	"reflect"
	"regexp"
	"strings"
)

// scrubContactReferences keeps extracted contacts out of facts imported into the
// experience bank and served through the professional projection. It does not detect
// PII or call a model: it rejects fields repeating the contact values already extracted.
// Drop the affected string rather than storing a damaged achievement. Preserve
// identity fields for their authenticated owner.
func scrubContactReferences(s *Structured) {
	contacts := append([]string{s.Email, s.Phone}, s.Links...)
	for i, contact := range contacts {
		contacts[i] = strings.ToLower(strings.TrimSpace(contact))
	}
	var name *regexp.Regexp
	if fullName := strings.TrimSpace(s.FullName); fullName != "" {
		// Unicode word boundaries keep a short name out of unrelated words: Ann is
		// an identity, while "annual" is useful professional evidence.
		name = regexp.MustCompile(`(?i)(^|[^\pL\pN_])` + regexp.QuoteMeta(fullName) + `([^\pL\pN_]|$)`)
	}
	reject := func(value string) bool {
		if name != nil && name.MatchString(value) {
			return true
		}
		for _, contact := range contacts {
			if contact != "" && strings.Contains(strings.ToLower(value), contact) {
				return true
			}
		}
		return false
	}
	v := reflect.ValueOf(s).Elem()
	for i := range v.NumField() {
		switch v.Type().Field(i).Name {
		case "FullName", "Email", "Phone", "Links":
			continue
		}
		scrubValue(v.Field(i), reject)
	}
}

// scrubValue blanks rejected strings recursively, including in nested CV entries.
func scrubValue(v reflect.Value, reject func(string) bool) {
	switch v.Kind() {
	case reflect.String:
		if v.CanSet() && reject(v.String()) {
			v.SetString("")
		}
	case reflect.Struct:
		for i := range v.NumField() {
			scrubValue(v.Field(i), reject)
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			scrubValue(v.Index(i), reject)
		}
	case reflect.Pointer:
		if !v.IsNil() {
			scrubValue(v.Elem(), reject)
		}
	}
}
