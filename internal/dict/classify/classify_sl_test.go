package classify

import "testing"

// TestParse_SlovenianHeadsAndFields pins the Slovenian fallback (slovenian.go): live titles
// from the MojeDelo and ESS catalogues (2026-10-29) that resolved no category before, each
// filed where its English twin is.
func TestParse_SlovenianHeadsAndFields(t *testing.T) {
	cases := []struct{ title, want string }{
		// The machine operator, by the Slovenian heads and by the process he runs.
		{"Posluževalec stroja (m/ž)", "manufacturing"},
		{"Posluževalec strojev in naprav", "manufacturing"},
		{"Upravljalec stroja I (m/ž)", "manufacturing"},
		{"Upravljavec strojev za obdelavo lesa (m/ž) v okolici Celja", "manufacturing"},
		{"Upravljalec proizvodne linije (m/ž)", "manufacturing"},
		{"Upravljalec linije", "manufacturing"},
		{"Upravljalec robotizirane linije (m/ž)", "manufacturing"},
		{"UPRAVLJALEC AVTOMATSKE GALVANSKE LINIJE", "manufacturing"},
		{"Operater za brizganje plastike (m/ž)", "manufacturing"},
		{"Operater na brizgalnih strojih", "manufacturing"},
		{"Operater na upogibnem stroju - LVD (m/ž)", "manufacturing"},
		{"Operater laserskega razreza (m/ž)", "manufacturing"},
		{"Operater mehanske obdelave (m/ž)", "manufacturing"},
		{"Operater toplotne obdelave - začetnik m/ž", "manufacturing"},
		{"Operater litja m/ž", "manufacturing"},
		{"Operater v livarni (m/ž) v poslovnem področju Livarna", "manufacturing"},
		{"OPERATER NA GALVANSKI LINIJI", "manufacturing"},
		{"Žagarist, vodja izmene", "manufacturing"},
		// Warehouse and parcels.
		{"Logistični delavec (m/ž)", "logistics"},
		{"Operater visoko regalnih skladišč m/ž/d", "logistics"},
		{"Delavec v oddelku za prevzem pošiljk (m/ž)", "logistics"},
		// The engineers by discipline.
		{"Gradbeni inženir (m/ž)", "industrial_engineering"},
		{"Izkušen gradbeni inženir - statik (m/ž)", "industrial_engineering"},
		{"Gradbiščni inženir (m/ž)", "industrial_engineering"},
		{"Višji razvojni inženir strojništva (m/ž)", "industrial_engineering"},
		{"Višji inženir za kakovost", "industrial_engineering"},
		{"Razvojni inženir elektronike m/ž", "hardware"},
		{"Inženir za računalniški vid in strojno učenje (m/ž)", "ml_ai"},
		{"Inženir za omrežno varnost (m/ž)", "security"},
		{"IT Inženir za omrežja in informacijsko varnost (m/ž)", "security"},
		{"Omrežni tehnik T-2 (m/ž)", "network_engineering"},
		// Healthcare and care.
		{"Inženir laboratorijske biomedicine III - m/ž (šifra DM: E047033)", "healthcare"},
		{"VODJA II V LEKARNI", "healthcare"},
		{"VARUHINJA INSTITUCIONALNO VARSTVO F025014", "healthcare"},
		{"STROKOVNI DELAVEC NA CSD II", "social_services"},
		// The trades, the shop, the kitchen, the desks.
		{"Avtoelektrikar – Ljubljana", "skilled_trades"},
		{"Asfalterski delovodja (m/ž)", "skilled_trades"},
		{"Poslovodja (m/ž) v Murski Soboti", "retail"},
		{"VODJA IZMENE V STREŽBI", "hospitality"},
		{"Vodja slaščičarskega ateljeja (m/ž)", "hospitality"},
		{"Delavec v čistem delu pralnice (m/ž) - Trzin", "hospitality"},
		{"Operater v asistenčnem klicnem centru - m/ž", "support"},
		{"Saldakontist dobaviteljev (m/ž)", "finance"},
		{"Strokovnjak za finance - saldakonti (m/ž/d)", "finance"},
		{"Svetovalec za SAP EWM (m/ž/d)", "solutions_engineering"},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.want {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.want)
		}
	}
}

// TestParse_SlovenianUnresolved pins what stays unresolved, as its English twin does: a
// generic head with no field, a shift or branch manager naming no craft, the PLC
// programmer, the mystery shopper.
func TestParse_SlovenianUnresolved(t *testing.T) {
	for _, title := range []string{
		"Svetovalec (m/ž) – Ljubljana",
		"Višji svetovalec (m/ž)",
		"Vodja izmene (m/ž), Sežana",
		"Vodja poslovalnice (m/ž)",
		"Operater (m/ž)",
		"Referent (m/ž)",
		"Avtomatik - Programer PLC",
		"Skriti kupec (m/ž)",
		"Kotlovničar (m/ž)",
		"Aftersales (poprodajni referent)",
		// "CSD" is also a hospital's central sterile department in English.
		"CSD Technician",
	} {
		if got := Parse(title).Category; got != "" {
			t.Errorf("Parse(%q).Category = %q, want unresolved", title, got)
		}
	}
}

// TestParse_SlovenianSeniority pins the Slovenian grades: višji is senior and mlajši junior
// (the BCS "viši" and "mlađi" are spelled otherwise), the shift and team leads are lead,
// student work and traineeship intern. The assistant or deputy of a lead states no grade.
func TestParse_SlovenianSeniority(t *testing.T) {
	cases := []struct{ title, want string }{
		{"Višji svetovalec (m/ž) v poslovni enoti Koper", "senior"},
		{"VIŠJI SVETOVALEC PODROČJA I", "senior"},
		{"Višja računovodkinja", "senior"},
		{"Višji programer", "senior"},
		{"Mlajši programer", "junior"},
		{"Mlajša računovodkinja", "junior"},
		{"Operater toplotne obdelave - začetnik m/ž", "junior"},
		{"Vodja izmene (m/ž), Sežana", "lead"},
		{"Vodja skupine za kakovost dobaviteljev (m/ž)", "lead"},
		{"Vodja ekipe", "lead"},
		{"Izmenovodja (m/ž)", "lead"},
		{"Skupinovodja v sektorju prometne signalizacije (m/ž)", "lead"},
		{"Vodilni razvojni inženir za G3-PLC Hybrid komunikacijske sisteme (m/ž)", "lead"},
		{"Finančni direktor", "c_level"},
		{"Študentsko delo: Etiketiranje (m/ž)", "intern"},
		{"STROKOVNI DELAVEC NA CSD III (PRIPRAVNIŠTVO)", "intern"},
		{"Pomočnik vodje delavnice", ""},
		{"Namestnik vodje poslovne enote (m/ž) Žalec", ""},
		{"Pomočnik generalnega direktorja za pravne zadeve (B017400)", ""},
		// "Višje" is also "higher" (education, a pay class) and states no grade.
		{"Delavec z višjo izobrazbo", ""},
	}
	for _, c := range cases {
		if got := Parse(c.title).Seniority; got != c.want {
			t.Errorf("Parse(%q).Seniority = %q, want %q", c.title, got, c.want)
		}
	}
}
