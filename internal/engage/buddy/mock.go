package buddy

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/engage/slots"
)

// The shape of every mock interview (design D2). Constants rather than columns: a peer
// mock has one shape, and per-member tuning is a mentor feature.
const (
	// MockDuration is how long a mock runs. The participants swap roles halfway themselves.
	MockDuration = 60 * time.Minute
	// MockMinimumNotice is how far ahead of now the earliest bookable slot begins.
	MockMinimumNotice = 12 * time.Hour
	// MockHorizon is how far ahead of now the latest bookable slot begins.
	MockHorizon = 30 * 24 * time.Hour
	// maxMeetingURLRunes bounds a meeting link, matching buddy_mock_settings_url_check.
	maxMeetingURLRunes = 500
)

// mockSession is that shape as the engine takes it. No buffers: mocks may run back to back.
var mockSession = slots.SessionParams{Duration: MockDuration, MinimumNotice: MockMinimumNotice, Horizon: MockHorizon}

// The topics a host may offer; they match buddy_mock_settings_topics_check and
// buddy_mocks_topic_check.
const (
	TopicCoding       = "coding"
	TopicSystemDesign = "system_design"
	TopicBehavioral   = "behavioral"
)

// MockTopics is the topic vocabulary in display order; saved settings keep this order.
var MockTopics = []string{TopicCoding, TopicSystemDesign, TopicBehavioral}

// The statuses a mock holds. There is no "completed": that is confirmed and ended.
const (
	MockConfirmed = "confirmed"
	MockCancelled = "cancelled"
)

// The two roles a participant holds in one mock.
const (
	RoleHost  = "host"
	RoleGuest = "guest"
)

// Why a mock was cancelled; buddy_mocks.cancel_reason. A leave or a block is told to the
// other side only as "the mock was cancelled".
const (
	CancelByParticipant = "participant"
	CancelByLeave       = "left"
	CancelByBlock       = "blocked"
)

// ErrInvalidMockSettings → 422: a zone that is not an IANA place, a link that is not
// https://, or no known topic.
var ErrInvalidMockSettings = errors.New("buddy: invalid mock settings")

// MockSettings is what a member offers as a mock host.
type MockSettings struct {
	// Timezone is the IANA zone their availability is stated in.
	Timezone string
	// MeetingURL is their own call room. Only the two participants of a confirmed mock
	// ever read it, and they read the current one.
	MeetingURL string
	// Topics is a non-empty subset of MockTopics, in its order.
	Topics []string
	// Paused withdraws every slot and leaves booked mocks standing.
	Paused bool
}

// Mock is one booked mock interview as the service reads it. The two addresses exist to
// address each participant's OWN notice; nothing that reaches the other participant
// carries either of them, or either account id.
type Mock struct {
	ID            uuid.UUID
	HostUserID    int64
	GuestUserID   int64
	HostHandle    string
	GuestHandle   string
	StartsAt      time.Time
	EndsAt        time.Time
	Topic         string
	Status        string
	CancelReason  string
	HostTimezone  string
	GuestTimezone string
	MeetingURL    string
	HostEmail     string
	GuestEmail    string
}

// validateMockSettings refuses settings no slot or notice could be computed from, and
// normalises the rest: the link trimmed, the topics deduplicated into vocabulary order.
func validateMockSettings(in MockSettings) (MockSettings, error) {
	if _, ok := ianaZone(in.Timezone); !ok {
		return MockSettings{}, fmt.Errorf("%w: %q is not an IANA time zone", ErrInvalidMockSettings, in.Timezone)
	}
	link := strings.TrimSpace(in.MeetingURL)
	parsed, err := url.Parse(link)
	if !strings.HasPrefix(link, "https://") || err != nil || parsed.Host == "" ||
		utf8.RuneCountInString(link) > maxMeetingURLRunes {
		return MockSettings{}, fmt.Errorf("%w: the meeting link must be an https:// address", ErrInvalidMockSettings)
	}
	for _, t := range in.Topics {
		if !slices.Contains(MockTopics, t) {
			return MockSettings{}, fmt.Errorf("%w: unknown topic %q", ErrInvalidMockSettings, t)
		}
	}
	topics := make([]string, 0, len(MockTopics))
	for _, t := range MockTopics {
		if slices.Contains(in.Topics, t) {
			topics = append(topics, t)
		}
	}
	if len(topics) == 0 {
		return MockSettings{}, fmt.Errorf("%w: choose at least one topic", ErrInvalidMockSettings)
	}
	return MockSettings{Timezone: in.Timezone, MeetingURL: link, Topics: topics, Paused: in.Paused}, nil
}

// ianaZone loads an IANA zone name, refusing what time.LoadLocation accepts but is not a
// place: "Local" is the SERVER's zone, and "EST" or "Factory" are not where anybody lives.
// Requiring a slash, or exactly "UTC", is the rule mentorship's validateMentorZone states.
func ianaZone(name string) (*time.Location, bool) {
	if name != "UTC" && !strings.Contains(name, "/") {
		return nil, false
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, false
	}
	return loc, true
}

// mockSlots runs the shared engine with the mock shape.
func mockSlots(rules []slots.Rule, hostZone *time.Location, busy []slots.Interval, from, to, now time.Time, viewerZone string) (slots.SlotResult, error) {
	return slots.Slots(slots.SlotRequest{
		Rules:      rules,
		OwnerZone:  hostZone,
		Params:     mockSession,
		Busy:       busy,
		From:       from,
		To:         to,
		Now:        now,
		ViewerZone: viewerZone,
	})
}
