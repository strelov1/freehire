package classify

import (
	"testing"
)

// TestParse_SocialServicesScienceManufacturing pins the three SOC families the
// catalogue had no seat for (community and social service, science, production), and
// — the half that matters as much — the titles that must NOT move into them: every
// alias of these three is declared after every other block, so a title that resolved
// before keeps its category, and the technical scientist titles keep resolving none.
func TestParse_SocialServicesScienceManufacturing(t *testing.T) {
	cases := []struct {
		title, wantCategory, why string
	}{
		{"Social Worker", "social_services", ""},
		{"Licensed Clinical Social Worker (LCSW)", "social_services", ""},
		{"Social Worker LMSW", "social_services", ""},
		{"School Social Worker", "social_services", ""},
		{"Community Liaison", "social_services", ""},
		{"Youth Worker", "social_services", ""},
		{"Caseworker", "social_services", ""},
		{"Community Outreach Worker", "social_services", ""},
		{"Substance Abuse Counselor", "social_services", ""},
		{"Victim Advocate", "social_services", ""},
		{"Chaplain", "social_services", ""},
		{"Социальный работник", "social_services", ""},
		{"Специалист по социальной работе", "social_services", ""},
		{"Социальный педагог", "social_services", "not the bare педагог, which is education"},
		{"Production Team Member", "manufacturing", "not the bare team member, which is retail"},
		{"Travailleur social", "social_services", ""},
		{"Assistente Social", "social_services", ""},
		{"Sozialarbeiter*in (m/w/d)", "social_services", ""},
		{"Sozialpädagogin", "social_services", "the -in form is its own word"},
		{"After School Counselor", "education", "the afternoon club, not the guidance office"},

		{"Lab Technician", "science", ""},
		{"QC Lab Technician", "science", ""},
		{"Laboratory Assistant", "science", ""},
		{"Chemist", "science", ""},
		{"Microbiologist II", "science", ""},
		{"Exploration Geologist", "science", ""},
		{"Senior Environmental Scientist", "science", ""},
		{"Associate Scientist I", "science", ""},
		{"Лаборант химического анализа", "science", ""},
		{"Техник-лаборант", "science", "a hyphen is a word boundary"},
		{"Химик", "science", ""},
		{"Medical Lab Technician", "healthcare", "SOC 29-2010: the clinical lab is healthcare"},
		{"Clinical Laboratory Scientist", "healthcare", ""},
		{"Cath Lab Technician", "healthcare", "a cardiac procedure room"},
		{"Computer Lab Technician", "it_support", "the campus computer lab is IT support"},

		{"Assembler", "manufacturing", ""},
		{"Assembly Technician", "manufacturing", ""},
		{"CNC Operator", "manufacturing", ""},
		{"Packaging Operator", "manufacturing", ""},
		{"Quality Inspector", "manufacturing", ""},
		{"QC Inspector", "manufacturing", ""},
		{"Line Operator", "manufacturing", ""},
		{"Fabricator", "manufacturing", ""},
		{"Production Operator", "manufacturing", "moved from skilled_trades"},
		{"Production Associate", "manufacturing", "moved from skilled_trades"},
		{"Production Supervisor", "manufacturing", "moved from skilled_trades"},
		{"Machine Operator", "manufacturing", "moved from skilled_trades"},
		{"Сборщик мебели", "manufacturing", ""},
		{"Оператор производственной линии", "manufacturing", ""},
		{"Оператор станка с ЧПУ", "manufacturing", ""},
		{"Produktionsmitarbeiter (m/w/d)", "manufacturing", ""},
		{"Maschinenbediener (m/w/d)", "manufacturing", ""},

		// Must not move.
		{"Research Scientist", "", "as often software or ML as laboratory"},
		{"AI Research Scientist", "", "not lab science"},
		{"Applied Scientist", "", "not lab science"},
		{"Senior Scientist", "", "no bare scientist alias"},
		{"Research Associate", "", "also an equity analyst"},
		{"Data Scientist", "data_science", ""},
		{"Machine Learning Scientist", "ml_ai", ""},
		{"QA Lab Technician", "qa", "the qa alias above keeps winning"},
		{"QA Inspector", "qa", "the qa alias above keeps winning"},
		{"Nurse Case Manager", "healthcare", "the nurse names the craft; a bare case manager is not social work"},
		{"Family Support Worker", "social_services", "a care worker, not a help desk: moved from support deliberately"},
		{"Сборщик заказов", "logistics", "a warehouse picker, declared above"},
		{"Production Engineer", "industrial_engineering", ""},
		{"Software Engineer, Manufacturing Systems", "software_engineering", ""},
		{"Heavy Equipment Operator", "skilled_trades", "construction, not the plant floor"},
		{"CNC Machinist", "skilled_trades", "already resolved; keeps its category"},
		{"Welder Fabricator", "skilled_trades", "welder resolves first"},
		{"Mental Health Counselor", "healthcare", ""},
		{"Упаковщик", "logistics", ""},
	}
	for _, tc := range cases {
		if got := Parse(tc.title).Category; got != tc.wantCategory {
			t.Errorf("Parse(%q).Category = %q, want %q %s", tc.title, got, tc.wantCategory, tc.why)
		}
	}
}
