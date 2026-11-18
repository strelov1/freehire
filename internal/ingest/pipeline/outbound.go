package pipeline

import (
	"context"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// OutboundResolver discovers and imports the native ATS behind a provider's
// external application. An unresolved method or destination must never fall back
// to publishing that discovery as a native application on the referring provider.
type OutboundResolver interface {
	Resolve(context.Context, sources.CompanyEntry, sources.Job) error
}
