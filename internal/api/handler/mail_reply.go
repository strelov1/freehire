package handler

import (
	"context"
	"errors"
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/application/mailreply"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// DraftEmailReply drafts the caller's reply to one received message. Nothing is sent or
// stored: the draft goes back to the candidate, who edits and sends it from their own client.
//
// Every request charges one mail-reply allowance under a fresh reference (a redraft is a
// second model call), and a draft that fails gives it back.
func (h *inboxHandlers) DraftEmailReply(ctx context.Context, params apigen.DraftEmailReplyParams) (apigen.DraftEmailReplyRes, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := emailPathID(params.ID)
	if err != nil {
		return nil, err
	}
	msg, err := h.inbox.Get(ctx, userID, id)
	if err != nil {
		return nil, err // pgx.ErrNoRows → 404
	}
	if msg.Direction == "outgoing" {
		return nil, fiber.NewError(fiber.StatusConflict, "only a received message can be answered")
	}
	// After the ownership read, as on mail recall: "not yours" must not depend on configuration.
	if h.llm.client == nil {
		return nil, fiber.NewError(fiber.StatusServiceUnavailable, "reply drafting is not configured")
	}
	model, err := h.llm.bind(ctx, userID, llm.Feature(tagMailReply))
	if err != nil {
		return nil, err
	}
	ref, refusal, err := h.chargeReply(ctx, userID, msg.ID)
	if refusal != nil {
		return refusal.cv()
	}
	if err != nil {
		return nil, err
	}
	company := msg.LinkedCompany
	if company == "" {
		company = msg.SuggestedCompany
	}
	draft, err := mailreply.New(model).Draft(ctx, mailreply.Message{
		FromName: msg.FromName, FromAddr: msg.FromAddr, Subject: msg.Subject,
		Body: msg.BodyText, Company: company,
	})
	if err != nil {
		h.releaseReply(userID, ref)
		if errors.Is(err, mailreply.ErrModel) && ctx.Err() == nil {
			// Provider errors can quote the prompt, which holds the sender's mail: never log err.
			log.Printf("mail-reply: user %d email %d: the model gave no usable draft", userID, msg.ID)
			return nil, fiber.NewError(fiber.StatusBadGateway, "could not draft a reply right now")
		}
		return nil, err
	}
	return &apigen.DraftEmailReplyOK{Data: apigen.MailReplyDraft{
		Recipient: msg.FromAddr, RecipientName: msg.FromName,
		Subject: draft.Subject, Body: draft.Body,
	}}, nil
}

// chargeReply takes one mail-reply allowance. A deployment with no meter and an unreadable
// ledger both let the draft run (the rule every meter here follows); a reached ceiling
// returns the refusal to answer with.
func (h *inboxHandlers) chargeReply(ctx context.Context, userID, emailID int64) (string, *planRefusal, error) {
	if h.plans == nil {
		return "", nil, nil
	}
	ref := "mail-reply#" + strconv.FormatInt(emailID, 10) + "#" + uuid.NewString()
	d, err := h.plans.Consume(ctx, userID, plan.FeatureMailReply, ref)
	switch {
	case err == nil:
		return ref, nil, nil
	case isRefusal(err):
		r := decisionRefusal(d)
		return "", &r, nil
	default:
		log.Printf("plan: charging a reply draft for user %d: %v", userID, err)
		return "", nil, nil
	}
}

// releaseReply gives back a charge whose draft failed, on a detached context so a cancelled
// request still releases.
func (h *inboxHandlers) releaseReply(userID int64, ref string) {
	if h.plans == nil || ref == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.plans.Release(ctx, userID, plan.FeatureMailReply, ref); err != nil {
		log.Printf("plan: releasing a reply draft for user %d: %v", userID, err)
	}
}
