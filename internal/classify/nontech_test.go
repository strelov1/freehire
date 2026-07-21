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
