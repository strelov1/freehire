package classify

import (
	"slices"
	"testing"
)

// TestParse_ServiceSectors pins the last clusters that have a shape: logistics,
// education, personal services and office administration — plus the plural gap the
// consumer wave left behind.
// TestParse_MobileIsAPlatformNotAnErrand pins both halves of removing the bare "mobile"
// alias (and its Russian counterparts), the same treatment the bare "analyst" and
// "security" fall-throughs got before it.
//
// Outside software, "mobile" is the word for work that TRAVELS to the customer, and on
// the live catalogue that was much the commoner reading: of 28 650 open postings filed
// `mobile` on 2026-09-18, 18 483 named no mobile platform at all — a phone carrier's shop
// floor, a bank's travelling representative, field mechanics, and clinicians who drive to
// the patient. Because `mobile` is a technical category, `is_tech` followed it, so 1 538
// retail-sales postings were being enqueued for LLM enrichment as engineering.
//
// The negative half matters more than the positive one: a wrong category is not merely a
// wrong facet, it is the thing that puts a posting in the index at all
// (search.CategoryUnresolved returns false the moment a category exists).
func TestParse_MobileIsAPlatformNotAnErrand(t *testing.T) {
	// Software. These must keep the category — they are what the alias is FOR.
	for _, title := range []string{
		"Mobile Developer",
		"Senior Mobile Developer",
		"Mobile Engineer",
		"Staff Mobile Engineer",
		"Mobile Application Developer",
		"Mobile App Developer",
		"Mobile Apps Developer",
		"Mobile Software Engineer",
		"Mobile Architect",
		"Mobile QA Engineer",
		"Mobile Automation Engineer",
		"Principal Software Engineer - Mobile Platform",
		"Mobile/Xamarin Developer",
		"Android Developer",
		"iOS Engineer",
		"React Native Developer",
		"Мобильный разработчик",
		"Разработчик мобильных приложений",
	} {
		if got := Parse(title).Category; got != "mobile" {
			t.Errorf("Parse(%q).Category = %q, want %q", title, got, "mobile")
		}
	}

	// Errands. None of these is software, so none may claim the mobile category —
	// whatever else it resolves to, including nothing.
	for _, title := range []string{
		"Mobile Associate - Retail Sales",
		"Mobile Associate, Store-in-Store, Retail Sales",
		"Wireless Retail Sales Representative | Alliance Mobile - AT&T Authorized Retailer",
		"Sr Mobile Expert",
		"Mobile Service Technician",
		"Mobile Diesel Technician",
		"Mobile Building Engineer",
		"Mobile Maintenance Engineer, Facilities Maintenance",
		"Mobile Phlebotomist",
		"Mobile X-Ray Technologist",
		"Mobile Veterinarian",
		"Heavy Mobile Equipment Repairer",
		"Mobile Crane Operator",
		"Mobile Hair Stylist",
		"Mobile Commis Chef",
		"Мобильный банкир",
		"Мобильный банкир (Юридические лица)",
		"Мобильный клиентский менеджер",
	} {
		if got := Parse(title).Category; got == "mobile" {
			t.Errorf("Parse(%q).Category = %q — this is travelling work, not a phone app", title, got)
		}
	}
}

// TestCategories_ServiceOverlaps guards the multi-category CV path, which `Parse`
// cannot speak for: `Categories` returns EVERY matching alias rather than the
// strongest, so declaration order does nothing for it. An alias that looks harmless
// under `Parse` can still tag a title with a second, wrong category here — which is
// exactly what a "security officer" entry did to a CISO.
//
// What this protects is the CISO, and it still does, now that a plain "Security
// Officer" — 1 102 open postings, and a mall guard in nearly all of them — resolves the
// guard category. The infosec officer spellings are declared as whole phrases ahead of
// it, and an occurrence lying inside a span an earlier alias already matched is not
// counted, so "Chief Information Security Officer" does not come back tagged as a guard.
func TestCategories_ServiceOverlaps(t *testing.T) {
	for _, tc := range []struct {
		title    string
		mustNot  string
		mustHave string
	}{
		{"Chief Information Security Officer", "public_safety", "security"},
		{"Information Security Officer", "public_safety", "security"},
		{"IT Security Officer", "public_safety", "security"},
		{"Information Systems Security Officer", "public_safety", "security"},
		// The guard, and nothing technical.
		{"Security Officer", "security", "public_safety"},
	} {
		got := Categories(tc.title)
		if slices.Contains(got, tc.mustNot) {
			t.Errorf("Categories(%q) = %v, must NOT contain %q", tc.title, got, tc.mustNot)
		}
		if tc.mustHave != "" && !slices.Contains(got, tc.mustHave) {
			t.Errorf("Categories(%q) = %v, must contain %q", tc.title, got, tc.mustHave)
		}
	}
}

