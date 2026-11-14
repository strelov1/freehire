package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/catalogexport"
)

type fakeEntitlements db.GetUserAPIEntitlementRow

func (f fakeEntitlements) GetUserAPIEntitlement(context.Context, int64) (db.GetUserAPIEntitlementRow, error) {
	return db.GetUserAPIEntitlementRow(f), nil
}

// fakeSnapshots is an in-memory blobstore.Bucket holding one finished snapshot.
type fakeSnapshots struct{}

const fakeManifest = `{"schema_version":1,"date":"2026-11-13","rows":3,"bytes":10,"sha256":"ab","generated_at":"2026-11-13T04:00:00Z"}`

func (fakeSnapshots) Put(context.Context, string, string, io.Reader, int64) error { return nil }
func (fakeSnapshots) PutStream(context.Context, string, string, io.Reader) error  { return nil }
func (fakeSnapshots) Delete(context.Context, string) error                        { return nil }
func (fakeSnapshots) Get(_ context.Context, key string) (io.ReadCloser, error) {
	if key != catalogexport.ManifestKey("2026-11-13") {
		return nil, errors.New("not found")
	}
	return io.NopCloser(strings.NewReader(fakeManifest)), nil
}
func (fakeSnapshots) List(context.Context, string) ([]string, error) {
	return []string{catalogexport.DataKey("2026-11-13"), catalogexport.ManifestKey("2026-11-13")}, nil
}
func (fakeSnapshots) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://bucket.test/" + key, nil
}

func TestListCatalogueSnapshots(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	future := pgtype.Timestamptz{Time: now.Add(time.Hour), Valid: true}
	ctx := context.WithValue(context.Background(), auth.LocalsUserID, int64(7)) //nolint:staticcheck // Fiber Locals keys are plain strings

	for _, tc := range []struct {
		name   string
		row    db.GetUserAPIEntitlementRow
		bucket bool
		status int
	}{
		{"no plan", db.GetUserAPIEntitlementRow{}, true, http.StatusForbidden},
		{"api tier", db.GetUserAPIEntitlementRow{ApiUntilStripe: future}, true, http.StatusForbidden},
		{"pro is not api", db.GetUserAPIEntitlementRow{ProUntil: future}, true, http.StatusForbidden},
		{"data, no bucket", db.GetUserAPIEntitlementRow{ApiDataUntilStripe: future}, false, http.StatusNotImplemented},
		{"data", db.GetUserAPIEntitlementRow{ApiDataUntilStripe: future}, true, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &catalogueHandlers{entitlements: fakeEntitlements(tc.row), now: func() time.Time { return now }}
			if tc.bucket {
				h.bucket = fakeSnapshots{}
			}
			res, err := h.ListCatalogueSnapshots(ctx)
			if tc.status != http.StatusOK {
				var fe *fiber.Error
				if !errors.As(err, &fe) || fe.Code != tc.status {
					t.Fatalf("err = %v, want status %d", err, tc.status)
				}
				return
			}
			if err != nil || len(res.Data) != 1 {
				t.Fatalf("res = %+v, err = %v", res, err)
			}
			s := res.Data[0]
			if s.Date != "2026-11-13" || s.Rows != 3 || s.DownloadURL != "https://bucket.test/catalogue/2026-11-13/jobs.jsonl.gz" ||
				!s.URLExpiresAt.Equal(now.Add(catalogueURLTTL)) {
				t.Fatalf("snapshot = %+v", s)
			}
		})
	}
}

func TestListCatalogueSnapshotsNeedsCaller(t *testing.T) {
	h := &catalogueHandlers{entitlements: fakeEntitlements{}, bucket: fakeSnapshots{}, now: time.Now}
	var fe *fiber.Error
	if _, err := h.ListCatalogueSnapshots(context.Background()); !errors.As(err, &fe) || fe.Code != http.StatusUnauthorized {
		t.Fatalf("err = %v", err)
	}
}
