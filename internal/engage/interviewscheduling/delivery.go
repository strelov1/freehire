package interviewscheduling

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/application/gmailsync"
	"github.com/strelov1/freehire/internal/engage/bookinglink"
	"github.com/strelov1/freehire/internal/engage/slots"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

func (w *Worker) deliver(ctx context.Context, row db.ClaimInterviewSchedulingThreadRow, gmail *gmailsync.ReplyClient, thread []gmailsync.ThreadMessage) error {
	for _, message := range thread {
		if message.Sent && message.MessageID == row.PendingMessageID {
			return w.finish(ctx, row, message.ID)
		}
	}
	if row.AttemptedAt.Valid {
		if time.Since(row.AttemptedAt.Time) >= 10*time.Minute {
			return w.end(ctx, row, "review")
		}
		return nil
	}
	if len(thread) == 0 {
		return w.end(ctx, row, "review")
	}
	if automationInterrupted(thread, row) {
		return w.end(ctx, row, "closed")
	}
	latest := thread[len(thread)-1]
	expectedLatest := row.PendingParentID
	if row.PendingKind == "followup" {
		expectedLatest = row.LastSentID
	}
	if latest.ID != expectedLatest {
		if latest.Sent {
			return w.end(ctx, row, "closed")
		}
		return w.end(ctx, row, "review")
	}
	parent, found := findMessage(thread, row.PendingParentID)
	if !found || !replyable(parent, row.RecruiterEmail) {
		return w.end(ctx, row, "review")
	}
	ok, err := w.q.BeginInterviewReply(ctx, db.BeginInterviewReplyParams{
		ID: row.ID, RequiredScopes: RequiredScopes(), RefreshTokenEnc: row.RefreshTokenEnc,
	})
	if err != nil || ok != 1 {
		return err
	}
	id, err := gmail.Send(ctx, gmailsync.Reply{
		ThreadID: row.GmailThreadID, Sender: row.MailboxEmail, Recipient: row.RecruiterEmail,
		Subject: parent.Subject, ParentMessageID: parent.MessageID, MessageID: row.PendingMessageID,
		Body: row.PendingBody,
	})
	if err != nil {
		// A lost response could mean Gmail accepted the message. Recovery checks its RFC ID
		// before the thread can advance, and hands off a send it cannot confirm.
		return err
	}
	return w.finish(ctx, row, id)
}

func (w *Worker) finish(ctx context.Context, row db.ClaimInterviewSchedulingThreadRow, sentID string) error {
	return w.q.FinishInterviewReply(ctx, db.FinishInterviewReplyParams{
		ID: row.ID, SentID: sentID, MessageID: row.PendingMessageID,
	})
}

func (w *Worker) followup(ctx context.Context, row db.ClaimInterviewSchedulingThreadRow, thread []gmailsync.ThreadMessage) error {
	if !row.FollowupAt.Valid || time.Now().Before(row.FollowupAt.Time) {
		return nil
	}
	if row.Followups >= 2 {
		return w.end(ctx, row, "review")
	}
	parent, found := findMessage(thread, row.LastProcessedID)
	if !found || !replyable(parent, row.RecruiterEmail) {
		return w.end(ctx, row, "review")
	}
	return w.queue(ctx, row, parent, "followup", wording[language(row.Language)].followup,
		row.OfferedStarts, row.OfferZone, row.Language)
}

func (w *Worker) reserve(ctx context.Context, row db.ClaimInterviewSchedulingThreadRow, parent gmailsync.ThreadMessage) error {
	args := db.GetSchedulingBookingParams{
		UserID: row.UserID, ApplicationID: row.ApplicationID,
		StartsAt: row.SelectedStart, RecruiterEmail: row.RecruiterEmail,
	}
	booking, err := w.q.GetSchedulingBooking(ctx, args)
	if errors.Is(err, pgx.ErrNoRows) {
		token, err := w.bookingToken(ctx, row)
		if err != nil {
			return err
		}
		recruiter, err := mail.ParseAddress(parent.From)
		if err != nil {
			return w.end(ctx, row, "review")
		}
		name := strings.TrimSpace(recruiter.Name)
		if name == "" {
			name = "Recruiter"
		}
		_, err = w.booking.Book(ctx, bookinglink.BookInput{
			Token: token, StartsAt: row.SelectedStart.Time, Name: name, Email: row.RecruiterEmail,
		})
		if errors.Is(err, bookinglink.ErrSlotUnavailable) {
			return w.offer(ctx, row, parent, row.Language, zoneFor(row, Decision{}))
		}
		return err
	}
	if err != nil {
		return err
	}
	if !booking.CalendarEventKey.Valid || !row.SelectedStart.Time.After(time.Now()) {
		return w.end(ctx, row, "review")
	}
	if !booking.CalendarInvitedAt.Valid {
		return nil
	}
	zone := zoneFor(row, Decision{})
	body := fmt.Sprintf(wording[language(row.Language)].confirm, displayTime(row.SelectedStart.Time, zone))
	return w.queue(ctx, row, parent, "confirm", body, row.OfferedStarts, zone.String(), row.Language)
}