func TestParse_ServiceSectors(t *testing.T) {
	cases := []struct {
		title        string
		wantCategory string
		why          string
	}{
		// Logistics.
		{"Delivery Specialist", "logistics", ""},
		{"Commercial Driver", "logistics", ""},
		{"Driver", "logistics", ""},
		{"CDL A Driver", "logistics", ""},
		{"Courier", "logistics", ""},
		{"Warehouse Associate", "logistics", ""},
		{"Warehouse Supervisor", "logistics", ""},
		{"Forklift Operator", "logistics", ""},
		{"Truck Unloader", "logistics", ""},
		{"Fulfillment Associate", "logistics", ""},
		{"Dispatcher", "logistics", ""},
		{"Водитель", "logistics", ""},
		{"Курьер", "logistics", ""},
		{"Кладовщик", "logistics", ""},
		{"Сборщик заказов", "logistics", ""},
		{"Грузчик", "logistics", ""},

		// THE hazard of this wave: an office clerk that ENDS in "водитель". Asserting
		// a specific category, not merely "resolves to something" — a not-empty check
		// would pass while the row was wrong.
		{"Делопроизводитель", "administration", "ends in водитель; must not be logistics"},

		// Education.
		{"Swim Instructor", "education", ""},
		{"Private Swim Instructor", "education", ""},
		{"Chess Instructor", "education", ""},
		{"Teacher", "education", ""},
		{"Preschool Teacher", "education", ""},
		{"Tutor", "education", ""},
		{"Lecturer", "education", ""},
		{"Педагог-психолог", "education", ""},
		{"Помощник воспитателя", "education", ""},
		{"Преподаватель", "education", ""},
		{"Методист", "education", ""},

		// Administration.
		{"Receptionist", "administration", ""},
		{"Secretary", "administration", ""},
		{"Legal Secretary", "administration", ""},
		// Office Manager stays `operations`: an IT company's office manager IS its
		// operations. The assistants moved here from `operations`, because the
		// virtual, executive and administrative assistant are one job and splitting
		// them across two facets hid half of it from whoever filtered on either.
		{"Office Manager", "operations", ""},
		{"Administrative Assistant", "administration", ""},
		{"Data Entry Specialist", "administration", ""},
		{"Администратор", "administration", ""},
		{"Секретарь руководителя", "administration", ""},

		// Personal services.
		{"Stylist", "personal_services", ""},
		{"Master Stylist", "personal_services", ""},
		{"Barber", "personal_services", ""},
		{"Esthetician", "personal_services", ""},
		{"Lifeguard", "public_safety", "SOC 33-9092, a protective service"},
		{"Security Guard", "public_safety", "was resolving to the infosec facet"},
		{"Security Officer", "public_safety", "an unqualified one is a guard far more often than a CISO — 1 102 live postings say so; the qualified forms carry the discipline"},
		{"Security Specialist", "", "ambiguous the same way, and 602 live postings of it"},
		{"Chief Information Security Officer", "security", ""},
		{"Janitor", "personal_services", ""},
		{"Housekeeper", "personal_services", ""},
		{"Парикмахер", "personal_services", ""},
		{"Охранник", "public_safety", ""},
		{"Уборщик", "personal_services", ""},

		// The plural gap. wordmatch has no morphology, so a singular alias cannot
		// reach a plural title — invisible from the alias list, and it left the three
		// largest automotive spellings in the catalogue resolving to nothing.
		{"Automotive Mechanic", "skilled_trades", ""},
		{"Automotive Mechanics", "skilled_trades", "the plural the shipped alias could not reach"},
		{"Automotive Tire Technicians", "skilled_trades", ""},
		{"Automotive Alignment Technicians", "skilled_trades", ""},

		// Missed outright by the consumer wave.
		{"Optometrist", "healthcare", ""},
		{"Host", "hospitality", "3 446 open postings spell it exactly"},

		// Russian building maintenance joins the trades already there.
		{"Рабочий по комплексному обслуживанию и ремонту зданий", "skilled_trades", ""},
		{"Рабочий по благоустройству населенных пунктов", "skilled_trades", ""},

		// Everything the earlier waves settled must be untouched.
		{"Store Driver", "retail", "a shop's own driver stays with the shop"},
		{"Software Engineer", "software_engineering", ""},
		{"Project Engineer", "industrial_engineering", ""},
		{"Registered Nurse", "healthcare", ""},
		{"Server", "hospitality", ""},
		{"Team Member", "retail", ""},
		{"Agile Coach", "project_management", "the coach that is not a teacher"},

		// Field-facing delivery work.
		{"Professional Services Engineer", "solutions_engineering", ""},
		{"Professional Services Consultant", "solutions_engineering", ""},
		{"Partner Engineer", "solutions_engineering", ""},
		{"Deployment Strategist", "solutions_engineering", ""},
		{"Presales Consultant", "solutions_engineering", ""},
		{"Technical Consultant", "solutions_engineering", ""},
		{"ServiceNow Technical Consultant", "solutions_engineering", ""},
		{"Delivery Consultant", "solutions_engineering", ""},
		{"Integration Consultant", "solutions_engineering", ""},
		// A correctness fix, not an addition: roletag already declared the hyphenated
		// spelling, so the ROLE fired while the category stayed empty. Both spellings
		// must agree.
		{"Forward Deployed Engineer", "solutions_engineering", ""},
		{"Forward-Deployed Engineer", "solutions_engineering", "the role fired; the category did not"},
		// Unchanged neighbours.
		{"Delivery Manager", "project_management", ""},
		{"Engagement Manager", "", "no function named; there is no management category"},
		{"Implementation Consultant", "customer_success", ""},
		{"Solutions Architect", "architecture", ""},

		// The ERP and CRM functional consultants. Deferred by the IT wave "to the
		// industrial wave" for want of a home, and then not delivered by it — the
		// industrial category was for plant engineering, and these are platform
		// implementation work. They belong beside the platform consultants the service
		// wave already resolved: "Oracle Fusion Technical Consultant" and "Workday
		// Integration Consultant" have been `solutions_engineering` since then.
		{"SAP Consultant", "solutions_engineering", ""},
		{"SAP FICO Consultant", "solutions_engineering", ""},
		{"SAP SD Consultant", "solutions_engineering", ""},
		{"SAP MM Consultant", "solutions_engineering", ""},
		{"SAP Basis Administrator", "devops", "Basis is the platform's infrastructure, not its business logic"},
		{"Dynamics 365 CE Consultant", "solutions_engineering", ""},
		{"Dynamics 365 F&O Consultant", "solutions_engineering", ""},
		{"Dynamics 365 Developer", "software_engineering", "a developer, like the Salesforce and ServiceNow ones"},
		{"Microsoft Dynamics CRM", "solutions_engineering", ""},
		{"HubSpot CRM Administrator", "solutions_engineering", ""},
		// NOT "CRM Specialist": an unqualified one is a marketing-operations seat at
		// least as often as a platform one, and the IT wave already declined it for
		// that reason. The qualified spellings above are what carry the volume.
		{"CRM Specialist", "", ""},
		// Unchanged: these already resolved through the service wave's aliases.
		{"Oracle Fusion Technical Consultant", "solutions_engineering", ""},
		{"Workday Integration Consultant", "solutions_engineering", ""},
		{"SAP Developer", "software_engineering", ""},
		{"SAP ABAP Developer", "software_engineering", ""},
	}

	for _, tc := range cases {
		if got := Parse(tc.title).Category; got != tc.wantCategory {
			t.Errorf("Parse(%q).Category = %q, want %q %s", tc.title, got, tc.wantCategory, tc.why)
		}
	}
}
