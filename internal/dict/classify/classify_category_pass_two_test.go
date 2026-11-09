package classify

import (
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/dict/vocab"
)

// The second pass over the uncategorised tail and the retirement of `management`. Each
// group is a decision; the negative half of each is what the decision must not take.
func TestParse_CategoryPassTwo(t *testing.T) {
	cases := []struct {
		title, want, why string
	}{
		// German retail. An apprenticeship resolves through the role it names.
		{"Verkäufer (m/w/d)", "retail", ""},
		{"Verkäuferin Backwaren", "retail", ""},
		{"Verkäufer/in Obst & Gemüse", "retail", "the slash is a word boundary"},
		{"Fachverkäufer Bäckerei (m/w/d)", "retail", ""},
		{"Kaufmann im Einzelhandel (m/w/d)", "retail", ""},
		{"Ausbildung Kaufmann im Einzelhandel 2027 (m/w/d)", "retail", "classified by the role it trains for"},
		{"Ausbildung zum Verkäufer, Fachrichtung Lebensmittel", "retail", ""},
		{"Kassierer (m/w/d)", "retail", ""},
		{"Filialleiter (m/w/d)", "retail", "the store of a chain"},
		{"Abiturientenprogramm Handelsfachwirt 2027 (m/w/d)", "retail", ""},
		{"Mitarbeiter Verkauf (m/w/d)", "retail", ""},
		{"Ausbildung (m/w/d)", "", "a placeholder, never an alias"},
		{"Initiativbewerbung", "", "names no role"},
		// The Russian trudvsem tail.
		{"Экономист", "finance", ""},
		{"Ведущий экономист", "finance", ""},
		{"Швея", "manufacturing", ""},
		{"Швея-мотористка", "manufacturing", "the hyphen is a word boundary"},
		{"Горничная", "hospitality", ""},
		{"Мойщик посуды", "hospitality", ""},
		{"Буфетчик", "hospitality", ""},
		{"Кастелянша", "hospitality", ""},
		{"Гардеробщик", "hospitality", ""},
		{"Диспетчер", "logistics", "as the English dispatcher"},
		{"Диспетчер автомобильного транспорта", "logistics", ""},
		{"Старший диспетчер ЕДДС", "public_safety", "the emergency dispatcher"},
		{"Диспетчер пожарной связи", "public_safety", ""},
		{"Диспетчер факультета", "administration", "a university's scheduling clerk"},
		{"Секретарь учебной части (диспетчер)", "administration", "the secretary keeps its precedence"},
		{"Инженер оперативно-диспетчерской службы", "industrial_engineering", "the adjective is not the noun"},
		{"Оператор котельной", "skilled_trades", ""},
		{"Оператор газовой котельной", "skilled_trades", ""},
		{"Монтер пути", "skilled_trades", ""},
		{"Монтёр пути", "skilled_trades", ""},
		{"Дорожный рабочий", "skilled_trades", ""},
		{"Рабочий зеленого хозяйства", "personal_services", "filed with the groundskeeper"},
		{"Фармацевт", "healthcare", ""},
		{"Рентгенолаборант", "healthcare", "not the science лаборант"},
		{"Лаборант", "science", "the laboratory assistant stays science"},
		{"Инструктор по лечебной физкультуре", "healthcare", ""},
		{"Медицинский статистик", "healthcare", ""},
		{"Тьютор", "education", ""},
		{"Музыкальный руководитель", "education", ""},
		{"Мастер производственного обучения", "education", ""},
		{"Библиотекарь", "education", "SOC 25-4022"},
		{"Librarian I/II", "education", ""},
		{"Агроном", "science", "SOC 19-1013, as the English agronomist"},
		{"Зоотехник", "science", ""},
		{"Животновод", "", "a farmworker: SOC 45 has no category here"},
		{"Специалист по кадрам", "hr", ""},
		{"Психолог", "social_services", "ISCO-08 2634, beside social work"},
		{"Психолог в социальной сфере", "social_services", ""},
		{"Медицинский психолог", "healthcare", "the clinical psychologist"},
		{"Педагог-психолог", "education", "the school psychologist keeps education"},
		{"Заведующий хозяйством", "administration", ""},
		{"Культорганизатор", "personal_services", "SOC 39-9032 recreation worker"},
		// English.
		{"Buyer II", "operations", "procurement"},
		{"Media Buyer", "marketing", "declared far above the bare buyer"},
		{"Controller", "finance", ""},
		{"Assistant Controller", "finance", ""},
		{"Document Controller", "administration", ""},
		{"Air Traffic Controller", "logistics", ""},
		{"Stock Controller", "logistics", ""},
		{"Quality Controller", "manufacturing", ""},
		{"PLC Controller", "industrial_engineering", ""},
		{"Flight Controller", "", "aerospace operations or drone firmware: neither is finance"},
		{"Financial Controller", "finance", "unchanged"},
		{"Psychiatrist", "healthcare", ""},
		{"Laundry Attendant", "hospitality", ""},
		{"Office Coordinator", "administration", ""},
		{"Porter", "hospitality", ""},
		{"Restaurant Porter - Unit 1654", "hospitality", ""},
		{"Day Porter", "personal_services", "building cleaning"},
		{"Service Porter", "logistics", "the dealership porter moves cars"},
		{"Estimator", "industrial_engineering", "the quantity surveyor's US twin"},
		{"Electrical Estimator", "industrial_engineering", ""},
		{"Collision Estimator", "skilled_trades", "prices a car repair"},
		{"Superintendent", "skilled_trades", "construction"},
		{"Construction Superintendent", "skilled_trades", ""},
		{"Superintendent of Schools", "education", ""},
		{"Assistant Superintendent of Curriculum and Instruction", "education", "the school district's"},
		{"Marine Superintendent - Tanker", "logistics", ""},
		// Disability and residential care is social service, not a help desk.
		{"Direct Support Professional", "social_services", "was support"},
		{"Direct Support Professional (DSP)", "social_services", ""},
		{"Support Worker - Nights", "social_services", ""},
		{"Personal Support Worker", "social_services", ""},
		{"IT Support Worker", "it_support", "the desk keeps it"},
		{"Customer Support Worker", "support", ""},
		{"Administrative Support Worker", "administration", ""},
		{"Customer Support Specialist", "support", "unchanged"},
		{"Amazon DSP Delivery Driver", "logistics", "DSP is never an alias"},
		{"DSP Engineer", "", "digital signal processing, not disability care"},
		// Public safety (SOC 33).
		{"Police Officer", "public_safety", ""},
		{"Deputy Sheriff", "public_safety", ""},
		{"Correctional Officer", "public_safety", ""},
		{"Corrections Officer I", "public_safety", ""},
		{"Detention Officer", "public_safety", ""},
		{"Firefighter", "public_safety", ""},
		{"Firefighter/Paramedic", "public_safety", "a firefighter who also holds the paramedic licence"},
		{"Paramedic", "healthcare", "EMS stays healthcare"},
		{"EMT", "", "no alias: EMT stays where it was"},
		{"Lifeguard", "public_safety", ""},
		{"School Crossing Guard", "public_safety", ""},
		{"Loss Prevention Agent", "public_safety", ""},
		{"Asset Protection Detective", "public_safety", ""},
		{"Data Loss Prevention Analyst", "security", "information security, pinned first"},
		{"Emergency Dispatcher", "public_safety", ""},
		{"Truck Dispatcher", "logistics", "every other dispatcher keeps its field"},
		{"Food Service Worker - Sheriff's Office", "hospitality", "the sheriff is the employer"},
		{"Sheriff's Office Nurse", "healthcare", "the sheriff is the employer"},
		{"Police Software Engineer", "software_engineering", ""},
		{"Army National Guard Soldier", "", "the military: no bare guard alias"},
		{"Охранник", "public_safety", ""},
		{"Сторож", "public_safety", "the watchman is a guard, SOC 33-9032"},
		{"Вахтер", "personal_services", "the doorkeeper stays"},
		{"Пожарный", "public_safety", ""},
		{"Начальник пожарной части", "public_safety", ""},
		{"Полицейский", "public_safety", ""},
		{"Участковый уполномоченный полиции", "public_safety", ""},
		{"Инспектор ДПС", "public_safety", ""},
		{"Следователь", "public_safety", ""},
		{"Стажер-исследователь", "", "contains no whole-word следователь"},
		{"Стрелок ВОХР", "public_safety", ""},
		{"Военнослужащий по контракту — пулемётчик, стрелок", "", "military contract service"},
		{"Военнослужащий по контракту — повар", "hospitality", "the pin does not blank a craft named earlier"},
		{"Sicherheitsmitarbeiter (m/w/d) im Objektschutz", "public_safety", ""},
		{"Fachkraft für Schutz und Sicherheit", "public_safety", ""},
		{"Polizeivollzugsbeamter", "public_safety", ""},
		{"Feuerwehrmann", "public_safety", ""},
		{"Vigilante", "public_safety", "Brazilian Portuguese security guard"},
		{"Porteiro", "personal_services", "the doorman stays"},
		// The retired management category: a manager title resolves its craft or nothing.
		{"Restaurant Manager", "hospitality", ""},
		{"Assistant Restaurant Manager", "hospitality", ""},
		{"Kitchen Manager", "hospitality", ""},
		{"Food and Beverage Manager", "hospitality", ""},
		{"Store Manager", "retail", ""},
		{"Assistant Store Manager", "retail", ""},
		{"Retail Assistant Manager", "retail", ""},
		{"Plant Manager", "manufacturing", ""},
		{"Production Manager", "manufacturing", ""},
		{"Senior Event Production Manager", "marketing", "a show, not a line"},
		{"Quality Manager", "industrial_engineering", ""},
		{"Construction Manager", "skilled_trades", ""},
		{"Site Manager", "skilled_trades", ""},
		{"Maintenance Manager", "skilled_trades", ""},
		{"Supply Chain Manager", "logistics", ""},
		{"Fleet Manager", "logistics", ""},
		{"Warehouse Manager", "logistics", ""},
		{"Tax Manager", "finance", ""},
		{"Relationship Manager", "finance", "the bank's"},
		{"Practice Manager", "healthcare", ""},
		{"IT Manager", "devops", ""},
		{"Engineering Manager", "software_engineering", ""},
		{"Senior Engineering Manager, Platform", "software_engineering", ""},
		{"Mechanical Engineering Manager", "industrial_engineering", "the discipline is named"},
		{"Project Engineering Manager", "industrial_engineering", ""},
		{"Sales Engineering Manager", "solutions_engineering", ""},
		{"Data Engineering Manager", "data_engineering", "unchanged"},
		{"Site Reliability Engineering Manager", "sre", "unchanged"},
		{"Territory Manager", "sales", ""},
		{"Product Manager", "product", "unchanged"},
		{"Project Manager", "project_management", "unchanged"},
		{"Sales Manager", "sales", "unchanged"},
		{"Nurse Case Manager", "healthcare", "the nurse names it"},
		{"Foster Care Case Manager", "social_services", "foster care names it"},
		{"Case Manager", "", "a nurse as often as a social worker: no guess"},
		{"General Manager", "", "no craft; role_type reaches it"},
		{"Assistant Manager", "", "no craft; role_type reaches it"},
		{"District Manager", "", "a chain's multi-unit operator, not a field-sales seat"},
		{"Manager", "", "no fall-through any more"},
		{"Менеджер", "", "no fall-through any more"},
		{"Менеджер по продажам", "sales", "unchanged"},
		{"Менеджер по продаже услуг", "sales", ""},
		{"Менеджер по логистике", "logistics", ""},
		{"Менеджер по закупкам", "operations", ""},
		{"Менеджер проектов", "project_management", ""},
		{"Hr-менеджер", "hr", ""},
		{"PR-менеджер", "marketing", ""},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.want {
			t.Errorf("Parse(%q).Category = %q, want %q %s", c.title, got, c.want, c.why)
		}
	}
}

