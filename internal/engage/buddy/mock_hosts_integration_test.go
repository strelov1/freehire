//go:build integration

package buddy_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/buddy"
	"github.com/strelov1/freehire/internal/engage/slots"
)

// listHosts is the viewer's hosts listing in UTC.
func listHosts(t *testing.T, mocks *buddy.MockService, viewerID int64, topic string) []buddy.MockHost {
	t.Helper()
	res, err := mocks.Hosts(context.Background(), viewerID, topic, "UTC")
	if err != nil {
		t.Fatalf("hosts: %v", err)
	}
	return res.Hosts
}

// hostHandles is the listing's handles, in order.
func hostHandles(t *testing.T, mocks *buddy.MockService, viewerID int64, topic string) []string {
	t.Helper()
	hosts := listHosts(t, mocks, viewerID, topic)
	out := make([]string, len(hosts))
	for i, h := range hosts {
		out[i] = h.Handle
	}
	return out
}

func TestMockHostsListsOnlyMembersTheViewerCouldBook(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	ctx := context.Background()
	viewer, _ := joined(t, pool, svc, "viewer@example.test")
	open, openHandle := joined(t, pool, svc, "open@example.test")
	openAllWeek(t, mocks, open, "Europe/Berlin")

	paused, _ := joined(t, pool, svc, "paused@example.test")
	openAllWeek(t, mocks, paused, "UTC")
	if _, err := mocks.SaveSettings(ctx, paused, buddy.MockSettings{
		Timezone: "UTC", MeetingURL: hostLink(paused), Topics: buddy.MockTopics, Paused: true,
	}); err != nil {
		t.Fatal(err)
	}

	noHours, _ := joined(t, pool, svc, "nohours@example.test")
	if _, err := mocks.SaveSettings(ctx, noHours, buddy.MockSettings{
		Timezone: "UTC", MeetingURL: hostLink(noHours), Topics: buddy.MockTopics,
	}); err != nil {
		t.Fatal(err)
	}

	left, _ := joined(t, pool, svc, "left@example.test")
	openAllWeek(t, mocks, left, "UTC")
	if err := svc.Leave(ctx, left); err != nil {
		t.Fatal(err)
	}

	blockedByViewer, _ := joined(t, pool, svc, "blocked@example.test")
	blockerOfViewer, _ := joined(t, pool, svc, "blocker@example.test")
	openAllWeek(t, mocks, blockedByViewer, "UTC")
	openAllWeek(t, mocks, blockerOfViewer, "UTC")
	if _, err := pool.Exec(ctx, `INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1, $2), ($3, $1)`,
		viewer, blockedByViewer, blockerOfViewer); err != nil {
		t.Fatal(err)
	}

	openAllWeek(t, mocks, viewer, "UTC") // the viewer hosts too, and never lists themselves

	if got := hostHandles(t, mocks, viewer, ""); !slices.Equal(got, []string{openHandle}) {
		t.Fatalf("hosts = %v, want only %s", got, openHandle)
	}
}

func TestMockHostsFiltersByTopic(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	viewer, _ := joined(t, pool, svc, "viewer@example.test")
	coder, coderHandle := joined(t, pool, svc, "coder@example.test")
	talker, talkerHandle := joined(t, pool, svc, "talker@example.test")
	openAllWeek(t, mocks, coder, "UTC", buddy.TopicCoding, buddy.TopicSystemDesign)
	openAllWeek(t, mocks, talker, "UTC", buddy.TopicBehavioral)

	if got := hostHandles(t, mocks, viewer, buddy.TopicBehavioral); !slices.Equal(got, []string{talkerHandle}) {
		t.Fatalf("behavioral hosts = %v", got)
	}
	if got := hostHandles(t, mocks, viewer, buddy.TopicSystemDesign); !slices.Equal(got, []string{coderHandle}) {
		t.Fatalf("system design hosts = %v", got)
	}
	if got := hostHandles(t, mocks, viewer, ""); len(got) != 2 {
		t.Fatalf("unfiltered hosts = %v", got)
	}
	if _, err := mocks.Hosts(context.Background(), viewer, "astrology", "UTC"); !errors.Is(err, buddy.ErrUnknownTopic) {
		t.Fatalf("unknown topic: err = %v", err)
	}
}

