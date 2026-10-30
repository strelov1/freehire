package normalize

import "testing"

func TestLatinizeSerbian(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Медицинска сестра-техничар", "Medicinska sestra-tehničar"},
		{"“ЗЛАТИБОРСКИ КОНАЦИ” ДОО", "“ZLATIBORSKI KONACI” DOO"},
		{"Љубав", "Ljubav"},
		{"ЉУБАВ", "LJUBAV"},
		{"НАЈЉЕПШИ", "NAJLJEPŠI"},
		{"Џ", "Dž"}, // a lone capital reads as an initial
		{"ЏЕМ", "DŽEM"},
		{"Џемпер", "Džemper"},
		{"Њ.", "Nj."},
		{"ђак Ђорђе ћуд Ћирић џеп", "đak Đorđe ćud Ćirić džep"},
		{"Dom zdravlja Niš", "Dom zdravlja Niš"},
		{"<p>Потребан <b>васпитач</b></p>", "<p>Potreban <b>vaspitač</b></p>"},
		{"", ""},
	} {
		if got := LatinizeSerbian(c.in); got != c.want {
			t.Errorf("LatinizeSerbian(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
