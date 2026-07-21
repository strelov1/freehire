package handler

import (
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/community"
)

// aiAuthor is the display name used for a reply with no persona (a future AI-authored
// reply). No AI posts exist at MVP; this keeps the wire shape stable for when they do.
const aiAuthor = "AI"

// threadResponse is the public shape of a thread: the persona handle is the only
// author identity — the author's user_id is never projected here.
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

// replyResponse is the public shape of a reply: handle only, or the AI persona for a
// system reply.
type replyResponse struct {
	ID        int64     `json:"id"`
	ThreadID  int64     `json:"thread_id"`
	ParentID  int64     `json:"parent_id"`
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

func toThreadResponse(t community.Thread) threadResponse {
	return threadResponse{
		ID: t.ID, SubjectType: t.SubjectType, SubjectSlug: t.SubjectRef, Title: t.Title,
		Body: t.Body, Author: t.AuthorHandle, ReplyCount: t.ReplyCount, Status: t.Status,
		CreatedAt: t.CreatedAt,
	}
}

func toReplyResponse(r community.Reply) replyResponse {
	author := r.AuthorHandle
	if r.IsAI || author == "" {
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
func (a *API) ListThreads(c *fiber.Ctx) error {
	q := queryValues(c)
	cur, err := decodeCursor(q.Get("cursor"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid cursor")
	}
	threads, err := a.community.ListThreads(c.Context(), q.Get("subject_type"), q.Get("subject_slug"), cur)
	if err != nil {
		return communityError(err)
	}
	out := make([]threadResponse, len(threads))
	for i, t := range threads {
		out[i] = toThreadResponse(t)
	}
	meta := fiber.Map{}
	if n := len(threads); n > 0 {
		last := threads[n-1]
		meta["next_cursor"] = encodeCursor(last.CreatedAt, last.ID)
	}
	return c.JSON(fiber.Map{"data": out, "meta": meta})
}

// CountThreads returns a subject's open-thread count — the detail-page badge. Public.
func (a *API) CountThreads(c *fiber.Ctx) error {
	q := queryValues(c)
	n, err := a.community.CountThreads(c.Context(), q.Get("subject_type"), q.Get("subject_slug"))
	if err != nil {
		return communityError(err)
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"count": n}})
}

// GetThread returns a single thread with its first page of replies. Public.
func (a *API) GetThread(c *fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	thread, err := a.community.GetThread(c.Context(), id)
	if err != nil {
		return communityError(err)
	}
	cur, err := decodeCursor(queryValues(c).Get("cursor"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid cursor")
	}
	replies, err := a.community.ListReplies(c.Context(), id, cur)
	if err != nil {
		return communityError(err)
	}
	out := make([]replyResponse, len(replies))
	for i, r := range replies {
		out[i] = toReplyResponse(r)
	}
	meta := fiber.Map{}
	if n := len(replies); n > 0 {
		last := replies[n-1]
		meta["next_cursor"] = encodeCursor(last.CreatedAt, last.ID)
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"thread": toThreadResponse(thread), "replies": out}, "meta": meta})
}

// CreateThread opens a thread on a company or job. RequireAuth; 400/404/422/429.
func (a *API) CreateThread(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in createThreadBody
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	thread, err := a.community.CreateThread(c.Context(), community.CreateThreadInput{
		UserID: userID, SubjectType: in.SubjectType, SubjectSlug: in.SubjectSlug, Title: in.Title, Body: in.Body,
	})
	if err != nil {
		return communityError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toThreadResponse(thread)})
}

// CreateReply posts a reply to an open thread. RequireAuth; 404/409/422/429.
func (a *API) CreateReply(c *fiber.Ctx) error {
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
	reply, err := a.community.Reply(c.Context(), id, in.ParentReplyID, userID, in.Body)
	if err != nil {
		return communityError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toReplyResponse(reply)})
}

// CloseThread closes a thread (moderator-gated at the route).
func (a *API) CloseThread(c *fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	if err := a.community.Close(c.Context(), id); err != nil {
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
