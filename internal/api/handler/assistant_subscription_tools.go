package handler

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/engage/notify"
	"github.com/strelov1/freehire/internal/engage/subscription"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
	"github.com/strelov1/freehire/internal/search/savedsearch"
	"github.com/strelov1/freehire/internal/search/search"
)

// The job-alert tools of the signed-in MCP server: an agent can subscribe a search to the
// person's alerts, list and change them, remove them, and ask what they matched. MCP cannot
// push, so delivery stays where it is (email, Telegram, mobile app, webhook).
// An ALERT is what the alerts page shows: one saved search delivered on one or more channels.
// In storage that is a saved_searches row plus one subscriptions row per channel; tools address
// the alert by its saved search id and name the per-channel rows only where they differ.
// Every write goes through the website's use cases (savedsearch.Service, subscription.Service),
// so Narrows, the push-device rule and the per-account cap are enforced once.

// subscriptionMCPTools is the set the account server adds for job alerts. Not offered to
// the in-app assistant: the browser there already has the alert controls beside the
// filters. origin builds the links an answer hands back.
func (h *assistantHandlers) subscriptionMCPTools(origin string) []assistant.Tool {
	return []assistant.Tool{
		h.subscribeTool(origin),
		h.listSubscriptionsTool(origin),
		h.updateSubscriptionTool(origin),
		h.unsubscribeTool(),
		h.subscriptionUpdatesTool(origin),
	}
}

const (
	// subscriptionUpdatesDefaultDays and subscriptionUpdatesMaxDays bound the look-back of
	// subscription_updates; a match older than a few weeks is no longer news.
	subscriptionUpdatesDefaultDays = 7
	subscriptionUpdatesMaxDays     = 30
	subscriptionUpdatesDefaultMax  = 20
	subscriptionUpdatesMax         = 50
)

// alertExample is the one worked example every alert tool's description shares, so the
// model sees that an alert is any search and not only an employer.
const alertExample = "e.g. remote senior Go roles in the EU paying at least 80k EUR is " +
	`{"filters":{"work_mode":["remote"],"seniority":["senior"],"skills":["go"],"regions":["eu"],` +
	`"salary_currency":["EUR"]},"salary_min":80000}`

// unfilteredRefusal is the website's own refusal (subscriptionError), worded for an agent.
const unfilteredRefusal = "add at least one filter or a query — an unfiltered search would notify the person about every job"

func (h *assistantHandlers) subscriptions() *subscription.Service {
	return subscription.New(subscription.NewQueriesRepository(h.queries))
}

func (h *assistantHandlers) savedSearches() *savedsearch.Service {
	return savedsearch.New(savedsearch.NewQueriesRepository(h.queries))
}

// alertChannelsHelp says where a person connects a delivery channel or turns alert mail
// back on. An agent cannot do either; it can only hand the person the link.
func alertChannelsHelp(origin string) string {
	return fmt.Sprintf("connect Telegram or a webhook at %s/my/integrations, or turn job-alert email "+
		"back on at %s/my/notifications/settings", origin, origin)
}

// deliverableChannels lists the channels that would reach this account now, in the order a
// default is picked from, using the delivery worker's own tests: email needs the job-alerts mail
// switch, Telegram a linked chat, push a registered device, a webhook an enabled destination.
// A subscription on any other channel is soft-skipped on every pass.
func deliverableChannels(st db.GetAlertChannelStateRow) []string {
	var out []string
	if st.EmailAlertsEnabled {
		out = append(out, notify.ChannelEmail)
	}
	if st.TelegramLinked {
		out = append(out, notify.ChannelTelegram)
	}
	if st.HasPushDevice {
		out = append(out, notify.ChannelPush)
	}
	if st.WebhookEnabled {
		out = append(out, notify.ChannelWebhook)
	}
	return out
}

