package handler

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/application/mailbox"
	"github.com/strelov1/freehire/internal/platform/db"
)

// mailboxStatus is the wire shape for the hosted-mailbox endpoints: the caller's
// address (null when none) and whether the feature is configured.
type mailboxStatus struct {
	Available bool    `json:"available"`
	Address   *string `json:"address"`
}

// mailboxReady reports whether the hosted-mailbox feature is configured.
func (h *inboxHandlers) mailboxReady() bool { return h.mailDomain != "" }

// mailboxAddress composes the caller's hosted-mailbox address from their account
// username, if they are enrolled — a read, never an allocation: a caller with no
// mailbox row gets ok=false without EnsureUsername ever being called.
func (h *inboxHandlers) mailboxAddress(ctx context.Context, userID int64) (address string, ok bool, err error) {
	if _, err := h.queries.GetMailboxByUser(ctx, userID); errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	} else if err != nil {
		return "", false, err
	}
	name, _, hasName, err := h.accounts.Username(ctx, userID)
	if err != nil {
		return "", false, err
	}
	if !hasName {
		// A mailbox row exists but the account has no username yet. Not possible
		// via a fresh claim (mailbox.GetOrCreate resolves the username before
		// enrolling), but real for the deploy window before
		// cmd/backfill-username-from-mailbox has visited a pre-existing legacy
		// row. Report no address rather than have a read allocate one.
		return "", false, nil
	}
	return name + "@" + h.mailDomain, true, nil
}

// GetMailbox returns the caller's mailbox address (or null) and feature availability.
func (h *inboxHandlers) GetMailbox(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	addr, ok, err := h.mailboxAddress(c.Context(), userID)
	if err != nil {
		return err
	}
	st := mailboxStatus{Available: h.mailboxReady()}
	if ok {
		st.Address = &addr
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
	addr, err := mailbox.GetOrCreate(c.Context(), dbMailboxStore{h.queries}, h.accounts, userID, user.Email, h.mailDomain)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": mailboxStatus{Available: true, Address: &addr}})
}

// ReleaseMailbox drops the caller's mailbox and purges its received mail; Gmail
// mail is untouched. The account's username is untouched too — releasing stops
// mail delivery, it is not a username change.
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

// dbMailboxStore adapts *db.Queries to mailbox.Store.
type dbMailboxStore struct{ q *db.Queries }

func (s dbMailboxStore) EnsureRow(ctx context.Context, userID int64) error {
	return s.q.EnsureMailbox(ctx, userID)
}
