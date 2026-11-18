package classify

import "testing"

func TestIsNonTech(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  bool
	}{
		// Positives — confident non-tech role nouns beyond the 4 category buckets.
		{"nurse", "NICU Registered Nurse - Per Diem", true},
		{"veterinary nurse", "Veterinary Nurse", true},
		{"forklift operator", "Forklift Operator - Night Shift", true},
		{"warehouse cleaner", "Warehouse Janitorial Cleaner", true},
		{"cashier", "Cashier / Front End Associate", true},
		{"housekeeping", "Accommodation Cleaner Part Time", true},
		{"electrician", "Journeyman Electrician", true},
		{"plumber", "Commercial Plumber", true},
		{"pilates instructor", "Pilates Instructor", true},
		{"dental hygienist", "Dental Hygienist", true},
		{"line cook", "Line Cook - Banquet Setup", true},
		{"barista", "Barista (Seasonal)", true},
		{"truck driver", "Class A Truck Driver", true},
		{"security guard", "Overnight Security Guard", true},
		{"teacher", "Preschool Teacher", true},

		// Russian non-tech roles (the trudvsem/hh tail) — must now flag as non-tech
		// instead of falling to unknown.
		{"ru cashier", "Продавец-кассир", true},
		{"ru cleaner", "Уборщик производственных и служебных помещений", true},
		{"ru driver", "Водитель автомобиля", true},
		{"ru cook", "Повар", true},
		{"ru welder", "Электрогазосварщик", true},
		{"ru nurse", "Медицинская сестра", true},
		{"ru loader", "Грузчик", true},
		{"ru teacher", "Учитель математики", true},
		{"ru courier", "Курьер", true},
		// Portuguese (BR) non-tech roles (the gupy tail).
		{"br cashier", "Operador(a) de Caixa", true},
		{"br store", "Operador(a) de Loja", true},
		{"br cleaner", "Auxiliar de Limpeza", true},
		{"br driver", "Motorista Entregador", true},
		{"br cook", "Cozinheiro", true},
		// Russian/Portuguese trap negatives — ambiguous words must NOT flag.
		{"ru engineer bare", "Инженер-программист", false},
		{"ru technician bare", "Системный техник", false},
		{"ru analyst bare", "Аналитик данных", false},
		{"br analyst bare", "Analista de Sistemas", false},
		{"br technician bare", "Técnico de TI", false},

		// Trap negatives — technical titles that must NOT be flagged, including
		// shared words ("engineer") and non-tech-adjacent substrings from real data.
		{"software engineer", "Software Engineer II, AWS DynamoDB", false},
		{"hris engineer", "Senior HRIS Engineer (SAP SuccessFactors)", false},
		{"devops engineer", "DevOps Engineer", false},
		{"data scientist", "Data Scientist", false},
		{"retail application engineer", "Principal Systems Engineer - Retail Application Engineer", false},
		{"device driver engineer", "Storage Driver Engineer", false},
		{"support engineer", "Application Support Engineer", false},
		{"chef config tool", "Configuration Engineer (Chef, Puppet)", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNonTech(tt.title); got != tt.want {
				t.Errorf("IsNonTech(%q) = %v, want %v", tt.title, got, tt.want)
			}
		})
	}
}

// A title the category dictionary places in engineering_design must resolve that category
// despite the non-tech dictionary's word match. The two vocabularies overlap by design —
// the physical trades the non-tech list anchors ("hvac", "sheet metal", "machinist")
// are the same trades that post draughting work — and engineering_design is a
// deliberate placement, not the absence of a signal. Without this veto, splitting the
// category out of `design` (where TechCategories supplied the veto for free) turns an
// "HVAC Designer" away at ingest.
func TestNonTechEngineeringDesignResolvesACategory(t *testing.T) {
	vetoed := []string{
		"HVAC Design Engineer",
		"HVAC Designer",
		"Sheet Metal Design Engineer",
		"Design Engineer - Heavy Equipment",
		"CAD Designer - Manufacturing Engineer",
		"Электрик-конструктор",
	}
	for _, title := range vetoed {
		if !IsNonTech(title) {
			t.Fatalf("precondition: %q must match the non-tech dictionary for this test to mean anything", title)
		}
		if Parse(title).Category == "" {
			t.Errorf("Parse(%q) resolved no category, want one (engineering_design vetoes deletion)", title)
		}
	}

}

// The catalogue is broader than IT: a non-technical title the dictionary files is kept
// under its facet — the consumer and service categories as much as the craft ones.
func TestNonTechTitlesResolveACategory(t *testing.T) {
	for _, title := range []string{
		"Registered Nurse", "Line Cook", "Cashier", "HVAC Technician",
		"Warehouse Janitorial Cleaner", "Pharmacist", "Administrative Assistant",
		"Повар", "Продавец-кассир", "Operador(a) de Caixa",
	} {
		if !IsNonTech(title) {
			t.Fatalf("precondition: %q must match the non-tech dictionary for this test to mean anything", title)
		}
		if Parse(title).Category == "" {
			t.Errorf("Parse(%q) resolved no category, want one", title)
		}
	}

}

// The same veto, reached from the other direction and with a sharper edge. The non-tech
// term list carries "охрана труда"/"охране труда" — the Russian name of the
// occupational-safety profession — so before this category existed every Russian HSE
// title was turned away by the (since removed, #4412) ingest catalogue filter. A facet no posting can
// reach is not a facet, which is why this test guards the category rather than the
// dictionary entry.
func TestNonTechOccupationalSafetyResolvesACategory(t *testing.T) {
	for _, title := range []string{
		"Инженер по охране труда",
		"Специалист по охране труда",
	} {
		if !IsNonTech(title) {
			t.Fatalf("precondition: %q must match the non-tech dictionary for this test to mean anything", title)
		}
		if Parse(title).Category == "" {
			t.Errorf("Parse(%q) resolved no category, want one (occupational_safety vetoes deletion)", title)
		}
	}

	// English HSE titles do not match the non-tech term list at all, so they were never
	// at risk from this path. Asserted anyway: the veto must hold for the whole category,
	// not only for the spellings that happen to collide today.
	for _, title := range []string{"EHS Specialist", "HSE Manager", "Safety Coordinator"} {
		if Parse(title).Category == "" {
			t.Errorf("Parse(%q) resolved no category, want one", title)
		}
	}
}
