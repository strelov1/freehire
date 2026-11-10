package classify

import (
	"testing"
)

// TestParse_OccupationalSafety pins the health-safety-environment profession, which
// spells itself a dozen ways and which this catalogue could not see at all. Measured on
// prod 2026-09-24 over live non-duplicate postings, the acronyms alone matched 6,254:
// 4,515 resolved no category and were unreachable from every facet, and 1,454 resolved
// `management` off the bare `manager` fall-through, filing an HSE Manager next to a
// Sales Manager.
func TestParse_OccupationalSafety(t *testing.T) {
	cases := []struct {
		title        string
		wantCategory string
		why          string
	}{
		// The acronym spellings. Live prod titles, with their counts.
		{"EHS Specialist", "occupational_safety", "208 live postings"},
		{"EHS Coordinator", "occupational_safety", "95"},
		{"HSE Officer", "occupational_safety", "87"},
		{"HSE Advisor", "occupational_safety", "66"},
		{"HSE Engineer", "occupational_safety", "54"},
		{"HSEQ Advisor", "occupational_safety", "14"},
		{"QHSE Coordinator", "occupational_safety", "12"},
		{"HSSE Manager", "occupational_safety", "13"},
		{"SHEQ Coordinator", "occupational_safety", ""},
		{"SHES Advisor", "occupational_safety", ""},
		{"Senior EHS Specialist", "occupational_safety", "24; a grade word does not change the craft"},

		// The spelled-out forms, and the Russian one the non-tech list already names.
		{"Environmental Health and Safety Specialist", "occupational_safety", "30"},
		{"Environmental Health & Safety Specialist", "occupational_safety", "40"},
		{"Health & Safety Advisor", "occupational_safety", "30"},
		{"Health and Safety Coordinator", "occupational_safety", "20"},
		{"Occupational Health and Safety Manager", "occupational_safety", ""},
		{"Инженер по охране труда", "occupational_safety", ""},

		// The titles SOC lists as reported for 19-5011.
		{"Safety Coordinator", "occupational_safety", "283"},
		{"Safety Specialist", "occupational_safety", "278"},
		{"Safety Officer", "occupational_safety", "130"},
		{"Safety Technician", "occupational_safety", "67"},
		{"Safety Supervisor", "occupational_safety", "55"},
		{"Safety Advisor", "occupational_safety", "42"},
		{"Safety Director", "occupational_safety", "34"},
		{"Industrial Hygienist", "occupational_safety", "named in SOC's own reported-title list"},
		{"Risk Control Consultant", "occupational_safety", "named in SOC's own reported-title list"},

		// Moved off industrial_engineering, where it was this dictionary's only
		// relevant line. The plant category is for making and running the thing; this
		// is the compliance function that audits it.
		{"Safety Engineer", "occupational_safety", "46 postings change hands"},

		// The block sits ABOVE the bare `manager` fall-through, which is what claimed
		// 1,454 of these. SOC codes EHS Managers separately from 19-5011; this
		// catalogue deliberately does not, because `seniority` is its own facet and
		// splitting one profession across two category values would make a subscriber
		// tick two boxes to see one job market.
		{"EHS Manager", "occupational_safety", "255, the single most common spelling"},
		{"HSE Manager", "occupational_safety", "158"},
		{"Regional EHS Manager", "occupational_safety", "18"},
		{"Senior EHS Manager", "occupational_safety", "15"},
		{"Project HSE Manager", "occupational_safety", "12"},

		// …and a manager title naming no recognised function still falls through.
		{"Business Manager", "", "no function named, and no management category to fall through to"},

		// The words this block must NOT claim. Each was measured before exclusion.
		{"Software Engineer (she/her)", "software_engineering", "SHE is a pronoun in live titles; no bare alias"},
		{"Risk Analyst", "", "`risk` matched 41,468 live postings, mostly finance"},
		{"Credit Risk Manager", "finance", "finance risk is a different profession sharing a word"},
		{"Patient Safety Attendant", "healthcare", "23; bare `safety` names other crafts"},
		{"Public Safety Officer", "public_safety", "113; protective services, not HSE"},
		{"Campus Safety Officer", "public_safety", "38; protective services, not HSE"},
		{"Food Safety Manager", "industrial_engineering", "616 live; plant quality, where `quality engineer` already sits"},
		{"Product Safety Engineer", "industrial_engineering", "163; regulatory product compliance, and the category it already had"},
		{"Fire Safety Manager", "occupational_safety", "164; fire IS part of the HSE remit on an industrial site"},

		// The qualifier families a review found the first draft swallowing. Each is a
		// profession of its own that happens to carry the word, and each is routed to the
		// category that is true rather than blanked — blanking is what makes a facet
		// unreachable, which is the harm this change exists to undo.
		{"Trust and Safety Specialist", "", "14 live; platform integrity, the likeliest collision on an IT board"},
		{"Trust & Safety Manager", "", "it resolved management until that category was retired; no category is true for it"},
		{"Trust & Safety Director", "", "role_type reaches it"},
		{"Trust & Safety Lead", "", ""},
		{"Trust and Safety Engineer", "", ""},

		// …and the Trust & Safety titles a blanket sentinel BLANKED, which is what pairing
		// it with the role noun exists to prevent. Neither ever collided with the HSE
		// block: the matcher takes contiguous phrases, and "safety product manager" does
		// not contain "safety manager".
		{"Trust & Safety Product Manager", "product", "7 live"},
		{"Software Engineer, Trust & Safety", "software_engineering", "4 live"},
		{"Functional Safety Engineer", "industrial_engineering", "ISO 26262, automotive and semiconductor"},
		{"Drug Safety Specialist", "healthcare", "pharmacovigilance"},
		{"AI Safety Engineer", "ml_ai", "alignment research, not a safety department"},
		{"Life Safety Technician", "skilled_trades", "fire alarm and sprinkler trades"},
		{"School Safety Officer", "public_safety", "protective services"},
		{"Pool Safety Officer", "public_safety", ""},
		// The same professions in the role nouns a first pass missed — 27 live postings
		// that were filing under HSE because only the `officer` spelling was excluded.
		{"Public Safety Manager", "public_safety", "5 live"},
		{"Public Safety Specialist", "public_safety", "4"},
		{"Public Safety Director", "public_safety", "3"},
		{"Campus Safety Specialist", "public_safety", "5"},
		{"Campus Safety Coordinator", "public_safety", "1"},

		// …and the titles the exclusions must NOT blank. A blanket qualifier entry took
		// each of these away from a category it already resolved.
		{"Public Safety Dispatcher", "public_safety", "the emergency dispatcher, claimed ahead of the bare logistics dispatcher"},
		{"Patient Safety Registered Nurse", "healthcare", ""},

		// The qualified SHE spellings DO resolve — the exclusion is of the bare word,
		// not of the acronym.
		{"SHE Manager", "occupational_safety", ""},
		{"SHE Advisor", "occupational_safety", ""},
	}

	for _, tc := range cases {
		if got := Parse(tc.title).Category; got != tc.wantCategory {
			t.Errorf("Parse(%q).Category = %q, want %q %s", tc.title, got, tc.wantCategory, tc.why)
		}
	}
}

