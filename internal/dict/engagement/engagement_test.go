package engagement

import (
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/dict/vocab"
)

// Every case is a phrase read off a real catalogue posting in the 2026-11-03 sample
// (the 1,354 postings the public search returns for "IR35"), trimmed to the clause
// that decides it.
func TestFromText(t *testing.T) {
	cases := []struct {
		name, title, desc string
		want              []string
	}{
		// Inside.
		{"title", "Java Developer - Inside IR35 - Investment Banking", "", []string{"ir35_inside"}},
		{"rate line", "", "Up to £700/day (Inside IR35)", []string{"ir35_inside"}},
		{"status field", "", "Day rate: £500 per day. IR35 Status: Inside IR35 (Umbrella)", []string{"ir35_inside"}},
		{"bare field value", "", "Location: Filton - 60% onsite IR35: Inside SC Clearance: BPSS", []string{"ir35_inside"}},
		{"field with spaced colon", "", "Remote – occasional travel may be required IR35 : Inside Pay rate to intermediary", []string{"ir35_inside"}},
		{"value before the name", "", "The role is IR35 inside and requires SC clearance", []string{"ir35_inside"}},
		{"within", "", "Staff or contract (within IR35) options available.", []string{"ir35_inside"}},
		{"falls within", "", "The contract will last 3 to 6 months, falls within IR35 and offers a daily rate", []string{"ir35_inside"}},
		{"in-scope", "", "Pay rate: an hourly rate, in-scope IR35, of £14.90 PAYE", []string{"ir35_inside"}},
		{"inside the scope of", "", "The day rate is £350, inside the scope of IR35.", []string{"ir35_inside"}},
		{"inside scope of", "", "The rate is £552 per day inside scope of IR35.", []string{"ir35_inside"}},
		{"glued to umbrella", "", "Rate: up to £538.20 p/d Umbrellainside IR35 Own and evolve our Jenkins", []string{"ir35_inside"}},
		{"spaced and hyphenated name", "", "£450 per day, Inside IR-35. Also inside IR 35 for the extension.", []string{"ir35_inside"}},
		{"next line glued on inside", "", "Hybrid, 2–3 days onsite per weekContract – Inside IR35November 26 Start", []string{"ir35_inside"}},
		{"classification field", "", "Contract classification: Inside IR35 Daily rate breakdown", []string{"ir35_inside"}},
		// Outside.
		{"title outside", "Solution Architect Outside IR35", "", []string{"ir35_outside"}},
		{"outside of", "", "6 month (rolling) contract outside of IR35 £500 to £550 per day", []string{"ir35_outside"}},
		{"misspelt", "", "£500-£550 p/d outsdie of IR35 please call", []string{"ir35_outside"}},
		{"field outside", "", "Rate: up to £570 per day IR35: Outside Duration: initial 6 months", []string{"ir35_outside"}},
		{"status is outside", "", "a day rate between £500 and £600. IR35 status is outside. You will develop", []string{"ir35_outside"}},
		{"value before the name outside", "", "ensure stable releases. IR35 Outside.", []string{"ir35_outside"}},
		{"hashtag", "", "#sourcetopay #architecture #outsideir35 #contractjobs", []string{"ir35_outside"}},
		{"next line glued on", "", "Clojure | React | £550/day Outside IR35We are looking for an engineer", []string{"ir35_outside"}},
		{"b2b outside", "", "Assignment; B2B contract (outside IR35)", []string{"ir35_outside", "b2b"}},
		// Both stated.
		{"either", "", "The roles can be engaged Inside or Outside IR35 and will suit engineers", []string{"ir35_outside", "ir35_inside"}},
		{"field offers either", "", "Rate: £600/day IR35: Inside or Outside", []string{"ir35_outside", "ir35_inside"}},
		{"field offers either, reversed", "", "IR35 status: Outside / Inside, by agreement", []string{"ir35_outside", "ir35_inside"}},
		{"title and body disagree", "Data Engineer - Outside IR35", "Rate £500 inside IR35 via umbrella", []string{"ir35_outside", "ir35_inside"}},
		// Not stated.
		{"pending", "", "Contract Rate: £350 - £400 per day (LTD - IR35 Determination Pending)", nil},
		{"being confirmed", "", "Day rate and IR35 determination being confirmed.", nil},
		{"exempt client", "", "As our client is considered exempt for IR35 purposes, the successful candidate will be responsible for assessing IR35 status", nil},
		{"hr duty", "IR35 Specialist (London, ENG)", "Conduct and document IR35 status assessments and issue SDSs.", nil},
		{"tax adviser", "Senior Manager (Employment Tax)", "matters (including PAYE, NICs, benefits-in-kind, IR35, global mobility)", nil},
		{"bare mention", "FullStack Infrastructure Engineer", "London - UK Contract - IR35 The Role We are seeking", nil},
		{"denied", "", "Please note this role is not inside IR35.", nil},
		{"denied with an article", "", "This is not an Inside IR35 role.", nil},
		{"denied with to be", "", "This is not to be inside IR35.", nil},
		{"word inside another", "", "Outside IR350 is a part number; inside ir3500 too.", nil},
		{"empty", "", "", nil},

		// United States.
		{"w2 only, c2c refused", "", "Hybrid onsite model On W2 only No C2C Need someone strong", []string{"w2"}},
		{"w2 label in title", "Business Intelligence Developer (W2 Only)", "", []string{"w2"}},
		{"w2 and 1099 offered", "", "What We Offer! 1099 and W-2 options to fit your preferred schedule", []string{"w2", "1099"}},
		{"payroll forms are a duty", "Payroll Specialist", "Prepare and distribute W-2 and 1099 forms at year end.", nil},
		{"german pay grade", "Professur", "Besoldung nach W2 der Bundesbesoldungsordnung", nil},
		{"1099 contractor", "", "Mental Health Therapist — 1099 Contractor. Position Summary", []string{"1099"}},
		{"1099 is a price", "", "Our premium plan costs $1099 per year for a family of four.", nil},
		{"c2c rate", "", "Pay rate: $85 – $95/hr on C2C Location: 100% Remote", []string{"c2c"}},
		{"c2c refused in a list", "", "US Citizenship Required NO VISA SUPPORT or C2C Primary Function", nil},
		{"c2c not open", "", "This role is NOT open to corp2corp (C2C) or corp2hire (C2H).", nil},
		{"c2c third party refused", "", "No sponsorship needed. No third-party/C2C candidates.", nil},
		{"refused in a comma list", "", "We cannot accept H-1B, F-1/STEM OPT, 1099, C2C, or C2H arrangements.", nil},
		{"c2c marketplace", "", "Mercari is Japan's biggest C2C marketplace founded in 2013", nil},
		{"contract to hire", "Web Developer (Contract to Hire)", "", []string{"contract_to_hire"}},
		{"c2h duration", "", "Contract Duration: 12 Months (C2H) Budget: Up to 24 LPA", []string{"contract_to_hire"}},
		{"recruiter skill", "IT Recruiter", "Strong experience in IT recruitment (C2C/C2H/FTE).", nil},

		// Poland and Central Europe.
		{"justjoin pay paragraph", "", "<p><strong>Wynagrodzenie</strong><br>umowa o pracę: 16000 – 18000 PLN / mies. brutto<br>B2B: 20000 – 24000 PLN / mies. netto</p>", []string{"b2b", "uop"}},
		{"forma zatrudnienia", "", "Forma zatrudnienia: umowa o pracę/ B2B Tryb pracy: hybrydowy", []string{"b2b", "uop"}},
		{"b2b tag in title", "Senior PAM Business Analyst (B2B)", "", []string{"b2b"}},
		{"b2b sales", "B2B Sales Representative", "Grow our B2B sales pipeline across DACH.", nil},
		{"cbop field", "", "Rodzaj umowy: Umowa zlecenie / Umowa o świadczenie usług Wynagrodzenie brutto", []string{"umowa_zlecenie"}},
		{"either of two", "", "Rodzaj umowy: umowa zlecenie lub umowa o dzieło.", []string{"umowa_zlecenie", "umowa_o_dzielo"}},
		{"payroll duty", "Specjalista ds. płac", "Rozliczanie umów cywilnoprawnych (np. umowy zlecenia, umowy o dzieło).", nil},

		// Brazil.
		{"clt and pj", "", "Regime de contratação: CLT ou PJ, a combinar.", []string{"clt", "pj"}},
		{"clt efetivo", "", "Assistente de SDR. Efetivo (CLT) Assistente SDR", []string{"clt"}},
		{"pj in title", "Desenvolvedor(a) Python - Remoto - PJ", "", []string{"pj"}},
		{"pj seller", "Vendedor PJ - Fixo de R$ 2.000 + Comissão", "", []string{"pj"}},
		{"no is a preposition", "", "Vaga no regime CLT, com benefícios.", []string{"clt"}},
		{"no before both", "", "Contratação no regime CLT ou PJ.", []string{"clt", "pj"}},
		{"portuguese refusal", "", "Não contratamos no regime CLT.", nil},
		{"corporate clients", "Gerente de Negócios PJ - Sicredi", "Atender a carteira de associados PJ do segmento PJ.", nil},

		// Russia and the CIS.
		{"tk", "", "Условия: Оформление по ТК РФ, официальная зарплата", []string{"tk"}},
		{"tk or gph", "", "Официальное трудоустройство (трудовой договор или ГПХ по договоренности)", []string{"tk", "gph"}},
		{"ip and self-employed", "", "Наши условия: Оформление по ИП/Самозанятости, удаленный формат", []string{"samozanyaty", "ip"}},
		{"employer name", "", "Вакансия компании: ИП Комаров А. С. В команду требуется", nil},
		{"accountant for entrepreneurs", "Бухгалтер по сопровождению ИП", "Ведение бухгалтерии ИП.", nil},
		{"bank's clients", "", "Анализ деятельности заемщиков (ЮЛ, ИП, самозанятых), расчет показателей", nil},
		{"a product's clients", "", "Для предпринимательского статуса (ИП, самозанятый или обычное физическое лицо)", nil},
		{"hr knows the code", "", "Требования: знание ТК РФ, кадровое делопроизводство", nil},

		// Germany, the Netherlands, France.
		{"freelance lecturer", "Meister / Techniker (m/w/d) als Dozent (freiberuflich)", "", []string{"freiberuflich"}},
		{"freelancers as clients", "", "Wir beraten Handwerker, Freiberufler und vermögende Privatpersonen.", nil},
		{"agency contract field", "", "Vertragsart: Festanstellung (Arbeitnehmerüberlassung) Befristung: unbefristet", []string{"arbeitnehmerueberlassung"}},
		{"agency portfolio", "", "Unser Portfolio umfasst die Arbeitnehmerüberlassung, Personalvermittlung und Outsourcing.", nil},
		{"zzp", "", "Deze rol is in te vullen in vast dienstverband of als ZZP’er.", []string{"zzp"}},
		{"zzp refused in the title", "Java Developer (geen ZZP)", "", nil},
		{"zzp clients", "", "Wij werken met uiteenlopende klanten van zzp’ers tot grote organisaties.", nil},
		{"auto-entrepreneur status", "", "Votre profil : Statut auto-entrepreneur obligatoire", []string{"auto_entrepreneur"}},
		{"entrepreneurship", "GTM Manager – Micro Entrepreneurship", "", nil},
		{"portage offered", "", "Un(e) Développeur(euse) Java en freelance ou portage salarial.", []string{"portage_salarial"}},
		{"portage refused", "", "Poste proposé exclusivement en CDI (hors prestation, freelance et portage salarial).", nil},

		// Spain and Argentina.
		{"autonomo driver", "Conductor autónomo en Bilbao", "", []string{"autonomo"}},
		{"working autonomously", "", "Capacidad de trabajo autónomo y en equipo.", nil},
		{"tax work on monotributo", "Contador/a", "Liquidación de Ganancias Sociedades, Bienes Personales y Monotributo.", nil},
		{"monotributo", "", "Requisitos: Contar con monotributo activo o la posibilidad de activarlo.", []string{"monotributo"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := FromText(c.title, c.desc); !slices.Equal(got, c.want) {
				t.Errorf("FromText(%q, %q) = %v, want %v", c.title, c.desc, got, c.want)
			}
		})
	}
}

// A field read as visible text: a tag between the name and its value cannot hide it.
func TestFromText_Markup(t *testing.T) {
	got := FromText("", "<ul><li><b>IR35:</b> <i>Outside</i></li></ul>")
	if want := []string{"ir35_outside"}; !slices.Equal(got, want) {
		t.Errorf("FromText = %v, want %v", got, want)
	}
}

// The rules are the vocabulary, in its order, and every value has a label: the SPA's
// chips are generated from both, so a missing rule is a chip that never matches.
func TestRulesCoverVocabulary(t *testing.T) {
	var slugs []string
	for _, r := range rules {
		slugs = append(slugs, r.slug)
	}
	if !slices.Equal(slugs, vocab.EngagementValues) {
		t.Errorf("rules = %v, want vocab.EngagementValues %v", slugs, vocab.EngagementValues)
	}
	labels := Labels()
	for _, v := range vocab.EngagementValues {
		if labels[v] == "" {
			t.Errorf("no label for %q", v)
		}
	}
}
