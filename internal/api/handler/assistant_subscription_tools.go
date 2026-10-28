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
	"strings"
	"time"
	"unicode/utf8"

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
// subscribe a search to the person's alerts, list and remove those subscriptions, and ask
// what they have matched. MCP cannot push, so delivery stays where it is — email,
// Telegram, the mobile app, the webhook — and these tools only manage what is delivered.
//
// A subscription here is made exactly the way the website's alert button makes one: a
// saved search (reused when the person already has one selecting the same postings),
// then savedsearch.Service and subscription.Service — the same use cases the HTTP routes
// call, so Narrows, the push-device rule and the per-account cap are enforced once.

// subscriptionMCPTools is the set the account server adds for job alerts. Not offered to
// the in-app assistant: the browser there already has the alert controls beside the
// filters. origin builds the links an answer hands back.
func (h *assistantHandlers) subscriptionMCPTools(origin string) []assistant.Tool {
	return []assistant.Tool{
		h.subscribeTool(origin),
		h.listSubscriptionsTool(),
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

// subscriptionSearch turns the tool's search into the stored query string a saved search
// holds. Unknown filter NAMES are refused (assistantFilterArgs.values, the search_jobs
// rule); a value outside a closed vocabulary is DROPPED and named, the rule the anonymous
// server's search_jobs follows: kept, it would narrow the alert to nothing, and a person
// would wait for mail that can never come.
func subscriptionSearch(query string, filters assistantFilterArgs) (url.Values, []string, error) {
	vals, err := filters.values()
	if err != nil {
		return nil, nil, err
	}
	var ignored []string
	out := url.Values{}
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
	if q := strings.TrimSpace(query); q != "" {
		out.Set("q", q)
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

// sameSelection reports whether two stored queries select the same postings: the same
// params with the same value sets, in any order and either list form. The ordering
// param is ignored, as the website's canonicalQuery ignores it.
func sameSelection(a, b string) bool {
	return maps.Equal(selectionOf(a), selectionOf(b))
}

func selectionOf(query string) map[string]string {
	vals, _ := url.ParseQuery(query)
	out := map[string]string{}
	for k, vs := range vals {
		if k == "sort" {
			continue
		}
		var parts []string
		for _, v := range vs {
			for _, p := range strings.Split(v, ",") {
				if p = strings.TrimSpace(p); p != "" && !slices.Contains(parts, p) {
					parts = append(parts, p)
				}
			}
		}
		if len(parts) == 0 {
			continue
		}
		sort.Strings(parts)
		out[k] = strings.Join(parts, ",")
	}
	return out
}

// filterSummary renders a stored query for a person: `"rust" · skills: go, rust ·
// company_slug: stripe`. Params are sorted so the same search always reads the same.
func filterSummary(query string) string {
	sel := selectionOf(query)
	var parts []string
	if q, ok := sel["q"]; ok {
		parts = append(parts, fmt.Sprintf("%q", q))
		delete(sel, "q")
	}
	keys := make([]string, 0, len(sel))
	for k := range sel {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		parts = append(parts, k+": "+strings.ReplaceAll(sel[k], ",", ", "))
	}
	return strings.Join(parts, " · ")
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
	switch {
	case errors.Is(err, savedsearch.ErrDuplicateName):
		return savedsearch.SavedSearch{}, fmt.Errorf("a saved search named %q already exists on this account — choose another name", name)
	case errors.Is(err, savedsearch.ErrCapExceeded):
		return savedsearch.SavedSearch{}, errors.New("this account has reached its limit of saved searches — " +
			"remove one (unsubscribe does not remove the search itself; the website's saved searches page does) and try again")
	case errors.Is(err, savedsearch.ErrInvalidName):
		return savedsearch.SavedSearch{}, errors.New("name must be 1-100 characters")
	case errors.Is(err, savedsearch.ErrQueryTooLong):
		return savedsearch.SavedSearch{}, errors.New("this search has too many filters to save — narrow it to fewer values")
	}
	return created, err
}

type subscribeResult struct {
	SavedSearchID int64              `json:"saved_search_id"`
	Name          string             `json:"name"`
	Filter        string             `json:"filter"`
	Subscriptions []subscribeOutcome `json:"subscriptions"`
	IgnoredParams []string           `json:"ignored_params,omitempty"`
	ManageURL     string             `json:"manage_url"`
}

type subscribeOutcome struct {
	ID      int64  `json:"id,omitempty"`
	Channel string `json:"channel"`
	// Status is created or already_subscribed.
	Status string `json:"status"`
}

func (h *assistantHandlers) subscribeTool(origin string) assistant.Tool {
	return assistant.Tool{
		Name:   "subscribe",
		Title:  "Subscribe to a job search",
		Effect: assistant.EffectAdds,
		Description: "Get new postings matching a search delivered to the person as job alerts, on the channels " +
			"their account has connected (email, Telegram, the mobile app, a webhook) — this conversation is not " +
			"notified. Takes the same query and filters as search_jobs; run that search first so the person sees " +
			"what they are subscribing to. The search must narrow the catalogue. Values that are not in a " +
			"filter's vocabulary are dropped and named in ignored_params. Leave channels empty for the default " +
			"(email when alert mail is on, otherwise the first connected channel). " +
			"Effect: saves the search on the account (or reuses the one already selecting the same postings) " +
			"and adds an alert subscription per channel; nothing existing is changed.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":   map[string]any{"type": "string", "description": "Free-text query, as in search_jobs."},
				"filters": assistantFilterSchema("Facet filters, as in search_jobs."),
				"name":    map[string]any{"type": "string", "description": "A name for the alert (1-100 characters). Omit to name it from its filters."},
				"channels": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string", "enum": notify.Channels},
					"description": "Where to deliver. Only a channel the account has connected is accepted.",
				},
			},
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				Query    string              `json:"query"`
				Filters  assistantFilterArgs `json:"filters"`
				Name     string              `json:"name"`
				Channels []string            `json:"channels"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			vals, ignored, err := subscriptionSearch(in.Query, in.Filters)
			if err != nil {
				return nil, err
			}
			// Checked before anything is saved, so a refused alert leaves no saved search
			// behind. subscription.Service.Create asks the same question again.
			if !search.Narrows(vals) {
				msg := "add at least one filter or a query before subscribing — an unfiltered search would notify the person about every job"
				if len(ignored) > 0 {
					msg += "; these values were not understood and dropped: " + strings.Join(ignored, ", ")
				}
				return nil, errors.New(msg)
			}

			state, err := h.queries.GetAlertChannelState(ctx, userID)
			if err != nil {
				return nil, err
			}
			available := deliverableChannels(state)
			if len(available) == 0 {
				return nil, errors.New("no subscription was created: this account has no way to receive job alerts — " +
					"alert email is turned off and no other channel is connected. Ask the person to " + alertChannelsHelp(origin))
			}
			channels := nonEmptyStrings(in.Channels)
			if len(channels) == 0 {
				channels = available[:1]
			}
			for _, ch := range channels {
				if !slices.Contains(available, ch) {
					return nil, fmt.Errorf("nothing was created: the %s channel cannot reach this account (connected: %s). "+
						"Choose one of those, or ask the person to %s", ch, strings.Join(available, ", "), alertChannelsHelp(origin))
				}
			}

			query := encodeSavedQuery(vals)
			saved, err := h.ensureSavedSearch(ctx, userID, in.Name, query)
			if err != nil {
				return nil, err
			}
			subs := h.subscriptions()
			out := subscribeResult{
				SavedSearchID: saved.ID,
				Name:          saved.Name,
				Filter:        filterSummary(saved.Query),
				IgnoredParams: ignored,
				ManageURL:     origin + "/my/notifications/searches",
			}
			for _, ch := range slices.Compact(slices.Sorted(slices.Values(channels))) {
				sub, err := subs.Create(ctx, userID, saved.ID, ch)
				switch {
				case errors.Is(err, subscription.ErrDuplicate):
					out.Subscriptions = append(out.Subscriptions, subscribeOutcome{Channel: ch, Status: "already_subscribed"})
				case errors.Is(err, subscription.ErrUnfilteredSearch):
					return nil, errors.New("add at least one filter or a query before subscribing — an unfiltered search would notify the person about every job")
				case err != nil:
					return nil, err
				default:
					out.Subscriptions = append(out.Subscriptions, subscribeOutcome{ID: sub.ID, Channel: ch, Status: "created"})
				}
			}
			return out, nil
		},
	}
}

type subscriptionItem struct {
	ID              int64      `json:"id"`
	Name            string     `json:"name"`
	Filter          string     `json:"filter"`
	Channel         string     `json:"channel"`
	Active          bool       `json:"active"`
	CreatedAt       *time.Time `json:"created_at,omitempty"`
	Pending         int64      `json:"pending"`
	LastDeliveredAt *time.Time `json:"last_delivered_at,omitempty"`
}

func (h *assistantHandlers) listSubscriptionsTool() assistant.Tool {
	return assistant.Tool{
		Name:   "list_subscriptions",
		Title:  "List job alert subscriptions",
		Effect: assistant.EffectReads,
		Description: "List the person's job alert subscriptions: id, name, what the search selects, the delivery " +
			"channel, whether it is active, how many matches wait for the next alert (pending) and when one was " +
			"last delivered. One search delivered on two channels is two subscriptions. Effect: none, read-only.",
		Schema: map[string]any{"type": "object", "properties": map[string]any{}},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct{}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
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
			out := make([]subscriptionItem, len(rows))
			for i, r := range rows {
				st := byID[r.ID]
				out[i] = subscriptionItem{
					ID: r.ID, Name: r.SavedSearchName, Filter: filterSummary(r.SavedSearchQuery),
					Channel: r.Channel, Active: r.Active, CreatedAt: r.CreatedAt,
					Pending: st.Pending, LastDeliveredAt: pgconv.TimePtr(st.LastDeliveredAt),
				}
			}
			return map[string]any{"subscriptions": out}, nil
		},
	}
}

func (h *assistantHandlers) unsubscribeTool() assistant.Tool {
	return assistant.Tool{
		Name:   "unsubscribe",
		Title:  "Unsubscribe from a job alert",
		Effect: assistant.EffectOverwrites,
		Description: "Stop one job alert subscription, by the id list_subscriptions gives. " +
			"Effect: deletes that subscription and its record of matches; the saved search it delivered stays " +
			"on the account, and the same search on another channel keeps being delivered.",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"id": map[string]any{"type": "integer", "description": "The subscription id."}},
			"required":   []string{"id"},
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				ID int64 `json:"id"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			err := h.subscriptions().Delete(ctx, userID, in.ID)
			if errors.Is(err, subscription.ErrNotFound) {
				return nil, fmt.Errorf("this account has no subscription with id %d — call list_subscriptions for the ids", in.ID)
			}
			if err != nil {
				return nil, err
			}
			return map[string]any{"unsubscribed": true, "id": in.ID}, nil
		},
	}
}

type subscriptionUpdate struct {
	Title         string     `json:"title"`
	Company       string     `json:"company"`
	Location      string     `json:"location,omitempty"`
	URL           string     `json:"url"`
	PostedAt      *time.Time `json:"posted_at,omitempty"`
	MatchedAt     *time.Time `json:"matched_at,omitempty"`
	Delivered     bool       `json:"delivered"`
	Subscriptions []string   `json:"subscriptions"`
}

func (h *assistantHandlers) subscriptionUpdatesTool(origin string) assistant.Tool {
	return assistant.Tool{
		Name:   "subscription_updates",
		Title:  "Read what job alerts matched",
		Effect: assistant.EffectReads,
		Description: "List the new postings the person's job alerts have matched — the same matches their alerts " +
			"are built from, newest first, still-open postings only. Narrow to one subscription by id, and to a " +
			"window by since (RFC 3339) or days (default 7, max 30). A subscription only matches postings that " +
			"appeared after it was created, and matching runs every few minutes, so a new one starts empty. " +
			"delivered says whether an alert already carried the posting. Effect: none, read-only.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"subscription_id": map[string]any{"type": "integer", "description": "One subscription's id; omit for all."},
				"since":           map[string]any{"type": "string", "description": "Only matches recorded at or after this RFC 3339 time."},
				"days":            map[string]any{"type": "integer", "description": fmt.Sprintf("Look back this many days (default %d, max %d). Ignored when since is given.", subscriptionUpdatesDefaultDays, subscriptionUpdatesMaxDays)},
				"limit":           map[string]any{"type": "integer", "description": fmt.Sprintf("How many postings (default %d, max %d).", subscriptionUpdatesDefaultMax, subscriptionUpdatesMax)},
			},
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				SubscriptionID int64  `json:"subscription_id"`
				Since          string `json:"since"`
				Days           int    `json:"days"`
				Limit          int    `json:"limit"`
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

			var subID pgtype.Int8
			if in.SubscriptionID != 0 {
				// Said out loud rather than answered with an empty list: the SQL is owner-scoped
				// either way, but "nothing new" for an id that is not this account's is a lie.
				owned, err := h.subscriptions().List(ctx, userID)
				if err != nil {
					return nil, err
				}
				if !slices.ContainsFunc(owned, func(s subscription.SubscriptionListItem) bool { return s.ID == in.SubscriptionID }) {
					return nil, fmt.Errorf("this account has no subscription with id %d — call list_subscriptions for the ids", in.SubscriptionID)
				}
				subID = pgtype.Int8{Int64: in.SubscriptionID, Valid: true}
			}

			// Read several rows per posting asked for: one search on two channels records each
			// posting twice, and the duplicates fold below. A full read means there may be more.
			rowLimit := limit*4 + 1
			rows, err := h.queries.ListSubscriptionUpdates(ctx, db.ListSubscriptionUpdatesParams{
				UserID: userID, SubscriptionID: subID, Since: pgconv.Timestamptz(&since), RowLimit: int32(rowLimit),
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
// every subscription that matched it. A posting is delivered once ANY of its matches was.
func foldUpdates(rows []db.ListSubscriptionUpdatesRow, origin string, limit int) ([]subscriptionUpdate, bool) {
	out := []subscriptionUpdate{}
	index := map[string]int{}
	truncated := false
	for _, r := range rows {
		if i, seen := index[r.PublicSlug]; seen {
			if !slices.Contains(out[i].Subscriptions, r.SavedSearchName) {
				out[i].Subscriptions = append(out[i].Subscriptions, r.SavedSearchName)
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
			Delivered: r.NotifiedAt.Valid, Subscriptions: []string{r.SavedSearchName},
		})
	}
	return out, truncated
}
