package main

import (
	"context"
	"net/http"
	"testing"
)

func TestBeehireProbe(t *testing.T) {
	const api = "https://app.beehire.com/users/getPublicCampaigns/"
	g := statusGetter{
		fakeGetter: fakeGetter{
			api + "actemium": `{
				"campaigns": [{"_id": "a"}, {"_id": "b"}],
				"user": {"company": "VINCI Energies Belgium"},
				"employerBranding": {"companyInformation": {"name": "ACTEMIUM"}}
			}`,
			api + "qplus-consult": `{"campaigns":[],"employerBranding":{"companyInformation":{"name":"Qplus"}}}`,
			api + "nameless":      `{"campaigns":[{"_id":"a"}],"user":{"company":"Fallback Co"}}`,
			api + "reshaped":      `{"user":{}}`,
		},
		status: map[string]int{
			api + "missing": http.StatusNotFound,
		},
	}
	p := beehireProber{}
	if name, n, err := p.probe(context.Background(), g, "Actemium"); err != nil || name != "ACTEMIUM" || n != 2 {
		t.Errorf("live: got (%q,%d,%v), want (ACTEMIUM,2,nil)", name, n, err)
	}
	if name, n, err := p.probe(context.Background(), g, "nameless"); err != nil || name != "Fallback Co" || n != 1 {
		t.Errorf("fallback name: got (%q,%d,%v), want (Fallback Co,1,nil)", name, n, err)
	}
	for _, board := range []string{"qplus-consult", "missing", "../x"} {
		if _, n, err := p.probe(context.Background(), g, board); err != nil || n != 0 {
			t.Errorf("%s: got n=%d err=%v, want a skip", board, n, err)
		}
	}
	if _, _, err := p.probe(context.Background(), g, "reshaped"); err == nil {
		t.Error("a listing without campaigns must be surfaced")
	}
	if _, _, err := p.probe(context.Background(), fakeGetter{}, "refused"); err == nil {
		t.Error("a transport failure must be surfaced")
	}
}