// No alias may resolve the retired category: it is absent from vocab.CategoryValues, and
// TestCanonicalValuesAreInVocabulary would catch a stray entry — this states why.
func TestNothingResolvesTheRetiredManagementCategory(t *testing.T) {
	for _, e := range categoryTable {
		if e.canonical == vocab.RetiredCategoryManagement {
			t.Errorf("alias %q still resolves the retired %q category", e.alias, e.canonical)
		}
	}
	if _, ok := CategoryAliases()[vocab.RetiredCategoryManagement]; ok {
		t.Error("CategoryAliases still offers the retired category to the web role picker")
	}
}

// An occurrence inside a span an earlier alias already matched is not read again, so a
// pin holds on the multi-category path as it does for Parse.
func TestCategories_ClaimedSpanIsNotReadAgain(t *testing.T) {
	cases := []struct {
		text string
		want []string
	}{
		{"Front End Cashier", []string{"retail"}},
		{"Chief Information Security Officer", []string{"security"}},
		{"Software Design Engineer", nil},
		// The shorter alias still counts where it occurs outside every claimed span.
		{"Front End Cashier and Frontend Developer", []string{"retail", "frontend"}},
		{"Security Officer, Information Security", []string{"security", "public_safety"}},
	}
	for _, c := range cases {
		if got := Categories(c.text); !slices.Equal(got, c.want) {
			t.Errorf("Categories(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}

// The remote tail (2026-10-19): the families that left 40% of open remote postings
// without a category, and the lookalikes that must keep resolving to nothing.
func TestParse_RemoteTail(t *testing.T) {
	for _, tc := range []struct{ title, want string }{
		{"Real Estate Showing Agent - Kingman", "sales"},
		{"Agent immobilier indépendant - H/F", "sales"},
		{"Work From Anywhere-Licensed Life Insurance Agents", "sales"},
		{"Remote Benefits Representative", "sales"},
		{"Mortgage Loan Originator", "finance"},
		{"Remote Travel Advisor (Flexible Hours | No Experience Required)", "hospitality"},
		{"Board Certified Behavior Analyst (BCBA) - Remote", "healthcare"},
		{"Licensed Marriage and Family Therapist (LMFT)", "healthcare"},
		{"Telehealth Registered Dietitian (1099)", "healthcare"},
		{"Spa Therapist", "personal_services"},
		{"Experienced Spanish Interpreter (LATAM)", "technical_writing"},
		{"Creative Director, CPG", "creative"},
		{"Amazon PPC Specialist", "marketing"},
		{"React Engineer - Remote, Latin America", "frontend"},
		{"Laravel Developer + React - Remote", "backend"},
		{"Mecánico de motocicletas", "skilled_trades"},
		{"Profesor/a de\r\nguitarra - Purral", "education"},

		// Still nothing, on purpose.
		{"CAD Engineer - AI Trainer", ""},
		{"Product Engineer", ""},
		{"Tech Lead", ""},
		{"Case Manager", ""},
		{"Financial Counselor", ""},
		{"General Application", ""},
	} {
		if got := Parse(tc.title).Category; got != tc.want {
			t.Errorf("Parse(%q).Category = %q, want %q", tc.title, got, tc.want)
		}
	}
}
