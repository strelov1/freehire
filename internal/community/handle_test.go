package community

import (
	"regexp"
	"testing"
)

var handlePattern = regexp.MustCompile(`^[a-z]+-[a-z]+-[0-9]{2}$`)

func TestGenerateHandleFormat(t *testing.T) {
	for i := 0; i < 200; i++ {
		h := GenerateHandle()
		if !handlePattern.MatchString(h) {
			t.Fatalf("handle %q does not match adjective-noun-NN", h)
		}
	}
}

func TestGenerateHandleVaries(t *testing.T) {
	seen := make(map[string]struct{})
	for i := 0; i < 200; i++ {
		seen[GenerateHandle()] = struct{}{}
	}
	// With a few hundred draws over a large space we expect many distinct handles;
	// a tiny distinct count would signal a broken (constant) generator.
	if len(seen) < 50 {
		t.Fatalf("generator not varied enough: only %d distinct of 200", len(seen))
	}
}