// requireDeliverable refuses any channel in want the account cannot receive on, naming
// the ones it can. Called before anything is written.
func (h *assistantHandlers) requireDeliverable(ctx context.Context, userID int64, want []string, origin string) ([]string, error) {
	state, err := h.queries.GetAlertChannelState(ctx, userID)
	if err != nil {
		return nil, err
	}
	available := deliverableChannels(state)
	if len(available) == 0 {
		return nil, fiber.NewError(fiber.StatusConflict, "nothing was changed: this account has no way to receive job alerts — "+
			"alert email is turned off and no other channel is connected. Ask the person to "+alertChannelsHelp(origin))
	}
	for _, ch := range want {
		if !slices.Contains(available, ch) {
			return nil, fiber.NewError(fiber.StatusConflict, fmt.Sprintf("nothing was changed: the %s channel cannot reach this account (connected: %s). "+
				"Choose one of those, or ask the person to %s", ch, strings.Join(available, ", "), alertChannelsHelp(origin)))
		}
	}
	return available, nil
}

// alertSearchArgs is the search part of subscribe and update_subscription: search_jobs'
// query and facet filters, plus the salary bounds the website's filter panel offers. Every
// field is a pointer so an update can tell "leave as is" (absent) from "clear" (empty).
type alertSearchArgs struct {
	Query     *string              `json:"query"`
	Filters   *assistantFilterArgs `json:"filters"`
	SalaryMin *int                 `json:"salary_min"`
	SalaryMax *int                 `json:"salary_max"`
}

func (a alertSearchArgs) given() bool {
	return a.Query != nil || a.Filters != nil || a.SalaryMin != nil || a.SalaryMax != nil
}

// alertSearchSchema is the JSON schema of alertSearchArgs' fields, merged into a tool's
// own properties.
func alertSearchSchema(props map[string]any) map[string]any {
	props["query"] = map[string]any{"type": "string", "description": "Free-text query, with the job search's operators."}
	props["filters"] = assistantFilterSchema("Facet filters, the job search's own.")
	props["salary_min"] = map[string]any{"type": "integer", "description": "Only postings whose stated salary floor is at least this, in the posting's currency — pair it with filters.salary_currency."}
	props["salary_max"] = map[string]any{"type": "integer", "description": "Only postings whose stated salary ceiling is at most this."}
	return props
}

// applyAlertSearch writes the given search fields over base (the alert's stored query, or
// nothing for a new one). A given field replaces what was there: filters replaces every facet
// (an empty object clears them), query the text, a salary bound of 0 clears it. Params only the
// website sets (exclusions, match-all modes, visa) are kept.
// Unknown filter NAMES are refused (the search_jobs rule); a value outside a closed vocabulary
// is DROPPED and named, since kept it would narrow the alert to nothing.
func applyAlertSearch(base url.Values, a alertSearchArgs) (url.Values, []string, error) {
	out := url.Values{}
	for k, vs := range base {
		out[k] = slices.Clone(vs)
	}
	var ignored []string
	if a.Filters != nil {
		vals, err := a.Filters.values()
		if err != nil {
			return nil, nil, err
		}
		for name := range search.StringFacets {
			out.Del(name)
		}
		for param, list := range vals {
			for _, v := range list {
				if search.UnknownFacetValue(param, v) {
					ignored = append(ignored, param+"="+v)
					continue
				}
				if !slices.Contains(out[param], v) {
					out[param] = append(out[param], v)
				}
			}
		}
	}
	if a.Query != nil {
		out.Del("q")
		if q := strings.TrimSpace(*a.Query); q != "" {
			out.Set("q", q)
		}
	}
	for param, bound := range map[string]*int{"salary_min": a.SalaryMin, "salary_max": a.SalaryMax} {
		if bound == nil {
			continue
		}
		out.Del(param)
		if *bound > 0 {
			out.Set(param, strconv.Itoa(*bound))
		}
	}
	sort.Strings(ignored)
	return out, ignored, nil
}

// alertSearch is a stored query in the shape the tools take, so an agent can edit an
// alert precisely: read it here, change one field, pass it to update_subscription.
type alertSearch struct {
	Query     string              `json:"query,omitempty"`
	Filters   map[string][]string `json:"filters,omitempty"`
	SalaryMin int                 `json:"salary_min,omitempty"`
	SalaryMax int                 `json:"salary_max,omitempty"`
	// Other is what only the website sets (exclusions, match-all modes, visa and the
	// like). update_subscription keeps it untouched.
	Other map[string]string `json:"other,omitempty"`
}

