package handler

import (
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

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

func (h *communityHandlers) register(api fiber.Router, mw middleware) {
	// Community discussion threads: anonymous topics attached to a company or vacancy.
	// Reads are public — only pseudonymous persona handles are ever exposed, never a
	// user id — so discussions are browsable without signing in. Writing a thread or
	// reply requires a signed-in session (cookie); closing a thread is moderator-gated.
	api.Get("/threads", h.ListThreads)
	// Registered before "/threads/:id" so "count" and "recent" are not parsed as a
	// thread id.
	api.Get("/threads/count", h.CountThreads)
	api.Get("/threads/recent", h.ListRecentThreads)
	api.Get("/threads/:id", h.GetThread)
	api.Post("/threads", mw.cookie, h.CreateThread)
	api.Post("/threads/:id/replies", mw.cookie, h.CreateReply)
	api.Post("/threads/:id/close", mw.cookie, mw.moderator, h.CloseThread)
}

// aiAuthor is the display name used for a reply flagged is_ai (a future AI-authored
// reply). No AI posts exist at MVP; this keeps the wire shape stable for when they do.
const aiAuthor = "AI"

// deletedAuthor is the display name for content whose author deleted their account.
// Threads and replies outlive their author (the FK nulls the reference), and the
// persona — the handle — goes with the account, so there is nothing left to show. The
// brackets keep it out of the handle namespace, which is "adjective-noun-NN".
const deletedAuthor = "[deleted]"

// threadResponse is the public shape of a thread: the persona handle is the only
// author identity — the author's user_id is never projected here. A thread whose
// author deleted their account carries the deleted-member marker instead.
type threadResponse struct {
	ID          int64     `json:"id"`
	SubjectType string    `json:"subject_type"`
	SubjectSlug string    `json:"subject_slug"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	Author      string    `json:"author"`
	ReplyCount  int32     `json:"reply_count"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

// replyResponse is the public shape of a reply: the persona handle, the AI persona for
// a system reply, or the deleted-member marker once the author's account is gone.
type replyResponse struct {
	ID        int64     `json:"id"`
	ThreadID  int64     `json:"thread_id"`
	ParentID  int64     `json:"parent_id"`
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// personaOrDeleted is the author display name for a handle: an empty one means the
// persona is gone with its account, leaving the content de-authored.
func personaOrDeleted(handle string) string {
	if handle == "" {
		return deletedAuthor
	}
	return handle
}

func toThreadResponse(t community.Thread) threadResponse {
	return threadResponse{
		ID: t.ID, SubjectType: t.SubjectType, SubjectSlug: t.SubjectRef, Title: t.Title,
		Body: t.Body, Author: personaOrDeleted(t.AuthorHandle), ReplyCount: t.ReplyCount,
		Status: t.Status, CreatedAt: t.CreatedAt,
	}
}

func toReplyResponse(r community.Reply) replyResponse {
	author := personaOrDeleted(r.AuthorHandle)
	if r.IsAI {
		author = aiAuthor
	}
	return replyResponse{
		ID: r.ID, ThreadID: r.ThreadID, ParentID: r.ParentID, Author: author, Body: r.Body, CreatedAt: r.CreatedAt,
	}
}

type createThreadBody struct {
	SubjectType string `json:"subject_type"`
	SubjectSlug string `json:"subject_slug"`
	Title       string `json:"title"`
	Body        string `json:"body"`
}

type createReplyBody struct {
	Body string `json:"body"`
	// ParentReplyID nests this reply under another reply; 0/omitted = top-level.
	ParentReplyID int64 `json:"parent_reply_id"`
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
func (h *communityHandlers) ListThreads(c *fiber.Ctx) error {
	q := queryValues(c)
	cur, err := decodeCursor(q.Get("cursor"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid cursor")
	}
	threads, err := h.community.ListThreads(c.Context(), q.Get("subject_type"), q.Get("subject_slug"), cur)
	if err != nil {
		return communityError(err)
	}
	out := make([]threadResponse, len(threads))
	for i, t := range threads {
		out[i] = toThreadResponse(t)
	}
	return c.JSON(fiber.Map{"data": out, "meta": pageMeta(threads, h.community.PageSize())})
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
func pageMeta[T interface{ Position() community.Cursor }](items []T, pageSize int32) fiber.Map {
	meta := fiber.Map{}
	if n := len(items); n > 0 && int32(n) == pageSize {
		pos := items[n-1].Position()
		meta["next_cursor"] = encodeCursor(pos.CreatedAt, pos.ID)
	}
	return meta
}

// recentThreadResponse is a feed row: a thread plus the display name of the subject it
// hangs off, which the cross-subject listing must carry because a reader has no other
// way to know what the thread is about. Both name fields are empty when the subject no
// longer exists (there is no FK to it) — the client renders the slug instead.
type recentThreadResponse struct {
	threadResponse
	SubjectTitle   string `json:"subject_title"`
	SubjectCompany string `json:"subject_company"`
}

// ListRecentThreads returns open threads across every subject, newest first,
// keyset-paged — the /discussions feed. Public, like the other thread reads.
func (h *communityHandlers) ListRecentThreads(c *fiber.Ctx) error {
	cur, err := decodeCursor(queryValues(c).Get("cursor"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid cursor")
	}
	threads, err := h.community.ListRecentThreads(c.Context(), cur)
	if err != nil {
		return communityError(err)
	}
	out := make([]recentThreadResponse, len(threads))
	for i, t := range threads {
		out[i] = recentThreadResponse{
			threadResponse: toThreadResponse(t.Thread),
			SubjectTitle:   t.SubjectTitle,
			SubjectCompany: t.SubjectCompany,
		}
	}
	return c.JSON(fiber.Map{"data": out, "meta": pageMeta(threads, h.community.PageSize())})
}

// CountThreads returns a subject's open-thread count — the detail-page badge. Public.
func (h *communityHandlers) CountThreads(c *fiber.Ctx) error {
	q := queryValues(c)
	n, err := h.community.CountThreads(c.Context(), q.Get("subject_type"), q.Get("subject_slug"))
	if err != nil {
		return communityError(err)
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"count": n}})
}

// GetThread returns a single thread with its first page of replies. Public.
func (h *communityHandlers) GetThread(c *fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	thread, err := h.community.GetThread(c.Context(), id)
	if err != nil {
		return communityError(err)
	}
	cur, err := decodeCursor(queryValues(c).Get("cursor"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid cursor")
	}
	replies, err := h.community.ListReplies(c.Context(), id, cur)
	if err != nil {
		return communityError(err)
	}
	out := make([]replyResponse, len(replies))
	for i, r := range replies {
		out[i] = toReplyResponse(r)
	}
	return c.JSON(fiber.Map{
		"data": fiber.Map{"thread": toThreadResponse(thread), "replies": out},
		"meta": pageMeta(replies, h.community.PageSize()),
	})
}

// CreateThread opens a thread on a company or job. RequireAuth; 400/404/422/429.
func (h *communityHandlers) CreateThread(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in createThreadBody
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	thread, err := h.community.CreateThread(c.Context(), community.CreateThreadInput{
		UserID: userID, SubjectType: in.SubjectType, SubjectSlug: in.SubjectSlug, Title: in.Title, Body: in.Body,
	})
	if err != nil {
		return communityError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toThreadResponse(thread)})
}

// CreateReply posts a reply to an open thread. RequireAuth; 404/409/422/429.
func (h *communityHandlers) CreateReply(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}
	var in createReplyBody
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	reply, err := h.community.Reply(c.Context(), id, in.ParentReplyID, userID, in.Body)
	if err != nil {
		return communityError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toReplyResponse(reply)})
}

// CloseThread closes a thread (moderator-gated at the route).
func (h *communityHandlers) CloseThread(c *fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	if err := h.community.Close(c.Context(), id); err != nil {
		return communityError(err)
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"id": id, "status": community.StatusClosed}})
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
	case errors.Is(err, community.ErrRateLimited):
		return fiber.NewError(fiber.StatusTooManyRequests, "rate limit exceeded")
	default:
		return err
	}
}
