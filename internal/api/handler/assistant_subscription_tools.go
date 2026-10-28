package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/engage/notify"
	"github.com/strelov1/freehire/internal/engage/subscription"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
	"github.com/strelov1/freehire/internal/search/savedsearch"
	"github.com/strelov1/freehire/internal/search/search"
)

// The job-alert tools of the signed-in MCP server: an agent connected for a person can
// subscribe a search to the person's alerts, list them with their settings, change them,
// remove them, and ask what they have matched. MCP cannot push, so delivery stays where it
// is — email, Telegram, the mobile app, the webhook — and these tools only manage what is
// delivered.
//
// An ALERT is what the website shows on its alerts page: one saved search, over any of the
// search filters, delivered on one or more channels. In storage that is a saved_searches
// row plus one subscriptions row per channel; the tools address the alert by its saved
// search id and name the per-channel rows only where they differ (active, pending).
//
// Every write goes through the website's own use cases — savedsearch.Service for the
// search and its name, subscription.Service for the channels and pause — so Narrows, the
// push-device rule and the per-account cap are enforced once.

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
	// subscriptionUpdatesDefaultDays and subscriptionUpdatesMaxDays bound the look-back
	// of subscription_updates. The ceiling is the notify worker's freshness window in
	// spirit: a match older than a few weeks is no longer news.
	subscriptionUpdatesDefaultDays = 7
	subscriptionUpdatesMaxDays     = 30
	subscriptionUpdatesDefaultMax  = 20
	subscriptionUpdatesMax         = 50
	// alertNameMaxRunes is savedsearch's own bound on a name.
	alertNameMaxRunes = 100
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

