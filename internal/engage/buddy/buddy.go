package buddy

import (
	"context"
	"errors"
	"time"

	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/candidate/talentnetwork"
	"github.com/strelov1/freehire/internal/dict/classify"
	"github.com/strelov1/freehire/internal/engage/messaging"
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
// |adj| x |noun| x 9990, so a handful of attempts makes exhaustion a non-event.
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
	// Leave deactivates an active member and closes their pending and open buddy
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
	// authorsOnly is set only the authors named by handle are read.
	Feed(ctx context.Context, viewerID int64, authors []string, authorsOnly bool, cursorID int64, limit int) ([]FeedRow, error)
	// Activity counts authorID's feed events per UTC day from since on, under the same
	// visibility rules as Feed for viewerID. Days without an event are absent.
	Activity(ctx context.Context, viewerID, authorID int64, since time.Time) ([]ActivityDay, error)
	// HideApplication reports false when the application is not the user's.
	HideApplication(ctx context.Context, userID, applicationID int64) (bool, error)
	ShowApplication(ctx context.Context, userID, applicationID int64) error
	HiddenApplications(ctx context.Context, userID int64) ([]int64, error)
	// Blocked reports a block between the two users in either direction. Blocks are the
	// messaging core's and account-wide.
	Blocked(ctx context.Context, a, b int64) (bool, error)
	// Counterparts reads how each of userIDs is drawn in viewerID's conversation list.
	Counterparts(ctx context.Context, viewerID int64, userIDs []int64) ([]CounterpartRow, error)
	// Handles is the handle behind each account, active or left.
	Handles(ctx context.Context, userIDs []int64) (map[int64]string, error)
}

// Service is the membership, profile and invitation use cases. Conversations themselves are
// the messaging core's: an invitation is a conversation of kind "buddy" started through it.
type Service struct {
	repo Repository
	msg  *messaging.Service
	now  func() time.Time
}

// New builds a Service. msg must be built with this package's Persona.
func New(repo Repository, msg *messaging.Service) *Service {
	return &Service{repo: repo, msg: msg, now: time.Now}
}

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
	target, err := s.visibleMember(ctx, viewerID, handle)
	if err != nil {
		return Profile{}, err
	}
	in, err := s.repo.Inputs(ctx, target.UserID)
	if err != nil {
		return Profile{}, err
	}
	card := talentnetwork.ProjectCard(in.CV)
	return Profile{Handle: target.Handle, Card: card, EmptyCard: cardIsEmpty(card)}, nil
}

// visibleMember resolves the member behind handle for an active viewer. An unknown
// handle, a member who left, a blocked pair in either direction and the viewer's own
// handle all read as ErrNotFound, so none of them can be told apart from outside.
func (s *Service) visibleMember(ctx context.Context, viewerID int64, handle string) (Member, error) {
	if err := s.requireActive(ctx, viewerID); err != nil {
		return Member{}, err
	}
	target, ok, err := s.repo.MemberByHandle(ctx, handle)
	if err != nil {
		return Member{}, err
	}
	if !ok || !target.Active || target.UserID == viewerID {
		return Member{}, ErrNotFound
	}
	blocked, err := s.repo.Blocked(ctx, viewerID, target.UserID)
	if err != nil {
		return Member{}, err
	}
	if blocked {
		return Member{}, ErrNotFound
	}
	return target, nil
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
