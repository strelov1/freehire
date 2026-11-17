package skilltag

import (
	"slices"
	"testing"
)

// The procurement vocabulary, mined from live Purchasing and Procurement Specialist postings.
func TestParse_ProcurementCraft(t *testing.T) {
	cases := []struct{ text, want string }{
		{"Lead strategic sourcing for indirect categories.", "strategic-sourcing"},
		{"Experience in purchase order management is required.", "purchase-order-management"},
		{"Responsible for purchase order processing.", "purchase-order-management"},
		{"Create and release purchase orders in the ERP.", "purchase-order-management"},
		{"Own inventory management and cycle counts.", "inventory-management"},
		{"Apply inventory control best practice.", "inventory-management"},
		{"Degree in Supply Chain Management or similar.", "supply-chain-management"},
		{"Category management experience is a plus.", "category-management"},
		{"Run spend analysis to find savings.", "spend-analysis"},
		{"Run RFQ and RFP processes end to end.", "rfx"},
		{"Issue RFQs to approved suppliers.", "rfx"},
		{"Prepare the request for quotation pack.", "rfx"},
		{"Prepare a request for proposal for the new site.", "rfx"},
		{"Directed by Materials Requirements Planning (MRP) and ERP.", "mrp"},
		{"Proficient in Microsoft Excel and Word.", "excel"},
		{"Advanced Excel (VLOOKUP, PivotTables) required.", "excel"},
		{"Strong MS Excel skills.", "excel"},
		{"Proficiency in Excel is expected.", "excel"},
		{"Negotiate pricing in contract negotiations with suppliers.", "contract-negotiation"},
		{"Experience in contract negotiation.", "contract-negotiation"},
		{"Strong supplier management skills.", "vendor-management"},
		{"Own supplier relationship management for key accounts.", "vendor-management"},
		{"Vendor relationship management experience.", "vendor-management"},
	}
	for _, tc := range cases {
		if got := Parse(tc.text); !slices.Contains(got, tc.want) {
			t.Errorf("Parse(%q) = %v, want it to contain %q", tc.text, got, tc.want)
		}
	}
}

// MRP is a maximum retail price or a review panel outside operations roles, so the acronym
// resolves only inside an operations-like category. The spelled-out phrase is unambiguous.
func TestParse_MRPNeedsAnOperationsCategory(t *testing.T) {
	text := "Experience with ERP and MRP systems."
	for _, cat := range []string{"operations", "logistics", "manufacturing", "industrial_engineering"} {
		if got := Parse(text, WithAcronymCategory(cat)); !slices.Contains(got, "mrp") {
			t.Errorf("category %q: Parse = %v, want mrp", cat, got)
		}
	}
	for _, cat := range []string{"", "retail", "healthcare", "sales"} {
		var opts []Option
		if cat != "" {
			opts = append(opts, WithAcronymCategory(cat))
		}
		if got := Parse("Display the MRP on every product; Medical Review Panel (MRP) duties.", opts...); slices.Contains(got, "mrp") {
			t.Errorf("category %q: Parse = %v, must not contain mrp", cat, got)
		}
	}
	if got := Parse("Familiar with material requirements planning.", WithAcronymCategory("retail")); !slices.Contains(got, "mrp") {
		t.Errorf("the spelled-out phrase must resolve in any category, got %v", got)
	}
}

// Everything the vocabulary must NOT tag: ordinary prose, a recruiter's sourcing, a quality
// engineer's supplier work, a general operations manager.
func TestParse_ProcurementVocabularyStaysOffUnrelatedText(t *testing.T) {
	cases := []struct {
		name, text string
		banned     []string
	}{
		{"verb excel", "We want people who excel in a fast-paced environment and help customers excel.", []string{"excel"}},
		{"excellence is not excel", "Committed to excellence and Excel-lent service.", []string{"excel"}},
		{"company boilerplate", "A global supply chain and procurement leader serving customers worldwide.", []string{"supply-chain-management", "strategic-sourcing", "purchase-order-management"}},
		{"recruiter sourcing", "Recruiter: sourcing candidates on LinkedIn, boolean search, talent sourcing, building pipelines.", []string{"strategic-sourcing", "supply-chain-management", "vendor-management"}},
		{"supplier quality engineer", "Supplier quality engineer: run supplier audits, SCAR, PPAP and incoming inspection with suppliers.", []string{"vendor-management", "strategic-sourcing", "purchase-order-management", "rfx"}},
		{"general operations", "Operations manager: scheduling, KPIs, staffing, budgets and process documentation.", []string{"inventory-management", "supply-chain-management", "vendor-management", "purchase-order-management", "spend-analysis", "category-management"}},
		{"RFI is radio interference", "RF engineer: reduce RFI and EMI with shielding and filtering.", []string{"rfx"}},
		{"lowercase acronyms", "the rfp and rfq stay prose when lower case, like a typo in a chat", []string{"rfx"}},
	}
	for _, tc := range cases {
		got := Parse(tc.text)
		for _, b := range tc.banned {
			if slices.Contains(got, b) {
				t.Errorf("%s: Parse(%q) = %v, must not contain %q", tc.name, tc.text, got, b)
			}
		}
	}
}

// A procurement discipline is not a technology, so it must not vouch for the gated words
// ("ai", "crm", "react"): "inventory management" beside "AI-powered" prose is not an AI skill.
func TestParse_ProcurementDisciplinesDoNotCorroborateGatedWords(t *testing.T) {
	text := "Own inventory management, spend analysis, strategic sourcing and purchase orders. Our AI-first culture and CRM habit welcome you; we react fast."
	got := Parse(text)
	for _, banned := range []string{"ai", "crm", "react"} {
		if slices.Contains(got, banned) {
			t.Errorf("Parse = %v, must not contain gated %q", got, banned)
		}
	}
	if !slices.Contains(got, "inventory-management") {
		t.Errorf("Parse = %v, want inventory-management", got)
	}
}

// A realistic posting: the canonical set is the whole claim, so an alias that starts matching
// something else shows up here.
func TestParse_ProcurementPosting(t *testing.T) {
	text := `<p>Manage strategic sourcing and category management for direct materials. Run RFI, RFQ and RFP
	processes, spend analysis, contract negotiations and supplier relationship management. Issue purchase
	orders and keep inventory management accurate in our ERP and MRP systems; advanced Excel.</p>`
	got := Parse(text, WithAcronymCategory("operations"))
	for _, want := range []string{
		"strategic-sourcing", "category-management", "rfx", "spend-analysis", "contract-negotiation",
		"vendor-management", "purchase-order-management", "inventory-management", "mrp", "excel",
	} {
		if !slices.Contains(got, want) {
			t.Errorf("Parse = %v, missing %q", got, want)
		}
	}
}

// CVs reach the same dictionary through Canonicalize, so the new canonicals resolve there too.
func TestCanonicalize_Procurement(t *testing.T) {
	got := Canonicalize([]string{"Supplier Management", "Contract Negotiations", "Strategic Sourcing", "Microsoft Excel"})
	for _, want := range []string{"vendor-management", "contract-negotiation", "strategic-sourcing", "excel"} {
		if !slices.Contains(got, want) {
			t.Errorf("Canonicalize = %v, missing %q", got, want)
		}
	}
}