func alertSearchOf(query string) alertSearch {
	var out alertSearch
	for k, vs := range savedsearch.ParseQuery(query) {
		switch _, facet := search.StringFacets[k]; {
		case k == "sort":
		case k == "q":
			out.Query = strings.Join(vs, ",")
		case facet:
			if out.Filters == nil {
				out.Filters = map[string][]string{}
			}
			out.Filters[k] = vs
		case k == "salary_min" || k == "salary_max":
			n, err := strconv.Atoi(vs[0])
			if err != nil {
				out.addOther(k, vs)
				continue
			}
			if k == "salary_min" {
				out.SalaryMin = n
			} else {
				out.SalaryMax = n
			}
		default:
			out.addOther(k, vs)
		}
	}
	return out
}

func (a *alertSearch) addOther(k string, vs []string) {
	if a.Other == nil {
		a.Other = map[string]string{}
	}
	a.Other[k] = strings.Join(vs, ",")
}

// alertSavedSearchError words savedsearch's sentinels for an agent, then the search layer's
// (a q the parser refuses) and the subscription service's (no push device, bad channel). An
// error none of them knows stays as it is, and reads as internal_error.
func alertSavedSearchError(err error, name string) error {
	switch {
	case errors.Is(err, savedsearch.ErrDuplicateName):
		return fiber.NewError(fiber.StatusConflict, fmt.Sprintf("a saved search named %q already exists on this account — choose another name", name))
	case errors.Is(err, savedsearch.ErrCapExceeded):
		return fiber.NewError(fiber.StatusConflict, "this account has reached its limit of saved searches — remove one on the website's "+
			"alerts page and try again")
	case errors.Is(err, savedsearch.ErrInvalidName):
		return fiber.NewError(fiber.StatusBadRequest, "name must be 1-100 characters")
	case errors.Is(err, savedsearch.ErrQueryTooLong):
		return fiber.NewError(fiber.StatusBadRequest, "this search has too many filters to save — narrow it to fewer values")
	}
	return subscriptionError(searchToolError(err))
}

// alertChannel is one channel an alert is delivered on — one subscriptions row.
type alertChannel struct {
	Channel         string     `json:"channel"`
	SubscriptionID  int64      `json:"subscription_id"`
	Active          bool       `json:"active"`
	Pending         int64      `json:"pending"`
	LastDeliveredAt *time.Time `json:"last_delivered_at,omitempty"`
}

// alert is one saved search and the channels it is delivered on.
type alert struct {
	ID        int64          `json:"id"`
	Name      string         `json:"name"`
	Filter    string         `json:"filter"`
	Search    alertSearch    `json:"search"`
	Paused    bool           `json:"paused"`
	Channels  []alertChannel `json:"channels"`
	CreatedAt *time.Time     `json:"created_at,omitempty"`

	query string
}

// alerts reads the caller's alerts, newest first: their subscriptions grouped by saved
// search, with each channel's delivery state. A saved search with no subscription is a
// bookmarked view on the website, not an alert, and is not listed.
func (h *assistantHandlers) alerts(ctx context.Context, userID int64) ([]alert, error) {
	rows, err := h.subscriptions().List(ctx, userID)
	if err != nil {
		return nil, err
	}
	stats, err := h.queries.ListSubscriptionDeliveryStats(ctx, userID)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]db.ListSubscriptionDeliveryStatsRow, len(stats))
	for _, s := range stats {
		byID[s.SubscriptionID] = s
	}
	var out []alert
	index := map[int64]int{}
	for _, r := range rows {
		st := byID[r.ID]
		ch := alertChannel{
			Channel: r.Channel, SubscriptionID: r.ID, Active: r.Active,
			Pending: st.Pending, LastDeliveredAt: pgconv.TimePtr(st.LastDeliveredAt),
		}
		i, seen := index[r.SavedSearchID]
		if !seen {
			i = len(out)
			index[r.SavedSearchID] = i
			out = append(out, alert{
				ID: r.SavedSearchID, Name: r.SavedSearchName, Filter: savedsearch.FilterSummary(r.SavedSearchQuery),
				Search: alertSearchOf(r.SavedSearchQuery), CreatedAt: r.CreatedAt, query: r.SavedSearchQuery,
			})
		}
		out[i].Channels = append(out[i].Channels, ch)
		// The alert began with its first channel. Rows arrive newest first, so without this
		// adding a channel would move the alert's creation to that moment.
		if r.CreatedAt != nil && (out[i].CreatedAt == nil || r.CreatedAt.Before(*out[i].CreatedAt)) {
			out[i].CreatedAt = r.CreatedAt
		}
	}
	for i := range out {
		slices.SortFunc(out[i].Channels, func(a, b alertChannel) int { return strings.Compare(a.Channel, b.Channel) })
		out[i].Paused = !slices.ContainsFunc(out[i].Channels, func(c alertChannel) bool { return c.Active })
	}
	return out, nil
}