func TestMockHostsAreSortedBySoonestSlotThenHandle(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	ctx := context.Background()
	viewer, _ := joined(t, pool, svc, "viewer@example.test")
	_, firstHandle := joined(t, pool, svc, "first@example.test")
	_, secondHandle := joined(t, pool, svc, "second@example.test")
	late, lateHandle := joined(t, pool, svc, "late@example.test")
	for _, handle := range []string{firstHandle, secondHandle} {
		var id int64
		if err := pool.QueryRow(ctx, `SELECT user_id FROM buddy_members WHERE handle = $1`, handle).Scan(&id); err != nil {
			t.Fatal(err)
		}
		openAllWeek(t, mocks, id, "UTC")
	}
	// late offers only the weekday three days out, so their first slot is past everyone else's.
	if _, err := mocks.SaveSettings(ctx, late, buddy.MockSettings{
		Timezone: "UTC", MeetingURL: hostLink(late), Topics: buddy.MockTopics,
	}); err != nil {
		t.Fatal(err)
	}
	day := time.Now().UTC().AddDate(0, 0, 3).Weekday()
	if err := mocks.ReplaceWeekly(ctx, late, []slots.Rule{weekly(t, day, 10, 12)}); err != nil {
		t.Fatal(err)
	}

	hosts := listHosts(t, mocks, viewer, "")
	tied := []string{firstHandle, secondHandle}
	slices.Sort(tied)
	if got := []string{hosts[0].Handle, hosts[1].Handle, hosts[2].Handle}; !slices.Equal(got, append(tied, lateHandle)) {
		t.Fatalf("order = %v, want %v then %s", got, tied, lateHandle)
	}
	if !hosts[0].NextSlot.Start.Equal(hosts[1].NextSlot.Start) || !hosts[1].NextSlot.Start.Before(hosts[2].NextSlot.Start) {
		t.Fatalf("next slots = %v, %v, %v", hosts[0].NextSlot.Start, hosts[1].NextSlot.Start, hosts[2].NextSlot.Start)
	}
	if earliest := time.Now().Add(buddy.MockMinimumNotice); hosts[0].NextSlot.Start.Before(earliest) {
		t.Fatalf("next slot %v is inside the notice period", hosts[0].NextSlot.Start)
	}
}

func TestMockHostsCarryTheMemberPageSummary(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	ctx := context.Background()
	viewer, _ := joined(t, pool, svc, "viewer@example.test")
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	openAllWeek(t, mocks, host, "UTC")
	id := seedMock(t, pool, host, guest, time.Now().Add(-2*time.Hour))
	if err := mocks.Review(ctx, guest, id, 4, "Good"); err != nil {
		t.Fatal(err)
	}
	if err := mocks.Review(ctx, host, id, 5, ""); err != nil {
		t.Fatal(err)
	}

	hosts := listHosts(t, mocks, viewer, "")
	if len(hosts) != 1 || hosts[0].Handle != hostHandle {
		t.Fatalf("hosts = %+v", hosts)
	}
	if s := hosts[0].Summary; s.Completed != 1 || s.RatingCount != 1 || s.RatingAvg != 4 || len(s.Recent) != 0 {
		t.Fatalf("summary = %+v, want one completed mock rated 4", s)
	}

	// A block between the review's author and its subject withdraws it, as on the member
	// page; the completed mock still counts.
	if _, err := pool.Exec(ctx, `INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1, $2)`, guest, host); err != nil {
		t.Fatal(err)
	}
	hosts = listHosts(t, mocks, viewer, "")
	if s := hosts[0].Summary; s.Completed != 1 || s.RatingCount != 0 || s.RatingAvg != 0 {
		t.Fatalf("summary across a block = %+v", s)
	}
	page, err := mocks.MemberSummary(ctx, viewer, hostHandle)
	if err != nil {
		t.Fatal(err)
	}
	if page.Completed != hosts[0].Summary.Completed || page.RatingCount != hosts[0].Summary.RatingCount {
		t.Fatalf("member page %+v and listing %+v disagree", page, hosts[0].Summary)
	}
}

func TestMockHostsRefuseANonMemberAndNameTheZone(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	ctx := context.Background()
	outsider := seedUser(t, pool, "outsider@example.test")
	if _, err := mocks.Hosts(ctx, outsider, "", "UTC"); !errors.Is(err, buddy.ErrNotMember) {
		t.Fatalf("non-member: err = %v", err)
	}
	viewer, _ := joined(t, pool, svc, "viewer@example.test")
	res, err := mocks.Hosts(ctx, viewer, "", "Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}
	if res.Zone != "Asia/Tokyo" || res.Hosts == nil || len(res.Hosts) != 0 {
		t.Fatalf("empty listing = %+v", res)
	}
}