// TestParse_ConsumerIndustries pins the four consumer categories: the residue left
// after the IT and industrial waves, 225 000 open postings that were filterable by
// nothing at all.
func TestParse_ConsumerIndustries(t *testing.T) {
	cases := []struct {
		title        string
		wantCategory string
		why          string
	}{
		// Healthcare.
		{"Registered Nurse", "healthcare", ""},
		{"RN", "healthcare", ""},
		{"Nurse Practitioner", "healthcare", ""},
		{"Caregiver", "healthcare", ""},
		{"Home Health Aide", "healthcare", ""},
		{"Medical Assistant", "healthcare", ""},
		{"Dental Hygienist", "healthcare", ""},
		{"Pharmacy Technician", "healthcare", ""},
		{"Medication Technician", "healthcare", "declared above the technician family"},
		{"Patient Coordinator", "healthcare", ""},
		{"Phlebotomist", "healthcare", ""},
		// "Mobile" in a healthcare title means the clinician travels to the patient,
		// not that the work is a phone app. The imaging room was already named in
		// nontech.go's craft list, so the catalogue knew these were not technical and
		// simply had nowhere to file them.
		{"Mobile Phlebotomist", "healthcare", "the clinician travels, the work is not an app"},
		{"Mobile X-Ray Technologist", "healthcare", "the clinician travels, the work is not an app"},
		{"Radiology Technologist (Mobile Imaging)", "healthcare", ""},
		{"MRI Technologist", "healthcare", ""},
		{"Sonographer", "healthcare", ""},
		{"Physical Therapist", "healthcare", ""},
		{"Veterinarian", "healthcare", ""},
		// The Russian medical family — bare token, since the qualified forms hyphenate
		// or postfix a phrase.
		{"Врач", "healthcare", ""},
		{"Врач-терапевт", "healthcare", ""},
		{"Врач-акушер-гинеколог", "healthcare", ""},
		{"Врач ультразвуковой диагностики", "healthcare", ""},
		{"Ветеринарный врач", "healthcare", ""},
		{"Медсестра", "healthcare", ""},
		{"Фельдшер", "healthcare", ""},

		// Skilled trades.
		{"Service Technician", "skilled_trades", ""},
		{"Field Service Technician", "skilled_trades", ""},
		{"Installation Technician", "skilled_trades", ""},
		{"Diesel Technician", "skilled_trades", ""},
		{"Automotive Technician", "skilled_trades", ""},
		{"Automotive Mechanic", "skilled_trades", ""},
		{"Electrician", "skilled_trades", ""},
		{"Plumber", "skilled_trades", ""},
		{"Welder", "skilled_trades", ""},
		{"HVAC Technician", "skilled_trades", ""},
		{"Machinist", "skilled_trades", ""},
		{"Carpenter", "skilled_trades", ""},
		{"Электрик", "skilled_trades", ""},
		{"Электросварщик ручной сварки", "skilled_trades", ""},
		{"Слесарь", "skilled_trades", ""},
		{"Плотник", "skilled_trades", ""},
		{"Электромеханик", "skilled_trades", ""},

		// Retail, including the grocery clerk family that is shop floor, not office.
		{"Team Member", "retail", ""},
		// Stays `sales`, which resolves far above. A "Sales Associate" at a software
		// company IS a sales role, and taking the row would be exactly the theft this
		// whole line of work keeps guarding against.
		{"Sales Associate", "sales", ""},
		{"Cashier", "retail", ""},
		{"Retail Service Specialist", "retail", ""},
		{"Merchandising Specialist", "retail", ""},
		{"Brand Ambassador", "retail", ""},
		{"Product Demonstrator", "retail", ""},
		{"Deli Clerk", "retail", ""},
		{"Grocery Clerk", "retail", ""},
		{"Produce Clerk", "retail", ""},
		{"Store Driver", "retail", "a shop's own driver, not a haulier"},

		// Hospitality.
		{"Server", "hospitality", ""},
		{"Host/Hostess", "hospitality", ""},
		{"Chef", "hospitality", ""},
		{"Line Cook", "hospitality", ""},
		{"Prep Cook", "hospitality", ""},
		{"Barista", "hospitality", ""},
		{"Bartender", "hospitality", ""},
		{"Dishwasher", "hospitality", ""},
		{"Kitchen Assistant", "hospitality", ""},
		{"Banquet Server", "hospitality", ""},

		// The compound hazard: a short alias hiding INSIDE a longer word. Whole-word
		// matching prevents it, but nothing in the alias list shows the hazard — the
		// mining script, which matched on raw substrings, filed this one under
		// logistics because it ends in "водитель".
		// Resolved to nothing when this case was written; the service wave gave it a
		// category. What it must NEVER be is `logistics` — see TestParse_ServiceSectors.
		{"Делопроизводитель", "administration", ""},

		// Everything the earlier waves settled must be untouched.
		{"Software Engineer", "software_engineering", ""},
		{"Project Engineer", "industrial_engineering", ""},
		{"Field Service Engineer", "industrial_engineering", "the engineer, not the technician"},
		{"Systems Engineer", "software_engineering", ""},
		{"Sales Engineer", "solutions_engineering", ""},
		{"Support Engineer", "support", ""},
		{"Программист", "software_engineering", ""},
		{"Системный администратор", "devops", ""},
		{"Video Editor", "creative", ""},
		{"Sound Designer", "creative", ""},
	}

	for _, tc := range cases {
		if got := Parse(tc.title).Category; got != tc.wantCategory {
			t.Errorf("Parse(%q).Category = %q, want %q %s", tc.title, got, tc.wantCategory, tc.why)
		}
	}
}
