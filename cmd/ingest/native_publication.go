package main

import (
	"context"

	"github.com/strelov1/freehire/internal/platform/db"
)

func (d *nativeDiscovery) retireCoveredDiscovery(ctx context.Context, previous db.Job) error {
	if _, err := d.store.CloseCovered(ctx, "linkedin", []string{previous.ExternalID}); err != nil {
		return err
	}
	return d.q.RetirePendingLinkedInDiscovery(ctx, previous.ID)
}