func (w *Worker) bookingToken(ctx context.Context, row db.ClaimInterviewSchedulingThreadRow) (string, error) {
	token, ok, err := w.booking.TokenFor(ctx, row.UserID, row.ApplicationID)
	if err != nil || ok {
		return token, err
	}
	zone := zoneFor(db.ClaimInterviewSchedulingThreadRow{CandidateZone: row.CandidateZone}, Decision{})
	return w.booking.Enable(ctx, row.UserID, row.ApplicationID, zone.String())
}

func (w *Worker) firstAvailable(ctx context.Context, row db.ClaimInterviewSchedulingThreadRow, times []time.Time, zone *time.Location) (time.Time, error) {
	if len(times) == 1 {
		// Reserve will check that the slot is free immediately before writing.
		return times[0], nil
	}
	token, err := w.bookingToken(ctx, row)
	if err != nil {
		return time.Time{}, err
	}
	free, err := w.booking.Slots(ctx, token, zone.String())
	if err != nil {
		return time.Time{}, err
	}
	for _, at := range times {
		if slices.ContainsFunc(free.Slots, func(slot slots.Interval) bool { return slot.Start.Equal(at) }) {
			return at, nil
		}
	}
	return time.Time{}, nil
}

func (w *Worker) offer(ctx context.Context, row db.ClaimInterviewSchedulingThreadRow, parent gmailsync.ThreadMessage, lang string, zone *time.Location) error {
	token, err := w.bookingToken(ctx, row)
	if err != nil {
		return err
	}
	free, err := w.booking.Slots(ctx, token, zone.String())
	if err != nil {
		return err
	}
	options := make([]pgtype.Timestamptz, 0, 3)
	lines := make([]string, 0, 3)
	for _, slot := range free.Slots {
		if slices.ContainsFunc(row.OfferedStarts, func(at pgtype.Timestamptz) bool { return at.Time.Equal(slot.Start) }) {
			continue
		}
		options = append(options, pgconv.Instant(slot.Start))
		lines = append(lines, fmt.Sprintf("%d. %s", len(options), displayTime(slot.Start, zone)))
		if len(options) == 3 {
			break
		}
	}
	if len(options) < 3 {
		return w.end(ctx, row, "review")
	}
	body := fmt.Sprintf(wording[language(lang)].offer, strings.Join(lines, "\n"), w.origin+"/booking/"+token)
	return w.queue(ctx, row, parent, "offer", body, options, zone.String(), lang)
}

func (w *Worker) queue(ctx context.Context, row db.ClaimInterviewSchedulingThreadRow, parent gmailsync.ThreadMessage,
	kind, body string, offers []pgtype.Timestamptz, zone, lang string,
) error {
	row.Language = language(lang)
	body = w.compose(ctx, row, kind, parent.ID, body)
	return w.q.ScheduleInterviewReply(ctx, db.ScheduleInterviewReplyParams{
		ID: row.ID, Kind: kind, Body: body, ParentID: parent.ID, ProcessedID: parent.ID,
		MessageID: "<" + uuid.NewString() + "@freehire.me>", OfferedStarts: offers,
		OfferZone: zone, Language: language(lang),
	})
}

func zoneFor(row db.ClaimInterviewSchedulingThreadRow, decision Decision) *time.Location {
	for _, name := range []string{decision.Zone, row.OfferZone, row.CandidateZone} {
		if name != "UTC" && !strings.Contains(name, "/") {
			continue
		}
		if zone, err := time.LoadLocation(name); err == nil {
			return zone
		}
	}
	return time.UTC
}

func displayTime(at time.Time, zone *time.Location) string {
	return at.In(zone).Format("2006-01-02 15:04 MST (UTC-07:00)")
}

func language(name string) string {
	if _, ok := wording[name]; ok {
		return name
	}
	return "en"
}

var wording = map[string]struct{ offer, followup, confirm string }{
	"en": {
		"Hi, thanks for reaching out. Would one of these times work for the interview?\n\n%s\n\nYou can also choose a time here: %s",
		"Hi, just following up on the interview times I sent. Does one of those options work for you?",
		"Thanks, that works for me. The interview is confirmed for %s. A calendar invitation with a Google Meet link has been sent.",
	},
	"pt": {
		"Olá, obrigado pelo contato. Algum destes horários funciona para a entrevista?\n\n%s\n\nSe preferir, escolha um horário aqui: %s",
		"Olá, gostaria de saber se algum dos horários que enviei funciona para você.",
		"Obrigado. A entrevista está confirmada para %s. Enviei um convite de calendário com o link do Google Meet.",
	},
	"es": {
		"Hola, gracias por escribir. ¿Te viene bien alguno de estos horarios para la entrevista?\n\n%s\n\nTambién puedes elegir un horario aquí: %s",
		"Hola, retomo mi mensaje sobre la entrevista. ¿Te viene bien alguno de los horarios que envié?",
		"Gracias. La entrevista queda confirmada para %s. Ya se envió la invitación del calendario con el enlace de Google Meet.",
	},
	"ru": {
		"Здравствуйте! Подойдёт ли вам один из этих вариантов для собеседования?\n\n%s\n\nТакже можно выбрать время по ссылке: %s",
		"Здравствуйте! Уточняю, подходит ли вам какой-то из предложенных вариантов для собеседования.",
		"Спасибо! Собеседование подтверждено на %s. Приглашение со ссылкой Google Meet отправлено.",
	},
}
