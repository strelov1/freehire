package buddy

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"time"

	"github.com/strelov1/freehire/internal/engage/slots"
)

// maxMockHostCandidates bounds the hosts listing's work. Every candidate's slots are
// computed in memory from three batched reads, which is cheap at today's few dozen
// members; past this many, the hosts with the most recently saved settings are the ones
// considered, and a host beyond the cap is still reachable from their member page.
const maxMockHostCandidates = 200

// ErrUnknownTopic → 422. A topic filter outside MockTopics.
var ErrUnknownTopic = errors.New("buddy: unknown mock topic")

// MockHostCandidate is a member who may host a mock for the viewer, before their slots
// are computed: their handle, the card summary every list shows, and their settings.
type MockHostCandidate struct {
	UserID   int64
	Handle   string
	Card     CardSummary
	Timezone string
	Topics   []string
}

// MockHost is one entry of the hosts listing: who, what they offer, their soonest open
// slot in the viewer's zone, and the member-page aggregate of their revealed reviews.
type MockHost struct {
	Handle   string
	Card     CardSummary
	Topics   []string
	NextSlot slots.Interval
	// Summary carries Completed, RatingCount and RatingAvg; Recent is left empty — the
	// reviews themselves are the member page's.
	Summary MockSummary
}

// MockHosts is the listing and the zone its slots are expressed in.
type MockHosts struct {
	Hosts []MockHost
	Zone  string
}

// Hosts lists, for an active viewer, the members they could book a mock with right now:
// saved, unpaused settings (offering topic, unless it is empty), at least one open slot in
// the ordinary window (MockMinimumNotice to MockHorizon), active, not the viewer, and no
// block with the viewer either way. Soonest next slot first, then by handle. Slots come
// from the same engine and shape OpenSlots uses; a host whose stored zone no longer
// resolves is skipped and logged rather than failing everybody's listing.
func (s *MockService) Hosts(ctx context.Context, viewerID int64, topic, viewerZone string) (MockHosts, error) {
	if topic != "" && !slices.Contains(MockTopics, topic) {
		return MockHosts{}, fmt.Errorf("%w: %q", ErrUnknownTopic, topic)
	}
	if err := s.members.requireActive(ctx, viewerID); err != nil {
		return MockHosts{}, err
	}
	now := s.now()
	from, to := now, now.Add(MockHorizon)
	// The engine over no rules resolves the viewer's zone, so an empty listing still says
	// which zone it would have been in.
	empty, err := mockSlots(nil, time.UTC, nil, from, to, now, viewerZone)
	if err != nil {
		return MockHosts{}, err
	}
	out := MockHosts{Hosts: []MockHost{}, Zone: empty.Zone}

	candidates, err := s.repo.MockHostCandidates(ctx, viewerID, topic, maxMockHostCandidates)
	if err != nil || len(candidates) == 0 {
		return out, err
	}
	ids := make([]int64, len(candidates))
	for i, c := range candidates {
		ids[i] = c.UserID
	}
	rules, err := s.repo.MockAvailabilityFor(ctx, ids)
	if err != nil {
		return MockHosts{}, err
	}
	// Busy is read a day wider than the window on both sides, as hostSlots does.
	busy, err := s.repo.MockBusyFor(ctx, ids, from.AddDate(0, 0, -1), to.AddDate(0, 0, 1))
	if err != nil {
		return MockHosts{}, err
	}

	hostIDs := make([]int64, 0, len(candidates))
	for _, c := range candidates {
		hostZone, ok := ianaZone(c.Timezone)
		if !ok {
			log.Printf("buddy: mock hosts: member %d has timezone %q", c.UserID, c.Timezone)
			continue
		}
		res, err := mockSlots(rules[c.UserID], hostZone, busy[c.UserID], from, to, now, viewerZone)
		if err != nil {
			return MockHosts{}, err
		}
		if len(res.Slots) == 0 {
			continue
		}
		next := slices.MinFunc(res.Slots, func(a, b slots.Interval) int { return a.Start.Compare(b.Start) })
		out.Hosts = append(out.Hosts, MockHost{Handle: c.Handle, Card: c.Card, Topics: c.Topics, NextSlot: next})
		hostIDs = append(hostIDs, c.UserID)
	}
	if len(hostIDs) == 0 {
		return out, nil
	}
	sums, err := s.repo.MockSummaries(ctx, hostIDs)
	if err != nil {
		return MockHosts{}, err
	}
	for i := range out.Hosts {
		out.Hosts[i].Summary = sums[hostIDs[i]]
	}
	slices.SortFunc(out.Hosts, func(a, b MockHost) int {
		return cmp.Or(a.NextSlot.Start.Compare(b.NextSlot.Start), cmp.Compare(a.Handle, b.Handle))
	})
	return out, nil
}
