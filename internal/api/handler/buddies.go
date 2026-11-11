package handler

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/engage/buddy"
)

// buddyHandlers serves the interview-buddy network (internal/engage/buddy): membership
// under a generated handle, similar members, the feed of other members' search progress,
// and the invitation that starts a chat. The chat itself is the messaging core's
// (conversationHandlers). Every route is cookie-only, so a leaked API key cannot act for a
// member. The routes implement the contract (typespec/buddies.tsp).
type buddyHandlers struct {
	buddy *buddy.Service
	mocks *buddy.MockService
}

func newBuddyHandlers(svc *buddy.Service, mocks *buddy.MockService) *buddyHandlers {
	return &buddyHandlers{buddy: svc, mocks: mocks}
}

func (h *buddyHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	api.Get("/me/buddies", mw.cookie, contract)
	api.Post("/me/buddies", mw.cookie, contract)
	api.Delete("/me/buddies", mw.cookie, contract)
	api.Get("/me/buddies/similar", mw.cookie, contract)
	api.Get("/me/buddies/feed", mw.cookie, contract)
	api.Put("/me/buddies/feed/:id/reaction", mw.cookie, contract)
	api.Delete("/me/buddies/feed/:id/reaction", mw.cookie, contract)
	api.Get("/me/buddies/profiles/:handle", mw.cookie, contract)
	api.Get("/me/buddies/profiles/:handle/activity", mw.cookie, contract)
	api.Get("/me/buddies/hidden-applications", mw.cookie, contract)
	api.Put("/me/buddies/hidden-applications/:id", mw.cookie, contract)
	api.Delete("/me/buddies/hidden-applications/:id", mw.cookie, contract)
	api.Post("/me/buddies/invite", mw.cookie, contract)
	h.registerMocks(api, mw, contract)
}

// buddyError renders a buddy sentinel as the status its declaration documents, and a
// messaging one as messagingError does. Anything else passes through to the error
// renderer, which answers 500.
func buddyError(err error) error {
	switch {
	case errors.Is(err, buddy.ErrNotMember):
		return fiber.NewError(fiber.StatusForbidden, "join the buddy network first")
	case errors.Is(err, buddy.ErrRecipientUnavailable):
		return fiber.NewError(fiber.StatusForbidden, "this member cannot be invited")
	case errors.Is(err, buddy.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, "not found")
	case errors.Is(err, buddy.ErrBadFeedRequest):
		return fiber.NewError(fiber.StatusBadRequest, "invalid feed mode or cursor")
	case errors.Is(err, buddy.ErrBadReaction):
		return fiber.NewError(fiber.StatusBadRequest, "unknown reaction")
	}
	return messagingError(err)
}

type buddyCardSummaryResponse struct {
	Category  string `json:"category"`
	Seniority string `json:"seniority"`
	Country   string `json:"country"`
}

func toBuddyCardSummary(c buddy.CardSummary) buddyCardSummaryResponse {
	return buddyCardSummaryResponse{Category: c.Category, Seniority: c.Seniority, Country: c.Country}
}

func buddyCardSummaryOut(c buddy.CardSummary) apigen.BuddyCardSummary {
	return apigen.BuddyCardSummary{Category: c.Category, Seniority: c.Seniority, Country: c.Country}
}

// buddyStatusResponse is the status the MCP tools answer; the routes answer its apigen twin.
type buddyStatusResponse struct {
	Member    bool   `json:"member"`
	Handle    string `json:"handle,omitempty"`
	CardEmpty bool   `json:"card_empty"`
}

func (h *buddyHandlers) statusOut(ctx context.Context, userID int64) (apigen.BuddyStatus, error) {
	st, err := h.status(ctx, userID)
	if err != nil {
		return apigen.BuddyStatus{}, err
	}
	return apigen.BuddyStatus{Member: st.Member, Handle: optString(st.Handle), CardEmpty: st.CardEmpty}, nil
}

