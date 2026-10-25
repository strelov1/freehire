package buddy

import (
	"context"
	"errors"
	"time"

	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/candidate/talentnetwork"
	"github.com/strelov1/freehire/internal/dict/classify"
)

var (
	// ErrNotMember is the answer to every buddy action asked by someone who has not
	// joined, or has left.
	ErrNotMember = errors.New("buddy: not an active member")
	// ErrNotFound means a handle names no active member. A member who has left is
	// indistinguishable from one who never existed.
	ErrNotFound = errors.New("buddy: member not found")
	// ErrHandleTaken is returned by a Repository when a minted handle collides with
	// another member's. The service retries with a fresh handle.
	ErrHandleTaken = errors.New("buddy: handle already taken")
)

// maxHandleAttempts bounds the retry on a handle collision. The space is
// |adj| x |noun| x 90, so a handful of attempts makes exhaustion a non-event.
const maxHandleAttempts = 8

// Member is one account's place in the network.
type Member struct {
	UserID    int64
	Handle    string
	Category  string
	Countries []string
	Regions   []string
	JoinedAt  time.Time
	// Active is false for a member who has left; the row and handle are kept.
	Active bool
}

// Keys are the values similarity is scored on.
type Keys struct {
	Category  string
	Countries []string
	Regions   []string
}

// Inputs is everything the ranking keys and the card are derived from. Countries and
// Regions are nil when the CV's geography is unknown, which differs from empty (a CV
// that named a place the dictionary could not resolve).
type Inputs struct {
	CV              resumeextract.Structured
	Countries       []string
	Regions         []string
	Specializations []string
	BaseCountry     string
}

// Repository is the storage the Service needs.
type Repository interface {
	// Join creates the member or reactivates a returning one, keeping the stored handle.
	// It returns ErrHandleTaken when handle collides with another member's.
	Join(ctx context.Context, userID int64, handle string, keys Keys) (Member, error)
	// Leave deactivates an active member and closes their pending and open
	// conversations. ok is false when the user was not an active member.
	Leave(ctx context.Context, userID int64) (ok bool, err error)
	Member(ctx context.Context, userID int64) (Member, bool, error)
	MemberByHandle(ctx context.Context, handle string) (Member, bool, error)
	SetKeys(ctx context.Context, userID int64, keys Keys) error
	Inputs(ctx context.Context, userID int64) (Inputs, error)
	// Similar is one page of the viewer's similarity ranking, best first.
	Similar(ctx context.Context, userID int64, limit, offset int) ([]SimilarRow, error)
	// Feed is up to limit rows, newest first, starting at the event cursorID (0 for the
	// first page; the cursor's own event is included when the viewer may see it). When
	// similarOnly is set only the authors named by handle are read.
	Feed(ctx context.Context, viewerID int64, authors []string, similarOnly bool, cursorID int64, limit int) ([]FeedRow, error)
	// HideApplication reports false when the application is not the user's.
	HideApplication(ctx context.Context, userID, applicationID int64) (bool, error)
	ShowApplication(ctx context.Context, userID, applicationID int64) error
	HiddenApplications(ctx context.Context, userID int64) ([]int64, error)
	// Blocked reports a block between the two users in either direction.
	Blocked(ctx context.Context, a, b int64) (bool, error)
	// PairHasConversation reports a conversation between the two users in any status.
	PairHasConversation(ctx context.Context, a, b int64) (bool, error)
	// InvitationsSince is how many invitations userID sent after since, and when the
	// oldest of them was sent (the zero time when there are none).
	InvitationsSince(ctx context.Context, userID int64, since time.Time) (sent int, oldest time.Time, err error)
	// CreateInvitation inserts a pending conversation and its first message atomically.
	// It returns ErrConversationExists when the pair already has one.
	CreateInvitation(ctx context.Context, from, to int64, body string) (ConversationRow, error)
	// AnswerInvitation moves a pending conversation to status (open or declined) on behalf
	// of the member who did not send it. It returns ErrNotFound when the caller is not a
	// party to the conversation and ErrConversationState when it is not pending or the
	// caller sent it.
	AnswerInvitation(ctx context.Context, userID, conversationID int64, status string) (ConversationRow, error)
	// Conversation reads one conversation by id, whoever it belongs to.
	Conversation(ctx context.Context, conversationID int64) (ConversationRow, bool, error)
	// InsertMessage stores a message from userID into an open conversation they belong to
	// and moves their read cursor onto it, guarding the status in the insert itself. It
	// returns ErrNotFound when userID is not a party and ErrConversationState when the
	// conversation is not open.
	InsertMessage(ctx context.Context, userID, conversationID int64, body string) (MessageRow, error)
	// Messages is up to limit messages with an id above after, oldest first.
	Messages(ctx context.Context, conversationID, after int64, limit int) ([]MessageRow, error)
	// MarkRead advances userID's read cursor to upTo, never back and never past the newest
	// message. It returns ErrNotFound when userID is not a party.
	MarkRead(ctx context.Context, userID, conversationID, upTo int64) error
	// Conversations lists every conversation userID may see, newest message first.
	Conversations(ctx context.Context, userID int64) ([]ConversationListRow, error)
}

