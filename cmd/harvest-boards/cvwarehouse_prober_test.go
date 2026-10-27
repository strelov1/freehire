package main

import (
	"context"
	"testing"
)

func TestCVWarehouseProbe(t *testing.T) {
	const flat = "78373ed3-eb9b-4bea-821b-367667e8825f"
	const sectioned = "0899d23b-6cd3-4e2e-8618-94db8719a1c4"
	g := fakeGetter{
		// The shapes of the pages as captured 2026-10-27, trimmed to the markers the probe reads.
		cvwarehouseHome + flat: `<html><head><meta name="author" content="agap2IT"></head><body class="home  " data-scroll="0">
			<a class="jobLink" data-item="readmore" data-jobid="402573" href="?job=402573">A</a>
			<a class="btn btn-readmore" data-item="readmore" data-jobid="402573">read</a>
			<a class="jobLink" data-item="readmore" data-jobid="396844" href="?job=396844">B</a>
			<div data-jobdetail-job-id="401708"></div></body></html>`,
		cvwarehouseHome + sectioned: `<html><head><meta name="author" content="De Zilveren Zwaan"></head><body class="home  ">
			<a href="/?companyGuid=` + sectioned + `&amp;lang=nl-BE&amp;section=05c750e4-b056-5260-0d56-9913992e40a0">Zorg</a>
			<a href="/?companyGuid=` + sectioned + `&amp;lang=nl-BE&amp;section=dcdb0dba-31c2-cc14-9b97-6b07a0f0f5c8">Andere</a></body></html>`,
		cvwarehouseHome + sectioned + "&section=05c750e4-b056-5260-0d56-9913992e40a0": `<body class="home"><a class="jobLink" data-jobid="372829">Z</a><a class="jobLink" data-jobid="372826">Y</a></body>`,
		cvwarehouseHome + sectioned + "&section=dcdb0dba-31c2-cc14-9b97-6b07a0f0f5c8": `<body class="home"></body>`,
		cvwarehouseHome + "00000000-1111-2222-3333-444444444444":                      `<html><head><meta name="author" content="CVWarehouse"></head><body role="document  ">Job niet beschikbaar</body></html>`,
	}
	p := cvwarehouseProber{}
	if name, n, err := p.probe(context.Background(), g, flat); err != nil || name != "agap2IT" || n != 2 {
		t.Errorf("flat: got (%q,%d,%v), want (agap2IT,2,nil)", name, n, err)
	}
	if name, n, err := p.probe(context.Background(), g, sectioned); err != nil || name != "De Zilveren Zwaan" || n != 2 {
		t.Errorf("sectioned: got (%q,%d,%v), want (De Zilveren Zwaan,2,nil)", name, n, err)
	}
	for _, board := range []string{"00000000-1111-2222-3333-444444444444", "acme"} {
		if _, n, err := p.probe(context.Background(), g, board); err != nil || n != 0 {
			t.Errorf("%s: got n=%d err=%v, want a skip", board, n, err)
		}
	}
}
