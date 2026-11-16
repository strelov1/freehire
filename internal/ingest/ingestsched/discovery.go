package ingestsched

import (
	"context"

	"github.com/strelov1/freehire/internal/platform/db"
)

// AdoptDiscoveredProvider schedules a new, verified native ATS using the fleet's
// existing defaults. An operator's existing override is never modified.
func (r *QueriesRepository) AdoptDiscoveredProvider(ctx context.Context, provider string) error {
	if err := ValidateProviderKey(provider); err != nil {
		return err
	}
	return r.q.AdoptDiscoveredIngestProvider(ctx, db.AdoptDiscoveredIngestProviderParams{
		Provider: provider, DefaultShards: int32(DefaultShards),
		DefaultCadenceSec: int32(DefaultCadence.Seconds()), DefaultTimeoutSec: int32(DefaultRunTimeout.Seconds()),
	})
}
