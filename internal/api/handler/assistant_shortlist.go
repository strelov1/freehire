package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/application/agentview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

// normalizeShortlistQuery validates the full list query before it becomes a durable
// constraint. A shortlist cannot silently ignore a condition the list reports as dropped.
func normalizeShortlistQuery(raw string) (string, error) {
	if len(raw) > 8000 {
		return "", fmt.Errorf("search query is too long")
	}
	vals, err := url.ParseQuery(strings.TrimPrefix(raw, "?"))
	if err != nil {
		return "", fmt.Errorf("invalid search query: %w", err)
	}
	for name := range vals {
		if name == "page" || name == "limit" || name == "offset" || strings.HasPrefix(name, "utm_") {
			vals.Del(name)
		}
	}
	if unknown := search.UnknownParams(vals, []string{"q", "sort", "order", "q_fields", hideSeenParam, hideAppliedCompaniesParam, formerEmployersParam, search.WatchlistParam}); len(unknown) > 0 {
		return "", fmt.Errorf("unsupported search condition: %s", unknown[0].Param)
	}
	if ignored := search.IgnoredValues(vals); len(ignored) > 0 {
		return "", fmt.Errorf("unsupported search values: %v", ignored)
	}
	if _, ignored := search.QFieldsFromValues(vals); len(ignored) > 0 {
		return "", fmt.Errorf("unsupported search fields")
	}
	for name, values := range vals {
		isFacet := false
		for _, suffix := range []string{"", "_exclude"} {
			if _, ok := search.StringFacets[strings.TrimSuffix(name, suffix)]; ok {
				isFacet = true
			}
		}
		if !isFacet && len(values) != 1 {
			return "", fmt.Errorf("search condition %s must have one value", name)
		}
		value := vals.Get(name)
		switch name {
		case "salary_min", "salary_max", "experience_years_min", "experience_years_max", "posted_within_days", "open_within_days", search.ApplyQuestionsMaxParam:
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 || search.FilterFromValues(url.Values{name: {value}}) == nil {
				return "", fmt.Errorf("invalid search bound: %s", name)
			}
		case "visa_sponsorship", hideSeenParam, hideAppliedCompaniesParam, formerEmployersParam, search.WatchlistParam, search.ExperienceIncludeUnspecifiedParam, search.SeniorityIncludeUnspecifiedParam, search.ExperienceOnlySeniorityUnspecifiedParam:
			if value != "true" && value != "false" {
				return "", fmt.Errorf("invalid search boolean: %s", name)
			}
		case "sort":
			if _, ok := searchSortable[value]; !ok && value != sortMatch {
				return "", fmt.Errorf("unsupported search sort")
			}
		case "order":
			if value != "asc" && value != "desc" {
				return "", fmt.Errorf("unsupported search order")
			}
		default:
			if !isFacet && strings.HasSuffix(name, "_mode") && value != "and" && value != "or" {
				return "", fmt.Errorf("unsupported search mode: %s", name)
			}
		}
	}
	normalized := vals.Encode()
	if len(normalized) > 8000 {
		return "", fmt.Errorf("encoded search query is too long")
	}
	return normalized, nil
}

func (h *searchHandlers) runShortlistSearch(ctx context.Context, userID int64, query string, limit, offset int) (search.SearchResult, error) {
	if h == nil || h.search == nil {
		return search.SearchResult{}, fmt.Errorf("search is not available")
	}
	normalized, err := normalizeShortlistQuery(query)
	if err != nil {
		return search.SearchResult{}, err
	}
	vals, _ := url.ParseQuery(normalized)
	exclude, ignored := h.seenExclusionFor(ctx, userID, vals)
	if ignored {
		return search.SearchResult{}, fmt.Errorf("cannot enforce hide_seen")
	}
	personal, ignored := h.appliedCompanyExclusionFor(ctx, userID, vals)
	if ignored {
		return search.SearchResult{}, fmt.Errorf("cannot enforce hide_applied_companies")
	}
	former, applied, err := h.formerEmployerSlugsFor(ctx, userID, vals)
	if err != nil {
		return search.SearchResult{}, err
	}
	if vals.Get(formerEmployersParam) == "true" && !applied {
		return search.SearchResult{}, fmt.Errorf("cannot enforce former_employers")
	}
	watch, watchApplied, err := h.watchlistSlugsFor(ctx, userID, vals)
	if err != nil {
		return search.SearchResult{}, err
	}
	if search.WantsWatchlist(vals) && !watchApplied {
		return search.SearchResult{}, fmt.Errorf("cannot enforce watchlist")
	}
	result, err := h.executeJobSearch(ctx, vals, jobSearchInput{limit: limit, offset: offset, former: former, hasFormer: applied, watch: watch, hasWatch: watchApplied, vector: h.matchVectorFor(ctx, userID, vals), exclude: exclude, personal: personal, strict: true})
	return result.res, err
}

