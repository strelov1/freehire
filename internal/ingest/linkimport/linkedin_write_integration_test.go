//go:build integration

package linkimport

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestWriteNativeRefusesPrivateDestination(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	q := db.New(pool)
	const key = "gistworld.mua.hrdepartment.com:3735"
	if _, err := pool.Exec(ctx, `INSERT INTO jobs(source,external_id,url,title,public_slug,is_private,description) VALUES('hrsmart',$1,'https://gistworld.mua.hrdepartment.com/hrsmart/ats/Posting/view/3735','Private Java role','private-role',true,'Private description')`, key); err != nil {
		t.Fatal(err)
	}
	im := New(pool, q, nil, nil, sources.All(nil), nil)
	_, err := im.WriteNative(ctx, nil, NativeDestination{Entry: sources.CompanyEntry{Provider: "hrsmart", Board: "gistworld.mua.hrdepartment.com", Company: "Gist Limited"}, Job: sources.Job{ExternalID: key, URL: "https://gistworld.mua.hrdepartment.com/hrsmart/ats/Posting/view/3735", Title: "Java Engineer", Company: "Gist Limited", Description: "Public description"}})
	if err == nil {
		t.Fatal("private destination was rewritten")
	}
	row, err := q.GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{Source: "hrsmart", ExternalID: key})
	if err != nil || !row.IsPrivate || row.Description != "Private description" {
		t.Fatalf("private row changed: %+v, %v", row, err)
	}
}
