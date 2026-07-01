package sources

import "testing"

func TestNamespaceExternalID(t *testing.T) {
	cases := []struct {
		name  string
		board string
		id    string
		want  string
	}{
		{"boarded platform namespaces by board", "acme", "42", "acme:42"},
		{"boardless source uses empty board prefix", "", "1000166598", ":1000166598"},
	}
	for _, tc := range cases {
		if got := NamespaceExternalID(tc.board, tc.id); got != tc.want {
			t.Errorf("%s: NamespaceExternalID(%q, %q) = %q, want %q", tc.name, tc.board, tc.id, got, tc.want)
		}
	}
}