// BuddyStatus reports the caller's standing. A member's visit also refreshes their ranking
// keys; no other route does, because the chat polls and a write per poll would be wrong.
func (h *buddyHandlers) BuddyStatus(ctx context.Context) (*apigen.BuddyStatusOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.buddy.Touch(ctx, userID); err != nil && !errors.Is(err, buddy.ErrNotMember) {
		return nil, err
	}
	st, err := h.statusOut(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &apigen.BuddyStatusOK{Data: st}, nil
}

// BuddyJoin opts the caller in, or restores a returning member under their old handle.
func (h *buddyHandlers) BuddyJoin(ctx context.Context) (*apigen.BuddyJoinOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := h.buddy.Join(ctx, userID); err != nil {
		return nil, err
	}
	st, err := h.statusOut(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &apigen.BuddyJoinOK{Data: st}, nil
}

// BuddyLeave opts the caller out and closes their conversations.
func (h *buddyHandlers) BuddyLeave(ctx context.Context) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	if err := h.buddy.Leave(ctx, userID); err != nil {
		return buddyError(err)
	}
	// The leave cancelled the member's future mocks in its own statement; this tells the
	// other participants.
	h.mocks.AnnounceCancellations(ctx, userID)
	return nil
}

// BuddySimilar lists the members most like the caller.
func (h *buddyHandlers) BuddySimilar(ctx context.Context, _ apigen.BuddySimilarParams) (*apigen.BuddySimilarPage, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	limit, offset, err := contractPageWindowed(ctx, buddy.DefaultSimilarLimit, buddy.MaxSimilarLimit)
	if err != nil {
		return nil, err
	}
	members, err := h.buddy.Similar(ctx, userID, limit, offset)
	if err != nil {
		return nil, buddyError(err)
	}
	out := make([]apigen.BuddySimilar, len(members))
	for i, m := range members {
		shared := make([]apigen.BuddyCompany, len(m.Shared))
		for j, co := range m.Shared {
			shared[j] = apigen.BuddyCompany{Slug: co.Slug, Name: co.Name}
		}
		out[i] = apigen.BuddySimilar{Handle: m.Handle, CardSummary: buddyCardSummaryOut(m.Card), Score: int64(m.Score), SharedCompanies: shared}
	}
	return &apigen.BuddySimilarPage{Data: out, Meta: apigen.BuddySimilarMeta{Limit: int64(limit), Offset: int64(offset)}}, nil
}

// BuddyFeed reads one page of other members' search progress. With ?handle= it reads that one
// member's events instead (their page's activity), and mode is not read.
func (h *buddyHandlers) BuddyFeed(ctx context.Context, params apigen.BuddyFeedParams) (*apigen.BuddyFeedPage, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	limit := queryIntOr(contractQuery(ctx), "limit", 0)
	var page buddy.FeedPage
	if handle := params.Handle.Value; handle != "" {
		page, err = h.buddy.MemberFeed(ctx, userID, handle, params.Cursor.Value, limit)
	} else {
		mode := buddy.FeedMode(params.Mode.Value)
		if mode == "" {
			mode = buddy.FeedSimilar
		}
		page, err = h.buddy.Feed(ctx, userID, mode, params.Cursor.Value, limit)
	}
	if err != nil {
		return nil, buddyError(err)
	}
	out := make([]apigen.BuddyFeedEntry, len(page.Entries))
	for i, e := range page.Entries {
		out[i] = apigen.BuddyFeedEntry{
			ID: e.ID, Handle: e.Handle, CardSummary: buddyCardSummaryOut(e.Card),
			Company:  apigen.BuddyCompany{Slug: e.Company.Slug, Name: e.Company.Name},
			JobTitle: nilString(e.JobTitle), JobSlug: nilString(e.JobSlug), JobLocation: nilString(e.JobLocation),
			Reactions: buddyReactionsOut(e.Reactions),
			Type:      e.Type, Day: e.Day,
		}
	}
	var next *string
	if page.NextCursor != "" {
		next = &page.NextCursor
	}
	return &apigen.BuddyFeedPage{Data: out, Meta: apigen.BuddyFeedMeta{NextCursor: nilString(next)}}, nil
}

// BuddyProfile shows one member's anonymous card.
func (h *buddyHandlers) BuddyProfile(ctx context.Context, params apigen.BuddyProfileParams) (*apigen.BuddyProfileOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	p, err := h.buddy.Profile(ctx, userID, params.Handle)
	if err != nil {
		return nil, buddyError(err)
	}
	return &apigen.BuddyProfileOK{Data: apigen.BuddyProfile{
		Handle: p.Handle, CardSummary: buddyCardSummaryOut(p.Summary), Card: talentCardOut(p.Card), CardEmpty: p.EmptyCard,
	}}, nil
}

// BuddyActivity is one member's feed events per day over the last year, for their page's grid.
func (h *buddyHandlers) BuddyActivity(ctx context.Context, params apigen.BuddyActivityParams) (*apigen.BuddyActivityOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	days, err := h.buddy.MemberActivity(ctx, userID, params.Handle)
	if err != nil {
		return nil, buddyError(err)
	}
	out := make([]apigen.BuddyActivityDay, len(days))
	for i, d := range days {
		out[i] = apigen.BuddyActivityDay{
			Day: d.Day, Count: int64(d.Count), Applied: int64(d.Applied), Interview: int64(d.Interview), Offer: int64(d.Offer),
		}
	}
	return &apigen.BuddyActivityOK{Data: apigen.BuddyActivity{Days: out}}, nil
}

// BuddyHiddenApplications lists the caller's applications kept out of the feed.
func (h *buddyHandlers) BuddyHiddenApplications(ctx context.Context) (*apigen.BuddyHiddenApplicationsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	ids, err := h.buddy.HiddenApplications(ctx, userID)
	if err != nil {
		return nil, buddyError(err)
	}
	if ids == nil {
		ids = []int64{}
	}
	return &apigen.BuddyHiddenApplicationsOK{Data: ids}, nil
}

// BuddyHideApplication keeps one of the caller's applications out of the feed.
func (h *buddyHandlers) BuddyHideApplication(ctx context.Context, params apigen.BuddyHideApplicationParams) error {
	return h.byID(ctx, params.ID, h.buddy.HideApplication)
}

// BuddyShowApplication lifts a hide.
func (h *buddyHandlers) BuddyShowApplication(ctx context.Context, params apigen.BuddyShowApplicationParams) error {
	return h.byID(ctx, params.ID, h.buddy.ShowApplication)
}

// byID runs an action on the caller and the id path param.
func (h *buddyHandlers) byID(ctx context.Context, rawID string, do func(ctx context.Context, userID, id int64) error) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	id, err := contractPathID(rawID)
	if err != nil {
		return err
	}
	return buddyError(do(ctx, userID, id))
}