type shortlistCandidates map[string]int64

func (c shortlistCandidates) ResolveSlugsToJobIDs(_ context.Context, slugs []string) ([]db.ResolveSlugsToJobIDsRow, error) {
	rows := make([]db.ResolveSlugsToJobIDsRow, 0, len(slugs))
	for _, slug := range slugs {
		if id, ok := c[slug]; ok {
			rows = append(rows, db.ResolveSlugsToJobIDsRow{ID: id, PublicSlug: slug})
		}
	}
	return rows, nil
}

func shortlistPresentTool(candidates shortlistCandidates, shown int) assistant.Tool {
	tool := presentJobsTool(candidates)
	tool.Schema["properties"].(map[string]any)["jobs"].(map[string]any)["maxItems"] = 5
	present := tool.Run
	tool.Run = func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
		var in presentJobsArgs
		if err := assistant.DecodeArgs(raw, &in); err != nil {
			return nil, err
		}
		if shown+len(in.Jobs) > 5 {
			return nil, fmt.Errorf("show at most five vacancies per shortlist")
		}
		for _, job := range in.Jobs {
			if _, ok := candidates[strings.TrimSpace(job.Slug)]; !ok {
				return nil, fmt.Errorf("vacancy is outside this search; call search_jobs first")
			}
		}
		result, err := present(ctx, userID, raw)
		if err == nil {
			shown += len(in.Jobs)
		}
		return result, err
	}
	return tool
}

func (h *assistantHandlers) createShortlistSession(ctx context.Context, userID int64, query, requestID string) (apigen.CreateAssistantSessionRes, error) {
	query, err := normalizeShortlistQuery(query)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	id, err := uuid.Parse(requestID)
	if err != nil || id == uuid.Nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, "a search session requires a valid request_id")
	}
	_, err = h.queries.CreateAssistantSearchSession(ctx, db.CreateAssistantSearchSessionParams{ID: id, UserID: userID, SearchQuery: pgtype.Text{String: query, Valid: true}})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	sess, err := h.store.Session(ctx, id, userID)
	if errors.Is(err, assistant.ErrNotFound) {
		return nil, fiber.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if sess.SearchContext == nil || sess.SearchContext.Query != query {
		return nil, fiber.NewError(fiber.StatusConflict, "request_id already belongs to another search")
	}
	return assistantSessionOK(sess)
}

