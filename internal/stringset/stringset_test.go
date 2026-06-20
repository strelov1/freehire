package stringset

import (
	"reflect"
	"testing"
)

func TestSorted(t *testing.T) {
	if Sorted(nil) != nil {
		t.Error("empty set must return nil")
	}
	if Sorted(map[string]struct{}{}) != nil {
		t.Error("empty (non-nil) set must return nil")
	}
	got := Sorted(map[string]struct{}{"go": {}, "c": {}, "rust": {}})
	if !reflect.DeepEqual(got, []string{"c", "go", "rust"}) {
		t.Errorf("got %v, want [c go rust]", got)
	}
}
