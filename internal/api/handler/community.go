package handler

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/engage/community"
	"github.com/strelov1/freehire/internal/platform/db"
)

// communityHandlers serves the anonymous discussion threads: topics attached to a
// company or vacancy, pseudonymous personas, flat replies. The use cases live in
// community.Service; the handlers translate wire ↔ domain and delegate to it.
type communityHandlers struct {
	community *community.Service
}

func newCommunityHandlers(queries *db.Queries) *communityHandlers {
	// One repository satisfies both the persistence port and the subject-existence
	// port, so it is passed for each.
	repo := community.NewQueriesRepository(queries)
	return &communityHandlers{community: community.New(repo, repo, community.Config{})}
}

func (h *communityHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// Community discussion threads: anonymous topics attached to a company or vacancy.
	// Reads are public — only pseudonymous persona handles are ever exposed, never a
	// user id — so discussions are browsable without signing in. Writing a thread or
	// reply requires a signed-in session (cookie); closing a thread is moderator-gated.
	api.Get("/threads", contract)
	// Registered before "/threads/:id" so "count" and "recent" are not parsed as a
	// thread id.
	api.Get("/threads/count", contract)
	api.Get("/threads/recent", contract)
	api.Get("/threads/:id", contract)
	api.Post("/threads", mw.cookie, contract)
	api.Post("/threads/:id/replies", mw.cookie, contract)
	api.Post("/threads/:id/close", mw.cookie, mw.moderator, contract)
}

// aiAuthor is the display name used for a reply flagged is_ai (a future AI-authored
// reply). No AI posts exist at MVP; this keeps the wire shape stable for when they do.
const aiAuthor = "AI"

// deletedAuthor is the display name for content whose author deleted their account.
// Threads and replies outlive their author (the FK nulls the reference), and the
// persona — the handle — goes with the account, so there is nothing left to show. The
// brackets keep it out of the handle namespace, which is "adjective-noun-NN".
const deletedAuthor = "[deleted]"

// personaOrDeleted is the author display name for a handle: an empty one means the
// persona is gone with its account, leaving the content de-authored.
func personaOrDeleted(handle string) string {
	if handle == "" {
		return deletedAuthor
	}
	return handle
}

func toThread(t community.Thread) apigen.Thread {
	return apigen.Thread{
		ID: t.ID, SubjectType: apigen.ThreadSubjectType(t.SubjectType), SubjectSlug: t.SubjectRef, Title: t.Title,
		Body: t.Body, Author: personaOrDeleted(t.AuthorHandle), ReplyCount: int64(t.ReplyCount),
		Status: apigen.ThreadStatus(t.Status), CreatedAt: t.CreatedAt,
	}
}

func toReply(r community.Reply) apigen.ThreadReply {
	author := personaOrDeleted(r.AuthorHandle)
	if r.IsAI {
		author = aiAuthor
	}
	return apigen.ThreadReply{
		ID: r.ID, ThreadID: r.ThreadID, ParentID: r.ParentID, Author: author, Body: r.Body, CreatedAt: r.CreatedAt,
	}
}

// encodeCursor packs a (created_at, id) keyset position into an opaque token.
func encodeCursor(t time.Time, id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(t.UnixNano(), 10) + ":" + strconv.FormatInt(id, 10)))
}

// decodeCursor parses a token from encodeCursor; an empty token is the first page.
func decodeCursor(s string) (community.Cursor, error) {
	if s == "" {
		return community.Cursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return community.Cursor{}, err
	}
	ns, idStr, ok := strings.Cut(string(raw), ":")
	if !ok {
		return community.Cursor{}, errors.New("malformed cursor")
	}
	nsN, err := strconv.ParseInt(ns, 10, 64)
	if err != nil {
		return community.Cursor{}, err
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return community.Cursor{}, err
	}
	return community.Cursor{CreatedAt: time.Unix(0, nsN), ID: id}, nil
}

// ListThreads returns a subject's open threads, newest first, keyset-paged. Public:
// discussions are browsable without signing in (only handles are exposed).
func (h *communityHandlers) ListThreads(ctx context.Context, params apigen.ListThreadsParams) (*apigen.ThreadPage, error) {
	cur, err := decodeCursor(params.Cursor.Value)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, "invalid cursor")
	}
	threads, err := h.community.ListThreads(ctx, params.SubjectType.Value, params.SubjectSlug.Value, cur)
	if err != nil {
		return nil, communityError(err)
	}
	out := make([]apigen.Thread, len(threads))
	for i, t := range threads {
		out[i] = toThread(t)
	}
	return &apigen.ThreadPage{Data: out, Meta: pageMeta(threads, h.community.PageSize())}, nil
}

// pageMeta carries a listing's continuation cursor, and only when the page came back
// FULL. A partial page is the last one, and a cursor on it tells the client a further
// page exists — which is how a single-thread subject rendered a "Load more" that
// fetched nothing.
//
// One function for all three community listings (subject threads, replies, the feed):
// the rule is one rule, and each row type reports its own keyset position. The page
// size is a parameter rather than read off the handler because a method cannot carry a
// type parameter in Go.
func pageMeta[T interface{ Position() community.Cursor }](items []T, pageSize int32) apigen.ThreadCursorMeta {
	var meta apigen.ThreadCursorMeta
	// >= not ==: a page longer than asked for is still a full one, and equality would
	// drop the cursor on it — the same silence, from the other side.
	if n := len(items); n > 0 && int32(n) >= pageSize {
		pos := items[n-1].Position()
		meta.NextCursor = apigen.NewOptString(encodeCursor(pos.CreatedAt, pos.ID))
	}
	return meta
}