// deliverableChannels lists the channels that would actually reach this account now, in
// the order a default is picked from. Each is the delivery worker's own test: email is
// gated by the job-alerts mail switch (ListActiveSubscriptions), Telegram needs a linked
// chat, push a registered device, a webhook an enabled destination
// (GetSubscriptionForDelivery). A subscription on any other channel is soft-skipped on
// every pass and nobody receives it.
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
		return nil, errors.New("nothing was changed: this account has no way to receive job alerts — " +
			"alert email is turned off and no other channel is connected. Ask the person to " + alertChannelsHelp(origin))
	}
	for _, ch := range want {
		if !slices.Contains(available, ch) {
			return nil, fmt.Errorf("nothing was changed: the %s channel cannot reach this account (connected: %s). "+
				"Choose one of those, or ask the person to %s", ch, strings.Join(available, ", "), alertChannelsHelp(origin))
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

// applyAlertSearch writes the given search fields over base — a copy of the alert's
// stored query, or nothing for a new one — and returns the result. A given field replaces
// what was there: filters replaces every facet the tool can name (an empty object clears
// them), query replaces the text, a salary bound of 0 clears it. Params only the website
// sets (exclusions, match-all modes, visa and the like) are kept as they were.
//
// Unknown filter NAMES are refused (assistantFilterArgs.values, the search_jobs rule); a
// value outside a closed vocabulary is DROPPED and named, the rule the anonymous server's
// search_jobs follows: kept, it would narrow the alert to nothing, and a person would wait
// for mail that can never come.
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

// encodeSavedQuery writes the values the way the website's serializer does (facetModel's
// filtersToParams): one param per facet, its values comma-joined. The matcher reads
// either form; this one is what the website's own dedupe compares against, so a search
// saved here is recognised there as the same set.
func encodeSavedQuery(vals url.Values) string {
	flat := url.Values{}
	for k, vs := range vals {
		flat.Set(k, strings.Join(vs, ","))
	}
	return flat.Encode()
}

// parseSavedQuery reads a stored query into one value per list element, whichever list
// form it was written in.
func parseSavedQuery(query string) url.Values {
	vals, _ := url.ParseQuery(query)
	out := url.Values{}
	for k, vs := range vals {
		for _, v := range vs {
			for _, p := range strings.Split(v, ",") {
				if p = strings.TrimSpace(p); p != "" && !slices.Contains(out[k], p) {
					out[k] = append(out[k], p)
				}
			}
		}
	}
	return out
}

// sameSelection reports whether two stored queries select the same postings: the same
// params with the same value sets, in any order and either list form. The ordering
// param is ignored, as the website's canonicalQuery ignores it.
func sameSelection(a, b string) bool {
	return maps.Equal(selectionOf(a), selectionOf(b))
}

func selectionOf(query string) map[string]string {
	out := map[string]string{}
	for k, vs := range parseSavedQuery(query) {
		if k == "sort" {
			continue
		}
		sorted := slices.Sorted(slices.Values(vs))
		out[k] = strings.Join(sorted, ",")
	}
	return out
}

// filterSummary renders a stored query for a person: `"rust" · salary_min: 80000 ·
// skills: go, rust`. Params are sorted so the same search always reads the same.
func filterSummary(query string) string {
	sel := selectionOf(query)
	var parts []string
	if q, ok := sel["q"]; ok {
		parts = append(parts, fmt.Sprintf("%q", q))
		delete(sel, "q")
	}
	for _, k := range slices.Sorted(maps.Keys(sel)) {
		parts = append(parts, k+": "+strings.ReplaceAll(sel[k], ",", ", "))
	}
	return strings.Join(parts, " · ")
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
	for k, vs := range parseSavedQuery(query) {
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

// alertName names a saved search the person did not name, from its summary — the
// website names one from its filters the same way (saveSearchAlert's alertName).
func alertName(query string) string {
	name := filterSummary(query)
	if utf8.RuneCountInString(name) > alertNameMaxRunes {
		name = string([]rune(name)[:alertNameMaxRunes-1]) + "…"
	}
	if name == "" {
		return "Job alert"
	}
	return name
}

// alertSavedSearchError words savedsearch's sentinels for an agent.
func alertSavedSearchError(err error, name string) error {
	switch {
	case errors.Is(err, savedsearch.ErrDuplicateName):
		return fmt.Errorf("a saved search named %q already exists on this account — choose another name", name)
	case errors.Is(err, savedsearch.ErrCapExceeded):
		return errors.New("this account has reached its limit of saved searches — remove one on the website's " +
			"alerts page and try again")
	case errors.Is(err, savedsearch.ErrInvalidName):
		return errors.New("name must be 1-100 characters")
	case errors.Is(err, savedsearch.ErrQueryTooLong):
		return errors.New("this search has too many filters to save — narrow it to fewer values")
	}
	return err
}

// ensureSavedSearch reuses the person's saved search selecting the same postings, or
// creates one — the website's ensureSaved, server-side. An auto-generated name that
// collides gets a numbered suffix; a name the person chose is theirs to change.
func (h *assistantHandlers) ensureSavedSearch(ctx context.Context, userID int64, name, query string) (savedsearch.SavedSearch, error) {
	saved := h.savedSearches()
	existing, err := saved.List(ctx, userID)
	if err != nil {
		return savedsearch.SavedSearch{}, err
	}
	for _, s := range existing {
		if sameSelection(s.Query, query) {
			return s, nil
		}
	}
	chosen := strings.TrimSpace(name) != ""
	if !chosen {
		name = alertName(query)
	}
	created, err := saved.Create(ctx, userID, name, query, false)
	if errors.Is(err, savedsearch.ErrDuplicateName) && !chosen {
		suffix := fmt.Sprintf(" (%d)", len(existing)+1)
		base := []rune(name)
		if len(base) > alertNameMaxRunes-len(suffix) {
			base = base[:alertNameMaxRunes-len(suffix)]
		}
		created, err = saved.Create(ctx, userID, string(base)+suffix, query, false)
	}
	if err != nil {
		return savedsearch.SavedSearch{}, alertSavedSearchError(err, name)
	}
	return created, nil
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
				ID: r.SavedSearchID, Name: r.SavedSearchName, Filter: filterSummary(r.SavedSearchQuery),
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
	return alert{}, fmt.Errorf("this account has no job alert with id %d — call list_subscriptions for the ids", id)
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
				return nil, errors.New(withIgnored(unfilteredRefusal, ignored))
			}
			channels := nonEmptyStrings(in.Channels)
			available, err := h.requireDeliverable(ctx, userID, channels, origin)
			if err != nil {
				return nil, err
			}
			if len(channels) == 0 {
				channels = available[:1]
			}

			saved, err := h.ensureSavedSearch(ctx, userID, in.Name, encodeSavedQuery(vals))
			if err != nil {
				return nil, err
			}
			out := subscribeResult{IgnoredParams: ignored, ManageURL: origin + "/my/notifications/searches"}
			subs := h.subscriptions()
			for _, ch := range slices.Compact(slices.Sorted(slices.Values(channels))) {
				_, err := subs.Create(ctx, userID, saved.ID, ch)
				switch {
				case errors.Is(err, subscription.ErrDuplicate):
					out.Outcomes = append(out.Outcomes, subscribeOutcome{Channel: ch, Status: "already_subscribed"})
				case errors.Is(err, subscription.ErrUnfilteredSearch):
					return nil, errors.New(unfilteredRefusal)
				case err != nil:
					return nil, err
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

// deliverySchedule is the account-wide timing every alert shares. It is shown beside the
// alerts because it decides when they arrive, but it is not a per-alert setting — the
// website keeps it on the notification settings page, for reminders as well as alerts —
// so update_subscription does not change it.
type deliverySchedule struct {
	DigestFrequency string `json:"digest_frequency"`
	DigestTime      string `json:"digest_time,omitempty"`
	QuietHours      string `json:"quiet_hours,omitempty"`
	Note            string `json:"note"`
}

func (h *assistantHandlers) deliverySchedule(ctx context.Context, userID int64, origin string) (deliverySchedule, error) {
	out := deliverySchedule{
		DigestFrequency: "instant",
		Note:            "account-wide, for every alert; the person changes it at " + origin + "/my/notifications/settings",
	}
	row, err := h.queries.GetNotificationSettings(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return deliverySchedule{}, err
	}
	if row.DigestFrequency != "" {
		out.DigestFrequency = row.DigestFrequency
	}
	if row.DigestTime.Valid && out.DigestFrequency != "instant" {
		out.DigestTime = clockOf(row.DigestTime)
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

func (h *assistantHandlers) listSubscriptionsTool(origin string) assistant.Tool {
	return assistant.Tool{
		Name:   "list_subscriptions",
		Title:  "List job alerts",
		Effect: assistant.EffectReads,
		Description: "List the person's job alerts with every setting: id, name, a readable filter summary AND the " +
			"search as the structured object the alert tools take (query, filters, " +
			"salary_min/max; `other` holds website-only filters an update keeps), whether it is paused, and " +
			"each delivery channel with its active flag, pending matches and last delivery. delivery_schedule " +
			"is the account-wide digest timing every alert shares. Effect: none, read-only.",
		Schema: map[string]any{"type": "object", "properties": map[string]any{}},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct{}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			all, err := h.alerts(ctx, userID)
			if err != nil {
				return nil, err
			}
			schedule, err := h.deliverySchedule(ctx, userID, origin)
			if err != nil {
				return nil, err
			}
			if all == nil {
				all = []alert{}
			}
			return map[string]any{"alerts": all, "delivery_schedule": schedule}, nil
		},
	}
}

func (h *assistantHandlers) updateSubscriptionTool(origin string) assistant.Tool {
	return assistant.Tool{
		Name:   "update_subscription",
		Title:  "Change a job alert",
		Effect: assistant.EffectOverwrites,
		Description: "Change one of the person's job alerts by its id. Every field is optional and only the ones " +
			"given change: name; the search (query, filters, salary_min, salary_max — each replaces that part, " +
			"an empty filters object clears the facet filters, a salary bound of 0 clears it; website-only " +
			"filters in `other` are kept); channels (the full set to deliver on — added ones must be connected, " +
			"missing ones stop); paused (true stops every channel, false resumes them). The new search must " +
			"still narrow the catalogue, and unknown values are dropped and named in ignored_params. " +
			alertExample + ". Digest timing is account-wide and not changed here. " +
			"Effect: overwrites the alert's name, search, channels or paused state; a removed channel's record " +
			"of matches is deleted with it.",
		Schema: map[string]any{
			"type": "object",
			"properties": alertSearchSchema(map[string]any{
				"id":   map[string]any{"type": "integer", "description": "The alert id."},
				"name": map[string]any{"type": "string", "description": "A new name (1-100 characters)."},
				"channels": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string", "enum": notify.Channels},
					"description": "The complete set of channels to deliver on.",
				},
				"paused": map[string]any{"type": "boolean", "description": "true pauses every channel, false resumes them."},
			}),
			"required": []string{"id"},
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				alertSearchArgs
				ID       int64    `json:"id"`
				Name     *string  `json:"name"`
				Channels []string `json:"channels"`
				Paused   *bool    `json:"paused"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			current, err := h.ownAlert(ctx, userID, in.ID)
			if err != nil {
				return nil, err
			}

			// Everything is validated before the first write, so a refused change leaves the
			// alert exactly as it was.
			var newQuery *string
			var ignored []string
			if in.given() {
				vals, dropped, err := applyAlertSearch(parseSavedQuery(current.query), in.alertSearchArgs)
				if err != nil {
					return nil, err
				}
				ignored = dropped
				if !search.Narrows(vals) {
					return nil, errors.New(withIgnored("nothing was changed: "+unfilteredRefusal, ignored))
				}
				if q := encodeSavedQuery(vals); !sameSelection(q, current.query) {
					newQuery = &q
				}
			}
			var add, remove []alertChannel
			if raw := in.Channels; raw != nil {
				want := slices.Compact(slices.Sorted(slices.Values(nonEmptyStrings(raw))))
				if len(want) == 0 {
					return nil, errors.New("nothing was changed: channels cannot be empty — to stop the alert use unsubscribe, to hold it use paused")
				}
				for _, ch := range want {
					if !slices.ContainsFunc(current.Channels, func(c alertChannel) bool { return c.Channel == ch }) {
						add = append(add, alertChannel{Channel: ch})
					}
				}
				for _, c := range current.Channels {
					if !slices.Contains(want, c.Channel) {
						remove = append(remove, c)
					}
				}
				if len(add) > 0 {
					names := make([]string, len(add))
					for i, c := range add {
						names[i] = c.Channel
					}
					if _, err := h.requireDeliverable(ctx, userID, names, origin); err != nil {
						return nil, err
					}
				}
			}

			if in.Name != nil || newQuery != nil {
				if _, err := h.savedSearches().Update(ctx, userID, current.ID, in.Name, newQuery); err != nil {
					name := ""
					if in.Name != nil {
						name = *in.Name
					}
					return nil, alertSavedSearchError(err, name)
				}
			}
			subs := h.subscriptions()
			for _, c := range add {
				if _, err := subs.Create(ctx, userID, current.ID, c.Channel); err != nil && !errors.Is(err, subscription.ErrDuplicate) {
					return nil, err
				}
			}
			for _, c := range remove {
				if err := subs.Delete(ctx, userID, c.SubscriptionID); err != nil && !errors.Is(err, subscription.ErrNotFound) {
					return nil, err
				}
			}
			// paused applies to the channels the alert has AFTER the change, so a channel
			// added in the same call is held with the rest. Without paused, a channel added
			// to a paused alert joins it paused: the alert stays one state, not half of each.
			if in.Paused != nil || (current.Paused && len(add) > 0) {
				paused := current.Paused
				if in.Paused != nil {
					paused = *in.Paused
				}
				after, err := h.ownAlert(ctx, userID, current.ID)
				if err != nil {
					return nil, err
				}
				for _, c := range after.Channels {
					if c.Active == !paused {
						continue
					}
					if _, err := subs.SetActive(ctx, userID, c.SubscriptionID, !paused); err != nil {
						return nil, err
					}
				}
			}
			updated, err := h.ownAlert(ctx, userID, current.ID)
			if err != nil {
				return nil, err
			}
			out := map[string]any{"alert": updated}
			if len(ignored) > 0 {
				out["ignored_params"] = ignored
			}
			return out, nil
		},
	}
}

func (h *assistantHandlers) unsubscribeTool() assistant.Tool {
	return assistant.Tool{
		Name:   "unsubscribe",
		Title:  "Stop a job alert",
		Effect: assistant.EffectOverwrites,
		Description: "Stop one job alert by its id, on every channel or on the one named. " +
			"Effect: deletes the alert's delivery on those channels and their record of matches; the saved " +
			"search itself stays on the account as a bookmarked search.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":      map[string]any{"type": "integer", "description": "The alert id."},
				"channel": map[string]any{"type": "string", "enum": notify.Channels, "description": "Stop only this channel. Omit for all."},
			},
			"required": []string{"id"},
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				ID      int64  `json:"id"`
				Channel string `json:"channel"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			current, err := h.ownAlert(ctx, userID, in.ID)
			if err != nil {
				return nil, err
			}
			var stopped []string
			subs := h.subscriptions()
			for _, c := range current.Channels {
				if in.Channel != "" && c.Channel != in.Channel {
					continue
				}
				if err := subs.Delete(ctx, userID, c.SubscriptionID); err != nil && !errors.Is(err, subscription.ErrNotFound) {
					return nil, err
				}
				stopped = append(stopped, c.Channel)
			}
			if len(stopped) == 0 {
				return nil, fmt.Errorf("alert %d is not delivered on %s", in.ID, in.Channel)
			}
			return map[string]any{"id": in.ID, "stopped_channels": stopped}, nil
		},
	}
}

type subscriptionUpdate struct {
	Title     string     `json:"title"`
	Company   string     `json:"company"`
	Location  string     `json:"location,omitempty"`
	URL       string     `json:"url"`
	PostedAt  *time.Time `json:"posted_at,omitempty"`
	MatchedAt *time.Time `json:"matched_at,omitempty"`
	Delivered bool       `json:"delivered"`
	Alerts    []string   `json:"alerts"`
}

func (h *assistantHandlers) subscriptionUpdatesTool(origin string) assistant.Tool {
	return assistant.Tool{
		Name:   "subscription_updates",
		Title:  "Read what job alerts matched",
		Effect: assistant.EffectReads,
		Description: "List the new postings the person's job alerts have matched — the same matches their alerts " +
			"are built from, newest first, still-open postings only. Narrow to one alert by id, and to a window " +
			"by since (RFC 3339) or days (default 7, max 30). An alert only matches postings that appeared after " +
			"it was created, and matching runs every few minutes, so a new one starts empty. delivered says " +
			"whether an alert already carried the posting. Effect: none, read-only.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":    map[string]any{"type": "integer", "description": "One alert's id; omit for all."},
				"since": map[string]any{"type": "string", "description": "Only matches recorded at or after this RFC 3339 time."},
				"days":  map[string]any{"type": "integer", "description": fmt.Sprintf("Look back this many days (default %d, max %d). Ignored when since is given.", subscriptionUpdatesDefaultDays, subscriptionUpdatesMaxDays)},
				"limit": map[string]any{"type": "integer", "description": fmt.Sprintf("How many postings (default %d, max %d).", subscriptionUpdatesDefaultMax, subscriptionUpdatesMax)},
			},
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				ID    int64  `json:"id"`
				Since string `json:"since"`
				Days  int    `json:"days"`
				Limit int    `json:"limit"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			since, err := updatesSince(time.Now(), in.Since, in.Days)
			if err != nil {
				return nil, err
			}
			limit := in.Limit
			if limit <= 0 {
				limit = subscriptionUpdatesDefaultMax
			}
			limit = min(limit, subscriptionUpdatesMax)

			var savedID pgtype.Int8
			if in.ID != 0 {
				// Said out loud rather than answered with an empty list: the SQL is owner-scoped
				// either way, but "nothing new" for an id that is not this account's is a lie.
				if _, err := h.ownAlert(ctx, userID, in.ID); err != nil {
					return nil, err
				}
				savedID = pgtype.Int8{Int64: in.ID, Valid: true}
			}

			// Read several rows per posting asked for: one alert on two channels records each
			// posting twice, and the duplicates fold below. A full read means there may be more.
			rowLimit := limit*4 + 1
			rows, err := h.queries.ListSubscriptionUpdates(ctx, db.ListSubscriptionUpdatesParams{
				UserID: userID, SavedSearchID: savedID, Since: pgconv.Timestamptz(&since), RowLimit: int32(rowLimit),
			})
			if err != nil {
				return nil, err
			}
			out, truncated := foldUpdates(rows, origin, limit)
			truncated = truncated || len(rows) == rowLimit
			return map[string]any{"since": since.UTC().Format(time.RFC3339), "postings": out, "truncated": truncated}, nil
		},
	}
}

// updatesSince resolves the look-back window: an explicit since wins, otherwise days
// (bounded) back from now.
func updatesSince(now time.Time, since string, days int) (time.Time, error) {
	if since = strings.TrimSpace(since); since != "" {
		t, err := time.Parse(time.RFC3339, since)
		if err != nil {
			return time.Time{}, fmt.Errorf("since must be an RFC 3339 time such as 2026-10-20T00:00:00Z: %w", err)
		}
		return t, nil
	}
	if days <= 0 {
		days = subscriptionUpdatesDefaultDays
	}
	days = min(days, subscriptionUpdatesMaxDays)
	return now.AddDate(0, 0, -days), nil
}

// foldUpdates turns ledger rows into one entry per posting, newest match first, naming
// every alert that matched it. A posting is delivered once ANY of its matches was.
func foldUpdates(rows []db.ListSubscriptionUpdatesRow, origin string, limit int) ([]subscriptionUpdate, bool) {
	out := []subscriptionUpdate{}
	index := map[string]int{}
	truncated := false
	for _, r := range rows {
		if i, seen := index[r.PublicSlug]; seen {
			if !slices.Contains(out[i].Alerts, r.SavedSearchName) {
				out[i].Alerts = append(out[i].Alerts, r.SavedSearchName)
			}
			out[i].Delivered = out[i].Delivered || r.NotifiedAt.Valid
			continue
		}
		if len(out) == limit {
			truncated = true
			break
		}
		index[r.PublicSlug] = len(out)
		out = append(out, subscriptionUpdate{
			Title: r.Title, Company: r.Company, Location: r.Location,
			URL:      jobURL(origin, r.PublicSlug),
			PostedAt: pgconv.TimePtr(r.PostedAt), MatchedAt: pgconv.TimePtr(r.MatchedAt),
			Delivered: r.NotifiedAt.Valid, Alerts: []string{r.SavedSearchName},
		})
	}
	return out, truncated
}
