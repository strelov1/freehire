package sponsored

import (
	"strings"
	"testing"
	"time"
)

func TestTokenRoundTrip(t *testing.T) {
	key := []byte("k")
	now := time.Unix(1_800_000_000, 0)
	tok := SignToken(key, Claims{CampaignID: 7, JobID: 42, Placement: PlacementSearch}, now)
	got, err := VerifyToken(key, tok, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if got.CampaignID != 7 || got.JobID != 42 || got.Placement != PlacementSearch || !got.IssuedAt.Equal(now) {
		t.Fatalf("claims = %+v", got)
	}
}

func TestVerifyTokenRefusals(t *testing.T) {
	key := []byte("k")
	now := time.Unix(1_800_000_000, 0)
	search := SignToken(key, Claims{CampaignID: 1, JobID: 2, Placement: PlacementSearch}, now)
	email := SignToken(key, Claims{CampaignID: 1, JobID: 2, Placement: PlacementEmail}, now)
	payload, sig, _ := strings.Cut(search, ".")
	cases := []struct {
		name string
		key  []byte
		tok  string
		at   time.Time
	}{
		{"tampered payload", key, "x" + payload + "." + sig, now},
		{"tampered signature", key, payload + ".AAAA", now},
		{"wrong key", []byte("other"), search, now},
		{"empty key", nil, search, now},
		{"not base64", key, "!!!.???", now},
		{"no separator", key, "abc", now},
		{"empty", key, "", now},
		{"search expired", key, search, now.Add(SearchTokenTTL + time.Second)},
		{"email expired", key, email, now.Add(EmailTokenTTL + time.Second)},
		{"issued in the future", key, search, now.Add(-time.Hour)},
	}
	for _, c := range cases {
		if _, err := VerifyToken(c.key, c.tok, c.at); err == nil {
			t.Errorf("%s: accepted", c.name)
		}
	}
	// Email outlives the search window.
	if _, err := VerifyToken(key, email, now.Add(SearchTokenTTL+time.Hour)); err != nil {
		t.Errorf("email token inside 30 days: %v", err)
	}
}

func TestVerifyTokenWrongPlacement(t *testing.T) {
	key := []byte("k")
	now := time.Unix(1_800_000_000, 0)
	// A correctly signed token naming a placement that does not exist.
	tok := SignToken(key, Claims{CampaignID: 1, JobID: 2, Placement: "banner"}, now)
	if _, err := VerifyToken(key, tok, now); err == nil {
		t.Error("unknown placement accepted")
	}
}