// Service is the membership and profile use cases.
type Service struct {
	repo Repository
	now  func() time.Time
}

// New builds a Service.
func New(repo Repository) *Service { return &Service{repo: repo, now: time.Now} }

// Status is a caller's own standing in the network.
type Status struct {
	Member    bool
	Handle    string
	EmptyCard bool
}

// Profile is what one member sees of another: the handle and the dictionary-only card.
type Profile struct {
	Handle    string
	Card      talentnetwork.CandidateCard
	EmptyCard bool
}

// Join opts the caller in under a generated handle, or brings a returning member back
// under the handle they already had.
func (s *Service) Join(ctx context.Context, userID int64) (Member, error) {
	in, err := s.repo.Inputs(ctx, userID)
	if err != nil {
		return Member{}, err
	}
	keys := rankingKeys(in)
	for range maxHandleAttempts {
		m, err := s.repo.Join(ctx, userID, GenerateHandle(), keys)
		if errors.Is(err, ErrHandleTaken) {
			continue
		}
		return m, err
	}
	return Member{}, ErrHandleTaken
}

// Leave opts the caller out and closes every conversation they are in.
func (s *Service) Leave(ctx context.Context, userID int64) error {
	ok, err := s.repo.Leave(ctx, userID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotMember
	}
	return nil
}

// Status reports whether the caller is an active member, their handle, and whether the
// card others would see is empty. A non-member gets the zero Status, not an error.
func (s *Service) Status(ctx context.Context, userID int64) (Status, error) {
	m, ok, err := s.repo.Member(ctx, userID)
	if err != nil || !ok || !m.Active {
		return Status{}, err
	}
	in, err := s.repo.Inputs(ctx, userID)
	if err != nil {
		return Status{}, err
	}
	return Status{Member: true, Handle: m.Handle, EmptyCard: cardIsEmpty(talentnetwork.ProjectCard(in.CV))}, nil
}

// Touch refreshes an active member's ranking keys from their current CV and profile.
// A caller who is not an active member gets ErrNotMember.
func (s *Service) Touch(ctx context.Context, userID int64) error {
	m, ok, err := s.repo.Member(ctx, userID)
	if err != nil {
		return err
	}
	if !ok || !m.Active {
		return ErrNotMember
	}
	in, err := s.repo.Inputs(ctx, userID)
	if err != nil {
		return err
	}
	return s.repo.SetKeys(ctx, userID, rankingKeys(in))
}

// Profile returns the anonymous profile behind handle for an active member viewing it.
func (s *Service) Profile(ctx context.Context, viewerID int64, handle string) (Profile, error) {
	viewer, ok, err := s.repo.Member(ctx, viewerID)
	if err != nil {
		return Profile{}, err
	}
	if !ok || !viewer.Active {
		return Profile{}, ErrNotMember
	}
	target, ok, err := s.repo.MemberByHandle(ctx, handle)
	if err != nil {
		return Profile{}, err
	}
	if !ok || !target.Active {
		return Profile{}, ErrNotFound
	}
	in, err := s.repo.Inputs(ctx, target.UserID)
	if err != nil {
		return Profile{}, err
	}
	card := talentnetwork.ProjectCard(in.CV)
	return Profile{Handle: target.Handle, Card: card, EmptyCard: cardIsEmpty(card)}, nil
}

// rankingKeys derives the similarity keys. Category is what the primary role title
// resolves to, falling back to the first specialization on the profile; geography is
// the CV's, falling back to the profile's base country when the CV's is unknown (nil).
func rankingKeys(in Inputs) Keys {
	k := Keys{Category: classify.Parse(talentnetwork.PrimaryTitle(in.CV)).Category}
	if k.Category == "" && len(in.Specializations) > 0 {
		k.Category = in.Specializations[0]
	}
	if in.Countries != nil || in.Regions != nil {
		k.Countries, k.Regions = in.Countries, in.Regions
	} else if in.BaseCountry != "" {
		k.Countries = []string{in.BaseCountry}
	}
	return k
}

func cardIsEmpty(c talentnetwork.CandidateCard) bool {
	return c.Category == "" && c.Seniority == "" && c.TotalYears == 0 && len(c.Skills) == 0 &&
		len(c.Roles) == 0 && len(c.Education) == 0 && len(c.Certifications) == 0
}
