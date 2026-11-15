package classify

import "testing"

func TestInfrastructureSpecializations(t *testing.T) {
	cases := map[string]string{
		"Senior Systems Administrator":        "system_administration",
		"System Administrator":                "system_administration",
		"Linux Administrator":                 "system_administration",
		"Windows Administrator":               "system_administration",
		"System Administration Specialist":    "system_administration",
		"Telecommunications Network Engineer": "telecommunications_engineering",
		"Sysadmin":                            "system_administration",
		"Telecommunications Engineer":         "telecommunications_engineering",
		"Telecom Engineer":                    "telecommunications_engineering",
		"Telecoms Engineer":                   "telecommunications_engineering",
		"Ingénieur télécom":                   "telecommunications_engineering",
		"Инженер связи":                       "telecommunications_engineering",
		"Telecommunicator":                    "public_safety",
		"Database Administrator":              "devops",
		"Network Engineer":                    "network_engineering",
		"Network Administrator":               "network_engineering",
		"DevOps Engineer":                     "devops",
		"Platform Engineer":                   "devops",
		"Site Reliability Engineer":           "sre",
	}
	for title, want := range cases {
		t.Run(title, func(t *testing.T) {
			if got := Parse(title).Category; got != want {
				t.Errorf("category = %q, want %q", got, want)
			}
		})
	}
}
