package skilltag

// procurementPhraseAliases is the buyer's craft, mined from 50 live "Purchasing Specialist" and
// "Procurement Specialist" descriptions (2026-11-16), where 15 carried no skill at all.
//
// Every canonical here is derived into nonCorroboratingPhrases: naming a duty is evidence the posting
// IS a procurement one, never that it is technical, so none may lift the gate off a neighbouring
// ambiguous word (`ai`, `crm`, `react`). A warehouse lead lists "inventory management" and a retail
// planner "category management" in exactly the same sentence.
//
// What is deliberately absent:
//   - bare "sourcing": talent sourcing is a recruiter's job, and `talent-sourcing` already owns that
//     phrase.
//   - bare "procurement", "purchasing", "supply chain" and "excel": a company's boilerplate says
//     "global supply chain" and "excel in a fast-paced team", neither of which is a skill.
//   - the "RFI" acronym: in an RF engineer's posting it is radio-frequency interference.
//   - "supplier management" and "supplier relationship management": aliases of the existing
//     `vendor-management` (professionalPhraseAliases), not a second canonical for the same work.
var procurementPhraseAliases = []phraseAlias{
	{"strategic sourcing", "strategic-sourcing"},
	{"purchase order management", "purchase-order-management"},
	{"purchase order processing", "purchase-order-management"},
	{"purchase orders", "purchase-order-management"}, {"purchase order", "purchase-order-management"},
	{"inventory management", "inventory-management"},
	{"inventory control", "inventory-management"},
	{"supply chain management", "supply-chain-management"},
	{"category management", "category-management"},
	{"spend analysis", "spend-analysis"},
	{"request for quotation", "rfx"}, {"request for quotations", "rfx"},
	{"request for quote", "rfx"}, {"request for quotes", "rfx"},
	{"request for proposal", "rfx"}, {"request for proposals", "rfx"},
	{"request for information", "rfx"},
	{"material requirements planning", "mrp"}, {"materials requirements planning", "mrp"},
	{"material requirement planning", "mrp"},
	// Excel is written as a product ("Microsoft Excel", "advanced Excel") or as the thing a
	// candidate is proficient in; the bare verb is never an alias.
	{"microsoft excel", "excel"}, {"ms excel", "excel"}, {"advanced excel", "excel"},
	{"excel proficiency", "excel"}, {"excel skills", "excel"}, {"excel spreadsheets", "excel"},
	{"proficiency in excel", "excel"}, {"proficient in excel", "excel"},
	{"proficiency with excel", "excel"}, {"proficient with excel", "excel"},
}

// procurementCategories are the categories whose postings may say "MRP" and mean material
// requirements planning. Outside them it is a maximum retail price or a medical review panel.
var procurementCategories = map[string]bool{
	"operations":             true,
	"logistics":              true,
	"manufacturing":          true,
	"industrial_engineering": true,
}
