package tracerlink

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// fakeRepo records what was minted and can be made to fail for one destination.
type fakeRepo struct {
	byKey    map[string]string
	failFor  string
	upserts  int
	lastHash string
}

func newFakeRepo() *fakeRepo { return &fakeRepo{byKey: map[string]string{}} }

func (f *fakeRepo) Upsert(_ context.Context, cvID uuid.UUID, userID int64, token, sourcePath, destURL, destHash string) (string, error) {
	f.upserts++
	f.lastHash = destHash
	if destURL == f.failFor {
		return "", errors.New("nope")
	}
	key := cvID.String() + "|" + sourcePath + "|" + destHash
	if existing, ok := f.byKey[key]; ok {
		return existing, nil
	}
	f.byKey[key] = token
	return token, nil
}

func TestMintPointsEveryTraceableLinkAtOurRedirect(t *testing.T) {
	repo := newFakeRepo()
	got := NewMinter(repo, ourHosts).Mint(context.Background(), uuid.New(), 7, "https://freehire.me", "acme",
		[]string{"github.com/ada", "mailto:ada@example.com"}, []string{"opensched.dev"})

	if !strings.HasPrefix(got.Header[0], "https://freehire.me/cv/acme-") {
		t.Errorf("header[0] = %q, want our redirect carrying the company prefix", got.Header[0])
	}
	// A mailto: is not traceable, and its position must stay empty rather than shift the rest.
	if got.Header[1] != "" {
		t.Errorf("header[1] = %q, want empty — a mailto: cannot be traced", got.Header[1])
	}
	if !strings.HasPrefix(got.Projects[0], "https://freehire.me/cv/acme-") {
		t.Errorf("projects[0] = %q, want our redirect", got.Projects[0])
	}
}

// The PDF is re-rendered on every download, so this is the common path, not an edge case.
func TestMintingTheSameCVTwiceYieldsTheSameLinks(t *testing.T) {
	repo := newFakeRepo()
	m := NewMinter(repo, ourHosts)
	id := uuid.New()
	first := m.Mint(context.Background(), id, 7, "https://freehire.me", "acme", []string{"github.com/ada"}, nil)
	second := m.Mint(context.Background(), id, 7, "https://freehire.me", "acme", []string{"github.com/ada"}, nil)

	if first.Header[0] != second.Header[0] {
		t.Errorf("a second download produced %q, want the first download's %q", second.Header[0], first.Header[0])
	}
}

// One failing link must not cost the candidate their CV.
func TestAFailedMintLeavesTheOtherLinksTraced(t *testing.T) {
	repo := newFakeRepo()
	repo.failFor = "https://github.com/ada"
	got := NewMinter(repo, ourHosts).Mint(context.Background(), uuid.New(), 7, "https://freehire.me", "acme",
		[]string{"github.com/ada", "linkedin.com/in/ada"}, nil)

	if got.Header[0] != "" {
		t.Errorf("header[0] = %q, want empty — its mint failed", got.Header[0])
	}
	if !strings.HasPrefix(got.Header[1], "https://freehire.me/cv/") {
		t.Errorf("header[1] = %q, want the link that minted fine to still be traced", got.Header[1])
	}
}

// A link already pointing at us is left alone: tracing it would nest a token inside a token.
func TestMintSkipsOurOwnHost(t *testing.T) {
	repo := newFakeRepo()
	got := NewMinter(repo, ourHosts).Mint(context.Background(), uuid.New(), 7, "https://freehire.me", "acme",
		[]string{"https://freehire.me/cv/acme-x7abc"}, nil)

	if got.Header[0] != "" || repo.upserts != 0 {
		t.Errorf("header[0] = %q after %d upserts, want neither", got.Header[0], repo.upserts)
	}
}
