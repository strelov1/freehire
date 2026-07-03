package main

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestRecordToParams_Full(t *testing.T) {
	rec := record{
		Name:             "Acme Corp",
		HomepageURI:      "acme.com",
		HQCountry:        "US",
		ParentCompany:    "Acme Holdings",
		Subsidiaries:     []string{"Acme Labs"},
		Industries:       []string{"Software", "Fintech"},
		Activities:       []string{"Builds widgets"},
		NbEmployees:      500,
		YearFounded:      1999,
		Tagline:          "We do things",
		OrganizationType: "Private",
		FundingType:      "Series B",
		FundingYear:      2020,
		FundingAmount:    250000000,
		FundingInvestors: []string{"VC One"},
		StockExchange:    "NASDAQ",
		StockSymbol:      "ACME",
	}
	p, ok := recordToParams(rec)
	if !ok {
		t.Fatal("ok = false, want true for a named record")
	}
	if p.Slug != "acme-corp" {
		t.Errorf("slug = %q, want normalized \"acme-corp\"", p.Slug)
	}
	if p.Name != "Acme Corp" {
		t.Errorf("name = %q", p.Name)
	}
	if !p.YearFounded.Valid || p.YearFounded.Int32 != 1999 {
		t.Errorf("year_founded = %+v", p.YearFounded)
	}
	if !p.EmployeeCount.Valid || p.EmployeeCount.Int32 != 500 {
		t.Errorf("employee_count = %+v", p.EmployeeCount)
	}
	if !p.HqCountry.Valid || p.HqCountry.String != "US" {
		t.Errorf("hq_country = %+v", p.HqCountry)
	}
	if !reflect.DeepEqual(p.Industries, []string{"Software", "Fintech"}) {
		t.Errorf("industries = %v", p.Industries)
	}

	var info map[string]any
	if err := json.Unmarshal(p.CompanyInfo, &info); err != nil {
		t.Fatalf("company_info not valid JSON: %v", err)
	}
	if info["homepage"] != "acme.com" || info["parent_company"] != "Acme Holdings" {
		t.Errorf("company_info missing homepage/parent: %v", info)
	}
	funding, _ := info["funding"].(map[string]any)
	if funding == nil || funding["type"] != "Series B" {
		t.Errorf("funding block wrong: %v", info["funding"])
	}
	stock, _ := info["stock"].(map[string]any)
	if stock == nil || stock["symbol"] != "ACME" || stock["exchange"] != "NASDAQ" {
		t.Errorf("stock block wrong: %v", info["stock"])
	}
}

func TestRecordToParams_EmptyValuesBecomeNullAndEmptyJSON(t *testing.T) {
	rec := record{Name: "Bare Co"} // everything else zero
	p, ok := recordToParams(rec)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if p.YearFounded.Valid || p.EmployeeCount.Valid {
		t.Error("zero numerics should be NULL, not set")
	}
	if p.HqCountry.Valid || p.OrganizationType.Valid || p.Tagline.Valid {
		t.Error("blank strings should be NULL, not set")
	}
	if p.Industries == nil {
		t.Error("industries must be non-nil ([]string{}) for the NOT NULL column")
	}
	if len(p.Industries) != 0 {
		t.Errorf("industries = %v, want empty", p.Industries)
	}
	if string(p.CompanyInfo) != "{}" {
		t.Errorf("company_info = %s, want {}", p.CompanyInfo)
	}
}

func TestRecordToParams_BlankNameSkipped(t *testing.T) {
	for _, name := range []string{"", "   ", "!!!"} {
		if _, ok := recordToParams(record{Name: name}); ok {
			t.Errorf("recordToParams(name=%q) ok = true, want false (empty slug)", name)
		}
	}
}
