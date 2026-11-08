package skillvec

import (
	"encoding/json"
	"os"
	"testing"
)

// The browser holds the same cases (web/src/lib/jobMatch.test.ts): the percent the filter
// keeps by and the percent on the card are one figure, and this file is what says so.
func TestMatchPercentSharesCasesWithTheCard(t *testing.T) {
	raw, err := os.ReadFile("../../../web/src/lib/fixtures/skillMatchPercent.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name    string   `json:"name"`
		Job     []string `json:"job"`
		Held    []string `json:"held"`
		Percent int      `json:"percent"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("no shared cases")
	}
	for _, c := range cases {
		if got := MatchPercent(c.Job, c.Held); got != c.Percent {
			t.Errorf("%s: MatchPercent = %d, want %d", c.Name, got, c.Percent)
		}
	}
}

// The stop rule is only safe if no posting of six skills or more that shows the percent
// scores beneath it, whatever its size and overlap.
func TestMinCosineAtPercentBoundsEveryQualifyingPosting(t *testing.T) {
	held := registry[:12]
	profile := ProfileVector(held)
	for _, percent := range []int{30, 50, 70, 90, 100} {
		floor := MinCosineAtPercent(profile, percent)
		for n := ballastFloorSkills; n <= 40; n++ {
			for m := 0; m <= n && m <= len(held); m++ {
				// m skills the reader holds, the rest ones it does not.
				skills := append(append([]string{}, held[:m]...), registry[100:100+n-m]...)
				if MatchPercent(skills, held) < percent {
					continue
				}
				if got := Similarity(profile, skills); got < floor {
					t.Errorf("percent %d, %d of %d skills: cosine %.5f below the floor %.5f", percent, m, n, got, floor)
				}
			}
		}
	}
}

// A posting of fewer skills than the ballast floor scores lower at the same percent. That
// is the documented limit of the stop rule, held here so a change to the ballast shows.
func TestShortPostingScoresBelowTheFloor(t *testing.T) {
	held := registry[:12]
	profile := ProfileVector(held)
	if got, floor := Similarity(profile, held[:1]), MinCosineAtPercent(profile, 100); got >= floor {
		t.Errorf("one skill at 100%% scores %.5f, not below the floor %.5f", got, floor)
	}
}

func TestSimilarityOfUnrecognisedSkillsIsZero(t *testing.T) {
	if got := Similarity(ProfileVector(registry[:3]), []string{"not-a-skill"}); got != 0 {
		t.Errorf("Similarity = %v, want 0", got)
	}
}
