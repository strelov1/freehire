package handler

import (
	"testing"

	"github.com/strelov1/freehire/internal/contribution"
)

// A recorded contribution's wire shape exposes the board it discovered and the surface it was
// submitted through.
func TestToContributionResponse_Shape(t *testing.T) {
	got := toContributionResponse(contribution.Contribution{
		ID: 9, URL: "https://jobs.ashbyhq.com/blitzy", Source: "ashby",
		Board: "blitzy", Status: "pending", Surface: contribution.SurfaceCLI,
	})
	if got.Source != "ashby" || got.Board != "blitzy" {
		t.Errorf("response = %+v, want source + board surfaced", got)
	}
	if got.Surface != contribution.SurfaceCLI {
		t.Errorf("surface = %q, want it surfaced so a caller can see where a row came from", got.Surface)
	}
}