func (h *assistantHandlers) shortlistDiscoveryTools(sess assistant.Session, shown int) []assistant.Tool {
	candidates := shortlistCandidates{}
	searchTool := assistant.Tool{
		Name: "search_jobs", Title: "Search within the current filters", Effect: assistant.EffectReads,
		Description: "Read a bounded page of vacancies under this conversation's fixed search constraints. Screen their full descriptions using the candidate's saved preferences. The query and filters cannot be changed here.",
		Schema:      map[string]any{"type": "object", "properties": map[string]any{"offset": map[string]any{"type": "integer", "minimum": 0}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": assistantMaxSearchLimit}}, "additionalProperties": false},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				Offset int  `json:"offset"`
				Limit  *int `json:"limit"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if userID != sess.UserID {
				return nil, assistant.ErrNotFound
			}
			limit := 5
			if in.Limit != nil {
				limit = *in.Limit
			}
			if limit < 1 || limit > assistantMaxSearchLimit {
				return nil, fmt.Errorf("limit must be between 1 and %d", assistantMaxSearchLimit)
			}
			if in.Offset < 0 || in.Offset > maxPageWindow-limit {
				return nil, fmt.Errorf("search page is out of range")
			}
			result, err := h.search.runShortlistSearch(ctx, userID, sess.SearchContext.Query, limit, in.Offset)
			if err != nil {
				return nil, err
			}
			if len(result.Hits) > 0 && h.search.descriptions == nil {
				return nil, fmt.Errorf("full vacancy descriptions are not available")
			}
			jobs, err := h.search.markdownJobs(ctx, result.Hits)
			if err != nil {
				return nil, err
			}
			payload := map[string]any{"total": result.Total, "offset": in.Offset, "limit": limit, "jobs": jobs}
			encoded, err := json.Marshal(payload)
			if err != nil {
				return nil, err
			}
			if len(encoded) > assistantResultCap {
				return nil, fmt.Errorf("full descriptions exceed the result budget; retry with a smaller limit, or advance offset past an oversized posting")
			}
			for _, hit := range result.Hits {
				candidates[hit.PublicSlug] = hit.ID
			}
			agentview.Record(ctx, agentview.KindSearch, hitIDs(result.Hits))
			return payload, nil
		},
	}
	profileTool := h.getProfileTool()
	readProfile := profileTool.Run
	profileTool.Run = func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
		result, err := readProfile(ctx, userID, raw)
		if missing, ok := result.(noProfileResult); ok {
			missing.Next = "No saved profile. Continue shortlisting using the captured search and the candidate's stated preferences; ask at most one consequential clarification when necessary."
			return missing, err
		}
		return result, err
	}
	return []assistant.Tool{profileTool, searchTool, shortlistPresentTool(candidates, shown), h.searchPreferencesTool(sess)}
}

func (h *assistantHandlers) searchPreferencesTool(sess assistant.Session) assistant.Tool {
	return assistant.Tool{
		Name: "remember_search_preferences", Title: "Remember preferences for this search", Effect: assistant.EffectOverwrites,
		Description: "Replace this conversation's selection brief with a compact account of the candidate's stated preferences, retaining earlier preferences unless corrected. Record exceptions as specific to their vacancy. Never save inferred candidate facts, change the initial search, or modify the global profile. At most 2000 characters.",
		Schema:      map[string]any{"type": "object", "properties": map[string]any{"brief": map[string]any{"type": "string", "maxLength": 2000}}, "required": []string{"brief"}, "additionalProperties": false},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				Brief string `json:"brief"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if userID != sess.UserID {
				return nil, assistant.ErrNotFound
			}
			in.Brief = strings.TrimSpace(in.Brief)
			if utf8.RuneCountInString(in.Brief) > 2000 {
				return nil, fmt.Errorf("selection preferences exceed 2000 characters")
			}
			count, err := h.queries.UpdateAssistantSelectionBrief(ctx, db.UpdateAssistantSelectionBriefParams{ID: sess.ID, UserID: userID, SelectionBrief: in.Brief})
			if err != nil {
				return nil, err
			}
			if count == 0 {
				return nil, assistant.ErrNotFound
			}
			return map[string]any{"saved": true}, nil
		},
	}
}

// shortlistShown restores the successful presentation count for the last user prompt.
// Retry continues that prompt rather than creating a new shortlist allowance.
func shortlistShown(transcript []assistant.Message) (int, error) {
	count := 0
	for _, message := range transcript {
		if message.Role == assistant.RoleUser {
			count = 0
			continue
		}
		if message.Role != assistant.RoleTool {
			continue
		}
		decoded, err := message.Decode()
		if err != nil {
			return 0, err
		}
		for _, part := range decoded.Parts {
			reply, ok := part.(llms.ToolCallResponse)
			if !ok || reply.Name != presentJobsToolName {
				continue
			}
			var receipt presentJobsResult
			if err := json.Unmarshal([]byte(reply.Content), &receipt); err != nil {
				return 0, err
			}
			count += len(receipt.Presented)
		}
	}
	return count, nil
}
