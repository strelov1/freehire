package classify

import (
	"testing"
)

// TestParse_ITSupportSplit pins the help desk's split out of `support`: the IT side
// resolves the technical `it_support`, customer service stays `support`, and the bare
// "support" — declared last — no longer outranks the profession a title names.
func TestParse_ITSupportSplit(t *testing.T) {
	cases := []struct{ title, wantCategory, why string }{
		{"IT Help Desk Technician", "it_support", ""},
		{"Service Desk Analyst L2", "it_support", ""},
		{"Desktop Support Technician", "it_support", ""},
		{"IT Support Engineer", "it_support", ""},
		{"Technical Support Engineer", "it_support", "the analyst/engineer/specialist forms only"},
		{"Technical Support Specialist", "it_support", ""},
		{"1st Level IT Supporter", "it_support", "the trailing \"er\" needs its own alias"},
		{"IT-Supporter (m/w/d)", "it_support", ""},
		{"1st Level Support (m/w/d)", "it_support", ""},
		{"2nd Line Support Engineer", "it_support", ""},
		{"L3 Support Engineer (Azure, M365, AVD & Automation)", "it_support", ""},
		{"Application Support Analyst", "it_support", ""},
		{"NOC Technician", "it_support", "resolved nothing before"},
		{"Desktop Engineer L2", "it_support", "resolved nothing before"},
		{"Technicien support informatique H/F", "it_support", ""},
		{"Специалист техподдержки", "it_support", ""},
		{"Инженер технической поддержки L2", "it_support", ""},
		{"ITサポートエンジニア", "it_support", ""},
		{"Servicedeskmedewerker", "it_support", "the Dutch compound head"},

		{"Customer Service Representative", "support", ""},
		{"Call Center Agent", "support", ""},
		{"Customer Support Engineer", "support", "names the customer side outright"},
		{"Client Support & Benefits Representative", "support", "not sales"},
		{"Customer Service Desk Associate", "support", "the customer desk, pinned ahead of \"service desk\""},
		{"Support Engineer", "support", "bare: also field-equipment and robotics support"},
		{"Technical Support", "support", "bare: also automotive and AGV support"},
		{"Support Specialist", "support", "nothing else in the title speaks"},
		{"サービスエンジニア・サポートエンジニア（工作機械・ロボット・機械系）", "support", "a machine-tool field engineer, not IT"},
		{"HR Help Desk Executive (Mandarin Speaker)", "hr", ""},

		{"Associate Dentist – Full Support, NO Weekends!", "healthcare", "the bare support names a benefit here"},
		{"Mortgage Loan Officer - Purchase-Focused, Full Support", "finance", ""},
		{"Child Autism Support Professional (Entry Level)", "healthcare", "ABA line staff, the RBT"},
		{"Administrative Support Specialist", "administration", ""},
	}
	for _, tc := range cases {
		if got := Parse(tc.title).Category; got != tc.wantCategory {
			t.Errorf("Parse(%q).Category = %q, want %q %s", tc.title, got, tc.wantCategory, tc.why)
		}
	}
}