// BuddyInvite starts a pending buddy conversation.
func (h *buddyHandlers) BuddyInvite(ctx context.Context, req *apigen.BuddyInviteBody) (*apigen.BuddyInviteCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if req.Handle.Value == "" {
		return nil, fiber.NewError(fiber.StatusBadRequest, "handle and body are required")
	}
	conv, err := h.buddy.Invite(ctx, userID, req.Handle.Value, req.Body.Value)
	if err != nil {
		return nil, buddyError(err)
	}
	out := apigen.BuddyConversation{
		ID: conv.ID, Kind: conv.Kind, StatusForMe: conv.StatusForMe, Unread: int64(conv.Unread), CreatedAt: conv.CreatedAt,
		Counterpart: apigen.BuddyConversationCounterpart{Persona: conv.Counterpart.Persona, Reachable: conv.Counterpart.Reachable},
	}
	if d, ok := conv.Counterpart.Detail.(buddy.Counterpart); ok {
		out.Counterpart.Detail.SetTo(apigen.BuddyCounterpartDetail{Handle: d.Handle, CardSummary: buddyCardSummaryOut(d.Card)})
	} else {
		out.Counterpart.Detail.SetToNull()
	}
	if conv.LastMessageAt.IsZero() {
		out.LastMessageAt.SetToNull()
	} else {
		out.LastMessageAt.SetTo(conv.LastMessageAt)
	}
	return &apigen.BuddyInviteCreated{Data: out}, nil
}

// buddyReactionsOut is one feed event's reactions: count per kind (no key for a kind
// nobody chose — always an object, never null) and the viewer's own choice, "" for none.
func buddyReactionsOut(r buddy.Reactions) apigen.BuddyReactions {
	counts := make(apigen.BuddyReactionsCounts, len(r.Counts))
	for kind, n := range r.Counts {
		counts[kind] = int64(n)
	}
	return apigen.BuddyReactions{Counts: counts, Mine: r.Mine}
}

// BuddyReact sets the caller's one reaction to a feed event they can see, replacing any other.
func (h *buddyHandlers) BuddyReact(ctx context.Context, req *apigen.BuddyReactBody, params apigen.BuddyReactParams) (*apigen.BuddyReactOK, error) {
	if req.Kind.Value == "" {
		return nil, fiber.NewError(fiber.StatusBadRequest, "kind is required")
	}
	r, err := h.react(ctx, params.ID, req.Kind.Value)
	if err != nil {
		return nil, err
	}
	return &apigen.BuddyReactOK{Data: r}, nil
}

// BuddyUnreact removes the caller's reaction to a feed event.
func (h *buddyHandlers) BuddyUnreact(ctx context.Context, params apigen.BuddyUnreactParams) (*apigen.BuddyUnreactOK, error) {
	r, err := h.react(ctx, params.ID, "")
	if err != nil {
		return nil, err
	}
	return &apigen.BuddyUnreactOK{Data: r}, nil
}

func (h *buddyHandlers) react(ctx context.Context, rawID, kind string) (apigen.BuddyReactions, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return apigen.BuddyReactions{}, err
	}
	id, err := contractPathID(rawID)
	if err != nil {
		return apigen.BuddyReactions{}, err
	}
	r, err := h.buddy.React(ctx, userID, id, kind)
	if err != nil {
		return apigen.BuddyReactions{}, buddyError(err)
	}
	return buddyReactionsOut(r), nil
}
