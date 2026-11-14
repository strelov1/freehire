package handler

import (
	"context"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/api/apigen"
)

// GetAutoApplyList shares the assistant's caller-scoped list and status derivation.
func (h *assistantHandlers) GetAutoApplyList(ctx context.Context) (*apigen.GetAutoApplyListOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	list, err := h.autoApplyList(ctx, userID, pgtype.Int8{})
	if err != nil {
		return nil, err
	}
	items := make([]apigen.AutoApplyListItem, 0, len(list.Attempts))
	for _, item := range list.Attempts {
		items = append(items, apigen.AutoApplyListItem{
			JobSlug: item.JobSlug, Title: item.Title, Company: item.Company,
			Status: apigen.AutoApplyListStatus(item.Status), LastActivityAt: item.LastActivityAt,
		})
	}
	return &apigen.GetAutoApplyListOK{Data: apigen.AutoApplyList{Attempts: items}}, nil
}