// ownAlert finds one of the caller's alerts, or says it is not theirs. The writes below
// are owner-scoped in SQL too; this is what turns "no row" into a sentence.
func (h *assistantHandlers) ownAlert(ctx context.Context, userID, id int64) (alert, error) {
	all, err := h.alerts(ctx, userID)
	if err != nil {
		return alert{}, err
	}
	for _, a := range all {
		if a.ID == id {
			return a, nil
		}
	}
	return alert{}, fiber.NewError(fiber.StatusNotFound, fmt.Sprintf("this account has no job alert with id %d — call list_subscriptions for the ids", id))
}

type subscribeResult struct {
	Alert         alert              `json:"alert"`
	Outcomes      []subscribeOutcome `json:"channels_outcome"`
	IgnoredParams []string           `json:"ignored_params,omitempty"`
	ManageURL     string             `json:"manage_url"`
}

type subscribeOutcome struct {
	Channel string `json:"channel"`
	// Status is created or already_subscribed.
	Status string `json:"status"`
}

func (h *assistantHandlers) subscribeTool(origin string) assistant.Tool {
	return assistant.Tool{
		Name:   "subscribe",
		Title:  "Create a job alert",
		Effect: assistant.EffectAdds,
		Description: "Create a job alert: a saved search over ANY of the job search's filters whose new postings are " +
			"delivered to the person on the channels their account has connected (email, Telegram, the mobile " +
			"app, a webhook) — this conversation is not notified. " + alertExample + ". The search must narrow the " +
			"catalogue. Values not in a filter's vocabulary are dropped and named in ignored_params. Leave " +
			"channels empty for the default (email when alert mail is on, otherwise the first connected " +
			"channel). The answer's alert.id addresses the alert from then on. " +
			"Effect: saves the search on the account (or reuses the one already selecting the same postings) " +
			"and delivers it on each channel; nothing existing is changed.",
		Schema: map[string]any{
			"type": "object",
			"properties": alertSearchSchema(map[string]any{
				"name": map[string]any{"type": "string", "description": "A name for the alert (1-100 characters). Omit to name it from its filters."},
				"channels": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string", "enum": notify.Channels},
					"description": "Where to deliver. Only a channel the account has connected is accepted.",
				},
			}),
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				alertSearchArgs
				Name     string   `json:"name"`
				Channels []string `json:"channels"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			vals, ignored, err := applyAlertSearch(nil, in.alertSearchArgs)
			if err != nil {
				return nil, err
			}
			// Checked before anything is saved, so a refused alert leaves no saved search
			// behind. subscription.Service.Create asks the same question again.
			if !search.Narrows(vals) {
				return nil, fiber.NewError(fiber.StatusBadRequest, withIgnored(unfilteredRefusal, ignored))
			}
			channels := nonEmptyStrings(in.Channels)
			available, err := h.requireDeliverable(ctx, userID, channels, origin)
			if err != nil {
				return nil, err
			}
			if len(channels) == 0 {
				channels = available[:1]
			}

			query := savedsearch.EncodeQuery(vals)
			saved, err := h.savedSearches().EnsureForQuery(ctx, userID, in.Name, query)
			if err != nil {
				return nil, alertSavedSearchError(err, cmp.Or(strings.TrimSpace(in.Name), savedsearch.DefaultName(query)))
			}
			out := subscribeResult{IgnoredParams: ignored, ManageURL: origin + "/my/notifications/searches"}
			subs := h.subscriptions()
			for _, ch := range slices.Compact(slices.Sorted(slices.Values(channels))) {
				_, err := subs.Create(ctx, userID, saved.ID, ch)
				switch {
				case errors.Is(err, subscription.ErrDuplicate):
					out.Outcomes = append(out.Outcomes, subscribeOutcome{Channel: ch, Status: "already_subscribed"})
				case errors.Is(err, subscription.ErrUnfilteredSearch):
					return nil, fiber.NewError(fiber.StatusBadRequest, unfilteredRefusal)
				case err != nil:
					return nil, alertSavedSearchError(err, "")
				default:
					out.Outcomes = append(out.Outcomes, subscribeOutcome{Channel: ch, Status: "created"})
				}
			}
			if out.Alert, err = h.ownAlert(ctx, userID, saved.ID); err != nil {
				return nil, err
			}
			return out, nil
		},
	}
}

func withIgnored(msg string, ignored []string) string {
	if len(ignored) == 0 {
		return msg
	}
	return msg + "; these values were not understood and dropped: " + strings.Join(ignored, ", ")
}

// deliverySchedule is the account-wide timing every alert shares. It is shown beside the alerts
// but lives on the notification settings page, so update_subscription does not change it.
type deliverySchedule struct {
	DigestFrequency string `json:"digest_frequency"`
	DigestTime      string `json:"digest_time,omitempty"`
	// DigestIntervalHours is set only for the interval frequency: at most one digest
	// per alert every that many hours.
	DigestIntervalHours int    `json:"digest_interval_hours,omitempty"`
	QuietHours          string `json:"quiet_hours,omitempty"`
	Note                string `json:"note"`
}

func (h *assistantHandlers) deliverySchedule(ctx context.Context, userID int64, origin string) (deliverySchedule, error) {
	out := deliverySchedule{
		DigestFrequency: notify.FrequencyDaily,
		Note:            "account-wide, for every alert; the person changes it at " + origin + "/my/notifications/settings",
	}
	row, err := h.queries.GetNotificationSettings(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		d := notify.DefaultDigestTime
		out.DigestTime = *formatHHMM(&d)
		return out, nil
	}
	if err != nil {
		return deliverySchedule{}, err
	}
	if row.DigestFrequency != "" {
		out.DigestFrequency = row.DigestFrequency
	}
	// What is shown is what takes effect: faster than daily is a paid benefit, and a Free
	// account that kept it is sent daily (at its stored time) until it renews.
	if out.DigestFrequency != notify.FrequencyDaily {
		untils, err := h.queries.GetPlanUntils(ctx, userID)
		if err != nil {
			return deliverySchedule{}, err
		}
		if !plan.TierOf(untils.ProUntil.Time, untils.UltraUntil.Time, time.Now()).Paid() {
			out.DigestFrequency = notify.FrequencyDaily
		}
	}
	switch out.DigestFrequency {
	case notify.FrequencyDaily:
		// An interval/instant rule may carry no time; delivery reads that as the default.
		if row.DigestTime.Valid {
			out.DigestTime = clockOf(row.DigestTime)
		} else {
			d := notify.DefaultDigestTime
			out.DigestTime = *formatHHMM(&d)
		}
	case notify.FrequencyInterval:
		out.DigestIntervalHours = int(row.DigestIntervalHours)
	}
	if row.QuietHoursStart.Valid && row.QuietHoursEnd.Valid {
		out.QuietHours = clockOf(row.QuietHoursStart) + "-" + clockOf(row.QuietHoursEnd)
	}
	return out, nil
}

func clockOf(t pgtype.Time) string {
	m := t.Microseconds / 60_000_000
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
}
