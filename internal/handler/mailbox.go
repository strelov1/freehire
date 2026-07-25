package handler

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/mailbox"
	"github.com/strelov1/freehire/internal/pgerr"
)

// mailboxStatus is the wire shape for the hosted-mailbox endpoints: the caller's
// address (null when none) and whether the feature is configured.
type mailboxStatus struct {
	Available bool    `json:"available"`
	Address   *string `json:"address"`
}

// mailboxReady reports whether the hosted-mailbox feature is configured.
func (h *inboxHandlers) mailboxReady() bool { return h.mailDomain != "" }

// GetMailbox returns the caller's mailbox address (or null) and feature availability.
func (h *inboxHandlers) GetMailbox(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	st := mailboxStatus{Available: h.mailboxReady()}
	mb, err := h.queries.GetMailboxByUser(c.Context(), userID)
	if err == nil {
		st.Address = &mb.Address
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	return c.JSON(fiber.Map{"data": st})
}

// ClaimMailbox allocates (or returns) the caller's hosted mailbox address.
func (h *inboxHandlers) ClaimMailbox(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	user, err := h.queries.GetUserByID(c.Context(), userID)
	if err != nil {
		return err
	}
	addr, err := mailbox.GetOrCreate(c.Context(), dbMailboxStore{h.queries}, userID, user.Email, h.mailDomain)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": mailboxStatus{Available: true, Address: &addr}})
}

// ReleaseMailbox drops the caller's mailbox and purges its received mail; Gmail
// mail is untouched.
func (h *inboxHandlers) ReleaseMailbox(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if err := h.queries.DeleteMailbox(c.Context(), userID); err != nil {
		return err
	}
	if err := h.queries.DeleteEmailsBySource(c.Context(), db.DeleteEmailsBySourceParams{UserID: userID, Source: "hosted"}); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": mailboxStatus{Available: true, Address: nil}})
}

// dbMailboxStore adapts *db.Queries to mailbox.Store, mapping a Postgres unique
// violation to mailbox.ErrTaken so the allocator can retry the next suffix.
type dbMailboxStore struct{ q *db.Queries }

func (s dbMailboxStore) AddressByUser(ctx context.Context, userID int64) (string, bool, error) {
	mb, err := s.q.GetMailboxByUser(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return mb.Address, true, nil
}

func (s dbMailboxStore) Insert(ctx context.Context, userID int64, address string) error {
	_, err := s.q.InsertMailbox(ctx, db.InsertMailboxParams{UserID: userID, Address: address})
	if pgerr.IsUniqueViolation(err) {
		return mailbox.ErrTaken
	}
	return err
}
