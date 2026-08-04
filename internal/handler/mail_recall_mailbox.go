package handler

import (
	"context"
	"time"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/gmailsync"
	"github.com/strelov1/freehire/internal/mailrecall"
	"github.com/strelov1/freehire/internal/tokencrypt"
)

// gmailMailboxes mints a per-caller view of their connected Gmail for the recall sweep.
//
// It is a factory rather than a mailbox because the credential is per user: `For` resolves
// one caller's grant and answers nil when there is none, which is exactly what the recall
// service reads as "fall back to stored mail". A caller on a hosted address, or on the
// bring-your-own-harness tier, therefore keeps today's path without the handler knowing
// which tier they are.
type gmailMailboxes struct {
	store     *gmailsync.DBStore
	cipher    *tokencrypt.Cipher
	connector *gmailsync.Connector
}

func newGmailMailboxes(queries *db.Queries, cipher *tokencrypt.Cipher, connector *gmailsync.Connector) *gmailMailboxes {
	return &gmailMailboxes{store: gmailsync.NewDBStore(queries), cipher: cipher, connector: connector}
}

// For returns the caller's mailbox, or nil when they have no usable Gmail grant.
//
// A missing or unreadable credential is nil rather than an error on purpose: it is not a
// failure, it is a caller the search path does not serve, and the stored path still does.
func (g *gmailMailboxes) For(ctx context.Context, userID int64) mailrecall.Mailbox {
	if g == nil || g.connector == nil || g.cipher == nil {
		return nil
	}
	enc, err := g.store.RefreshToken(ctx, userID)
	if err != nil || enc == "" {
		return nil
	}
	refresh, err := g.cipher.Decrypt(enc)
	if err != nil {
		return nil
	}
	reader := g.connector.ReaderFactory()(ctx, refresh, nil)
	searcher, ok := reader.(gmailsync.MailboxSearcher)
	if !ok {
		return nil
	}
	return &gmailMailbox{
		searcher: searcher,
		importer: gmailsync.NewImporter(reader, g.store),
	}
}

// maxSearchResults bounds one search. The recall query is already scoped to one employer
// inside one window — on production the widest application returned nine — so this is a
// guard against a pathological employer name, not a page size anybody is expected to hit.
const maxSearchResults = 25

// gmailMailbox is one caller's Gmail, for the length of one request.
type gmailMailbox struct {
	searcher gmailsync.MailboxSearcher
	importer gmailsync.MessageImporter
}

// Search asks Gmail for this employer's mail inside the window.
//
// The query is gmailsync's to build: it owns the ATS vocabulary and the gate that keeps a
// company-name search away from personal correspondence.
func (m *gmailMailbox) Search(ctx context.Context, _ int64, company, role string, since, until time.Time) ([]mailrecall.Message, error) {
	found, err := m.searcher.Search(ctx, gmailsync.BuildRecallQuery(company, role, since, until), maxSearchResults)
	if err != nil {
		return nil, err
	}
	out := make([]mailrecall.Message, 0, len(found))
	for _, f := range found {
		out = append(out, mailrecall.Message{
			ProviderID: f.ID, FromAddr: f.FromAddr, FromName: f.FromName,
			Subject: f.Subject, BodyText: f.BodyText, BodyHTML: f.BodyHTML,
			ReceivedAt: f.ReceivedAt, ICalUID: f.CalendarUID,
		})
	}
	return out, nil
}

// Import stores one message the caller chose. It is not part of mailrecall.Mailbox — the
// sweep may only look — and is reached from the link handler instead.
func (m *gmailMailbox) Import(ctx context.Context, userID int64, providerID string) error {
	return m.importer.Import(ctx, userID, providerID)
}