// ListRecentThreads returns open threads across every subject, newest first,
// keyset-paged — the /discussions feed. Public, like the other thread reads. Each row
// carries the display name of its subject, which the cross-subject listing needs because
// a reader has no other way to know what the thread is about; both are empty when the
// subject no longer exists (there is no FK to it).
func (h *communityHandlers) ListRecentThreads(ctx context.Context, params apigen.ListRecentThreadsParams) (*apigen.RecentThreadPage, error) {
	cur, err := decodeCursor(params.Cursor.Value)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, "invalid cursor")
	}
	threads, err := h.community.ListRecentThreads(ctx, cur)
	if err != nil {
		return nil, communityError(err)
	}
	out := make([]apigen.RecentThread, len(threads))
	for i, t := range threads {
		th := toThread(t.Thread)
		out[i] = apigen.RecentThread{
			ID: th.ID, SubjectType: th.SubjectType, SubjectSlug: th.SubjectSlug, Title: th.Title, Body: th.Body,
			Author: th.Author, ReplyCount: th.ReplyCount, Status: th.Status, CreatedAt: th.CreatedAt,
			SubjectTitle: t.SubjectTitle, SubjectCompany: t.SubjectCompany,
		}
	}
	return &apigen.RecentThreadPage{Data: out, Meta: pageMeta(threads, h.community.PageSize())}, nil
}

// CountThreads returns a subject's open-thread count — the detail-page badge. Public.
func (h *communityHandlers) CountThreads(ctx context.Context, params apigen.CountThreadsParams) (*apigen.CountThreadsOK, error) {
	n, err := h.community.CountThreads(ctx, params.SubjectType.Value, params.SubjectSlug.Value)
	if err != nil {
		return nil, communityError(err)
	}
	return &apigen.CountThreadsOK{Data: apigen.ThreadCount{Count: n}}, nil
}

// GetThread returns a single thread with its first page of replies. Public.
func (h *communityHandlers) GetThread(ctx context.Context, params apigen.GetThreadParams) (*apigen.ThreadDetailPage, error) {
	id, err := contractPathID(params.ID)
	if err != nil {
		return nil, err
	}
	thread, err := h.community.GetThread(ctx, id)
	if err != nil {
		return nil, communityError(err)
	}
	cur, err := decodeCursor(params.Cursor.Value)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, "invalid cursor")
	}
	replies, err := h.community.ListReplies(ctx, id, cur)
	if err != nil {
		return nil, communityError(err)
	}
	out := make([]apigen.ThreadReply, len(replies))
	for i, r := range replies {
		out[i] = toReply(r)
	}
	return &apigen.ThreadDetailPage{
		Data: apigen.ThreadWithReplies{Thread: toThread(thread), Replies: out},
		Meta: pageMeta(replies, h.community.PageSize()),
	}, nil
}

// CreateThread opens a thread on a company or job. RequireAuth; 400/404/422/429.
func (h *communityHandlers) CreateThread(ctx context.Context, req *apigen.CreateThreadRequest) (*apigen.CreateThreadCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	thread, err := h.community.CreateThread(ctx, community.CreateThreadInput{
		UserID: userID, SubjectType: req.SubjectType.Value, SubjectSlug: req.SubjectSlug.Value,
		Title: req.Title.Value, Body: req.Body.Value,
	})
	if err != nil {
		return nil, communityError(err)
	}
	return &apigen.CreateThreadCreated{Data: toThread(thread)}, nil
}

// CreateReply posts a reply to an open thread. RequireAuth; 404/409/422/429.
func (h *communityHandlers) CreateReply(ctx context.Context, req *apigen.CreateReplyRequest, params apigen.CreateReplyParams) (*apigen.CreateReplyCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := contractPathID(params.ID)
	if err != nil {
		return nil, err
	}
	reply, err := h.community.Reply(ctx, id, req.ParentReplyID.Value, userID, req.Body.Value)
	if err != nil {
		return nil, communityError(err)
	}
	return &apigen.CreateReplyCreated{Data: toReply(reply)}, nil
}

// CloseThread closes a thread (moderator-gated at the route).
func (h *communityHandlers) CloseThread(ctx context.Context, params apigen.CloseThreadParams) (*apigen.CloseThreadOK, error) {
	id, err := contractPathID(params.ID)
	if err != nil {
		return nil, err
	}
	if err := h.community.Close(ctx, id); err != nil {
		return nil, communityError(err)
	}
	return &apigen.CloseThreadOK{Data: apigen.ThreadClosed{ID: id, Status: community.StatusClosed}}, nil
}

// communityError maps community sentinel errors to HTTP statuses; anything else falls
// through to RenderError's 500.
func communityError(err error) error {
	switch {
	case errors.Is(err, community.ErrUnsupportedSubject):
		return fiber.NewError(fiber.StatusBadRequest, "unsupported subject type")
	case errors.Is(err, community.ErrSubjectNotFound):
		return fiber.NewError(fiber.StatusNotFound, "subject not found")
	case errors.Is(err, community.ErrThreadNotFound):
		return fiber.NewError(fiber.StatusNotFound, "thread not found")
	case errors.Is(err, community.ErrThreadClosed):
		return fiber.NewError(fiber.StatusConflict, "thread is closed")
	case errors.Is(err, community.ErrInvalidParentReply):
		return fiber.NewError(fiber.StatusBadRequest, "parent reply does not belong to this thread")
	case errors.Is(err, community.ErrEmptyTitle):
		return fiber.NewError(fiber.StatusUnprocessableEntity, "title is required")
	case errors.Is(err, community.ErrEmptyBody):
		return fiber.NewError(fiber.StatusUnprocessableEntity, "body is required")
	default:
		return err
	}
}
