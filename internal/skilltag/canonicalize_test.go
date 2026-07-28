package skilltag

import (
	"reflect"
	"testing"
)

// Canonicalize is the explicit-token entry to the same dictionary Parse mines text
// with. The distinction it exists for: Parse applies the corroboration rule because
// it is reading prose, where an ambiguous word is more likely English than tech.
// A token a caller hands over is an assertion, not prose, so corroboration must not
// apply to it — while "never guess" still must.
func TestCanonicalize(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "an alias resolves to its canonical slug",
			in:   []string{"k8s"},
			want: []string{"kubernetes"},
		},
		{
			name: "an ambiguous word needs no corroboration when stated explicitly",
			in:   []string{"react"},
			want: []string{"react"},
		},
		{
			name: "a multi-word alias resolves",
			in:   []string{"React Native"},
			want: []string{"react-native"},
		},
		{
			// The assistant's system prompt tells the model to speak canonical slugs
			// ("go, react, kubernetes"), and "go" is a canonical with no alias entry of
			// its own — only "golang" maps to it. Dropping it would lose the most common
			// backend skill from every atom the agent recorded.
			name: "an already-canonical slug resolves to itself",
			in:   []string{"go"},
			want: []string{"go"},
		},
		{
			name: "an unknown token emits nothing rather than passing through",
			in:   []string{"blorptech"},
			want: nil,
		},
		{
			name: "a known token survives beside an unknown one",
			in:   []string{"blorptech", "golang"},
			want: []string{"go"},
		},
		{
			name: "aliases of one skill collapse to a single slug",
			in:   []string{"k8s", "Kubernetes", "kubernetes"},
			want: []string{"kubernetes"},
		},
		{
			name: "output is sorted, not input-ordered",
			in:   []string{"typescript", "docker"},
			want: []string{"docker", "typescript"},
		},
		{
			name: "blank and whitespace tokens are dropped",
			in:   []string{"", "   ", "go"},
			want: []string{"go"},
		},
		{
			name: "no tokens yields nothing",
			in:   nil,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Canonicalize(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Canonicalize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// A token carrying more than one skill is a caller mistake, not an input format.
// Canonicalize resolves the whole token or nothing — it must not silently mine a
// sentence, because that would reintroduce the prose problem Parse exists for.
func TestCanonicalizeDoesNotMineSentences(t *testing.T) {
	got := Canonicalize([]string{"I used Kubernetes and Docker at work"})
	if got != nil {
		t.Errorf("Canonicalize(sentence) = %q, want nil — sentences belong to Parse", got)
	}
}
