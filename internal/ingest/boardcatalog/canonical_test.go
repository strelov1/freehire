package boardcatalog

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

type sitesGetter struct{}

func (sitesGetter) GetJSON(_ context.Context, _ string, out any) error {
	return json.Unmarshal([]byte(`{"hasMore":false,"items":[{"SiteNumber":"CX_1","StatusCode":"ORA_ACTIVE"}]}`), out)
}

// recordRepo echoes what Insert hands it, so a test sees the canonical board and the status.
type recordRepo struct{ fakeRepo }

func (recordRepo) InsertRow(_ context.Context, in InsertInput, st Status, reason string) (Board, error) {
	return Board{Board: in.Board, Status: st, RejectedReason: reason}, nil
}

func TestInsertCanonicalisesAndRefusesOracleSite(t *testing.T) {
	reg := map[string]sources.Source{"oracle": sources.NewOracle(sitesGetter{})}
	ins := NewInserter(&recordRepo{}, reg)
	in := InsertInput{Provider: "oracle", Company: "Acme", Board: "a.fa.us2.oraclecloud.com/cx_1"}

	b, err := ins.Insert(context.Background(), in, StatusPending)
	if err != nil || b.Status != StatusPending || b.Board != "a.fa.us2.oraclecloud.com/CX_1" {
		t.Errorf("listed site: %+v, %v", b, err)
	}
	in.Board = "a.fa.us2.oraclecloud.com/jobsearch"
	b, err = ins.Insert(context.Background(), in, StatusPending)
	if err != nil || b.Status != StatusRejected || b.RejectedReason == "" {
		t.Errorf("unlisted site: %+v, %v", b, err)
	}
}
