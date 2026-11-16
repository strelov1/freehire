package mailmeta

import "testing"

func TestGeneratedDigestsAreRecognized(t *testing.T) {
	for _, kind := range []string{InboxDigest, "application-digest"} {
		if !IsDigest(kind, "auto-generated") {
			t.Errorf("digest %q not recognized", kind)
		}
		if IsDigest(kind, "no") {
			t.Errorf("ordinary mail marked as digest")
		}
	}
	if IsDigest("unknown", "auto-generated") {
		t.Fatal("unrelated automated mail excluded")
	}
}
