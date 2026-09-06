package resumeextract

import (
	"reflect"

	"github.com/strelov1/freehire/internal/candidate/pii"
)

// scrubRedactionPlaceholders blanks every string in s that still carries a redaction
// placeholder.
//
// The model reads a DE-IDENTIFIED CV, so "[REDACTED_NAME_1]" is a mask it saw, never a
// fact the CV stated. Copying one into a field it was not asked for is a mistake the
// model makes and the schema cannot forbid — the same prompt that tells it never to copy
// a placeholder also tells it to copy each highlight faithfully, and the second
// instruction wins often enough to matter. What lands in `resume_structured` is seeded
// into the base CV (cv.Seed) and rendered into a PDF, so a placeholder that survives is
// a candidate's summary reading "[REDACTED_NAME_1] is a backend engineer".
//
// Blanking, never pii.Redactor.Restore: restoring would put the real name back into a
// field that goes to a de-identified reader, which is the failure this whole path exists
// to prevent. An emptied field then reads as "the CV does not state this", which is what
// Sanitize's own drop-the-empty-entry rule already handles.
//
// The walk is by reflection rather than field by field, for the same reason cvedit
// validates paths that way: a field added to Structured is covered without anybody
// remembering to add it here, and a list that has to be remembered is the one that will
// not be.
func scrubRedactionPlaceholders(s *Structured) {
	scrubValue(reflect.ValueOf(s).Elem())
}

// scrubValue blanks placeholder-carrying strings in v and everything it contains. Only
// the kinds Structured is built from are walked; anything else is left alone.
func scrubValue(v reflect.Value) {
	switch v.Kind() {
	case reflect.String:
		if v.CanSet() && pii.ContainsPlaceholder(v.String()) {
			v.SetString("")
		}
	case reflect.Struct:
		for i := range v.NumField() {
			scrubValue(v.Field(i))
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			scrubValue(v.Index(i))
		}
	case reflect.Pointer:
		if !v.IsNil() {
			scrubValue(v.Elem())
		}
	}
}
