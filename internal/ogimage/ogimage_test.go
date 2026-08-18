package ogimage

import (
	"bytes"
	"image/png"
	"testing"
)

func decodeCard(t *testing.T, data []byte) {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("response is not a valid PNG: %v", err)
	}
	if b := img.Bounds(); b.Dx() != width || b.Dy() != height {
		t.Errorf("size = %dx%d, want %dx%d", b.Dx(), b.Dy(), width, height)
	}
}

func TestRenderCatalogCard_ExactStats(t *testing.T) {
	data, err := RenderCatalogCard(
		"The open-source search engine for tech jobs",
		"Millions of openings indexed straight from company career boards.",
		Stats{OpenJobs: 3_300_658, Companies: 294_282, Sources: 227, Exact: true},
	)
	if err != nil {
		t.Fatalf("RenderCatalogCard: %v", err)
	}
	decodeCard(t, data)
}

// The degraded snapshot leaves Companies at zero and Exact false; the card must
// render without needing to know that in advance, and must not show the zero.
func TestRenderCatalogCard_DegradedStats(t *testing.T) {
	data, err := RenderCatalogCard(
		"freehire's numbers, live",
		"Every figure comes straight from the public API.",
		Stats{OpenJobs: 3_150_000, Sources: 227, Exact: false},
	)
	if err != nil {
		t.Fatalf("RenderCatalogCard: %v", err)
	}
	decodeCard(t, data)
}

func TestRenderCatalogCard_WrapsALongHeading(t *testing.T) {
	data, err := RenderCatalogCard(
		"A heading long enough that it cannot possibly fit on a single line of this card no matter the font size chosen",
		"",
		Stats{OpenJobs: 1, Companies: 1, Sources: 1, Exact: true},
	)
	if err != nil {
		t.Fatalf("RenderCatalogCard: %v", err)
	}
	decodeCard(t, data)
}

func TestGroupThousands(t *testing.T) {
	cases := map[int64]string{
		0:       "0",
		227:     "227",
		1000:    "1,000",
		294282:  "294,282",
		3300658: "3,300,658",
		-12345:  "-12,345",
	}
	for in, want := range cases {
		if got := groupThousands(in); got != want {
			t.Errorf("groupThousands(%d) = %q, want %q", in, got, want)
		}
	}
}
