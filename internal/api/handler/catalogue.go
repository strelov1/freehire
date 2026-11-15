package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/identity/billing"
	"github.com/strelov1/freehire/internal/platform/blobstore"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/catalogexport"
)

// catalogueURLTTL is how long a download link works. The files are multi-GB, so the link has
// to outlive the start of a slow download, but a leaked one should not stay useful.
const catalogueURLTTL = 15 * time.Minute

type entitlementReader interface {
	GetUserAPIEntitlement(context.Context, int64) (db.GetUserAPIEntitlementRow, error)
}

// catalogueHandlers serves the bulk catalogue snapshots to the API "Data" plan. A nil bucket
// (CATALOGUE_EXPORT_S3_* unset) answers 501.
type catalogueHandlers struct {
	entitlements entitlementReader
	bucket       blobstore.Bucket
	now          func() time.Time
}

func newCatalogueHandlers(q *db.Queries, bucket blobstore.Bucket) *catalogueHandlers {
	return &catalogueHandlers{entitlements: q, bucket: bucket, now: time.Now}
}

func (h *catalogueHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	api.Get("/catalogue/snapshots", mw.key, contract)
}

func (h *catalogueHandlers) ListCatalogueSnapshots(ctx context.Context) (*apigen.ListCatalogueSnapshotsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	row, err := h.entitlements.GetUserAPIEntitlement(ctx, userID)
	if err != nil {
		return nil, err
	}
	now := h.now()
	if apiPlanFrom(row, now).Tier != billing.APITierData {
		return nil, fiber.NewError(http.StatusForbidden, "bulk catalogue files are part of the Data plan")
	}
	if h.bucket == nil {
		return nil, fiber.NewError(http.StatusNotImplemented, "bulk catalogue files are not set up on this deployment")
	}
	snaps, err := catalogexport.List(ctx, h.bucket)
	if err != nil {
		return nil, err
	}
	out := make([]apigen.CatalogueSnapshot, len(snaps))
	for i, m := range snaps {
		url, err := h.bucket.PresignGet(ctx, catalogexport.DataKey(m.Date), catalogueURLTTL)
		if err != nil {
			return nil, err
		}
		out[i] = apigen.CatalogueSnapshot{
			Date: m.Date, SchemaVersion: int32(m.SchemaVersion), Rows: m.Rows, Bytes: m.Bytes,
			SHA256: m.SHA256, GeneratedAt: m.GeneratedAt, DownloadURL: url, URLExpiresAt: now.Add(catalogueURLTTL).UTC(),
		}
	}
	return &apigen.ListCatalogueSnapshotsOK{Data: out}, nil
}
