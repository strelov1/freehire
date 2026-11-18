package engagement

import (
	"regexp"
)

var rules = []rule{
	// ---- United Kingdom ----
	{slug: "ir35_outside", label: "Outside IR35", market: "United Kingdom", patterns: []*regexp.Regexp{
		// "outsdie" is a misspelling seen in postings; "out of scope" is HMRC's own
		// wording for the same determination.
		re(`\b(?:outside|outsdie|out[- ]of[- ]scope)` + ir35Scope + ir35),
		re(ir35Field + `(?:outside|out[- ]of[- ]scope)\b`),
		re(`#outsideir35\b`),
		// "Outside or Inside IR35": the name follows the second status only.
		re(`\boutside` + either + `inside` + ir35Scope + ir35),
		// "IR35: Inside or Outside": the field's second value.
		re(ir35Field + `(?:inside|in[- ]scope)` + either + `(?:outside|out[- ]of[- ]scope)\b`),
	}},
	{slug: "ir35_inside", label: "Inside IR35", market: "United Kingdom", patterns: []*regexp.Regexp{
		// "Umbrellainside IR35" is a template that glued the two words; it is the
		// umbrella company the inside status implies, so the glued form still counts.
		re(`(?:\bumbrella|\b)(?:inside|within|in[- ]scope)` + ir35Scope + ir35),
		re(ir35Field + `(?:inside|in[- ]scope)\b`),
		re(`#insideir35\b`),
		re(`\binside` + either + `outside` + ir35Scope + ir35),
		re(ir35Field + `(?:outside|out[- ]of[- ]scope)` + either + `(?:inside|in[- ]scope)\b`),
	}},

	// ---- United States ----
	// W-2: almost always the posting's own label ("W2 only", "W-2 hourly position",
	// "$45/hr W2"). The duty guards take payroll's forms out; "W2/W3" is a German
	// professorship's pay grade.
	{slug: "w2", label: "W-2", market: "United States",
		patterns:     []*regexp.Regexp{re(B + `w-?2` + E)},
		rejectBefore: re(`(?:` + usTaxDutyBefore + `|besoldung[^.;:]{0,20})$`),
		rejectAfter:  re(usTaxDutyAfter + `|^\s*[/-]?\s*w-?3\b|^\s*-?professur`),
	},
	// 1099: the number is also a price ("$1,099") and an address, so it needs a contract
	// word near it ("1099 contractor", "independent contractor (1099)").
	{slug: "1099", label: "1099", market: "United States",
		patterns:     []*regexp.Regexp{re(B + `1099` + E)},
		rejectBefore: re(`(?:[$€£]\s*|\d[,.]?|form\s+|` + usTaxDutyBefore + `)$`),
		rejectAfter:  re(usTaxDutyAfter + `|^[,.]\d`),
		context:      re(`contract|independent|w-?2|basis|position|role|employment|opportunit|status|engagement|freelance|per diem|self[- ]employed|own boss|\bpay\b|/hr|hourly`),
	},
	// Corp-to-corp: the marketplace sense (Mercari) and the recruiter's skill are
	// rejected, and most of the rest refuse it ("no C2C").
	{slug: "c2c", label: "Corp-to-Corp (C2C)", market: "United States",
		patterns:     []*regexp.Regexp{re(B + `(?:c2c|corp[- ]to[- ]corp|corp2corp)` + E)},
		rejectBefore: recruitingDuty,
		rejectAfter:  re(`^\s*(?:market|platform|app\b|e-?commerce|business|company|commerce|segment|transaction|sales|payments?|resale|model|startup)`),
		context:      re(`contract|w-?2|1099|c2h|\brate|basis|candidate|visa|h-?1b|\bonly\b|accept|allow|position|role|engagement|employment|duration|months|hourly|/hr|vendor|third[- ]part|\bok\b|restriction|tax term`),
	},
	{slug: "contract_to_hire", label: "Contract-to-hire", market: "International",
		patterns: []*regexp.Regexp{
			re(B + `(?:contract[- ]to[- ](?:hire|perm(?:anent)?)|temp[- ]to[- ](?:hire|perm(?:anent)?)|c2h|right[- ]to[- ]hire)` + E),
		},
		rejectBefore: recruitingDuty,
	},

	// ---- Poland and Central Europe ----
	// B2B: only its contract anchors; "B2B sales" is most of what the bare word means.
	{slug: "b2b", label: "B2B contract", market: "Poland & Central Europe",
		patterns: []*regexp.Regexp{
			re(B + `b2b\s*(?:contract|agreement|basis|cooperation|engagement|umow|kontrakt|współprac|-?\s*tätigkeit|-?\s*vertrag)`),
			re(`(?:umow[aęy]|kontrakt(?:u|em)?|contract|współprac[aęy]|cooperation|engagement|forma\s+(?:zatrudnienia|współpracy)\s*:?|rodzaj\s+umowy\s*:?|typ\s+umowy\s*:?|type\s+of\s+(?:contract|employment|agreement)\s*:?|contract\s+type\s*:?|employment\s+type\s*:?|zatrudnienie|na|on|via|through|form\s+of\s+(?:cooperation|employment)\s*:?)\s+(?:a\s+)?b2b` + E),
			// The salary label: "B2B: 20000 – 24000 PLN / mies. netto" (justjoin.it
			// writes contract types this way), "B2B (netto)".
			re(B + `b2b\s*(?:[:–-]\s*(?:od\s+|from\s+|up to\s+|do\s+)?\d|\(\s*(?:netto|net|\+\s*vat))`),
			re(B + `b2b` + either + `(?:uop|umow[aęy]\s+o\s+prac|employment contract|permanent|umow[aęy]\s+zleceni)`),
			re(`(?:uop|umow[aęy]\s+o\s+prac[ęe]|employment contract|permanent contract)` + either + `b2b` + E),
			re(`(?:netto|net|pln|eur|usd|zł|\d)\s*(?:\+\s*vat\s*)?(?:/\s*\pL+\.?\s*)?\(\s*b2b\s*\)`),
		},
		// A Polish IT title tags the contract in brackets: "Senior Software Engineer (B2B)".
		titlePatterns: []*regexp.Regexp{re(`\(\s*b2b\s*\)`)},
		rejectAfter:   re(`^\s*(?:sales|saas|market|marketing|segment|client|customer|compan|platform|e-?commerce|business|products?|solutions?|software|space|sector|environment|deals?|accounts?|partners?|leads?|services?|team|brand|payments?|ecommerce|sprzeda|klient|rynk)`),
	},
	// The three Polish contracts: CBOP's "Rodzaj umowy:" field states one on every
	// posting, rocketjobs and justjoin.it in the text.
	{slug: "uop", label: "Umowa o pracę", market: "Poland",
		patterns:     []*regexp.Regexp{re(B + `umow[aęy]\s+o\s+prac[ęe]` + E), re(B + `uop` + E)},
		rejectBefore: plDuty,
	},
	{slug: "umowa_zlecenie", label: "Umowa zlecenie", market: "Poland",
		patterns:     []*regexp.Regexp{re(B + `umow[aęy]\s*(?:-\s*)?zleceni[ea]` + E)},
		rejectBefore: plDuty,
	},
	// A rare contract, offered mostly beside the mandate one.
	{slug: "umowa_o_dzielo", label: "Umowa o dzieło", market: "Poland",
		patterns:     []*regexp.Regexp{re(B + `umow[aęy]\s+o\s+dzie[łl]o` + E)},
		rejectBefore: plDuty,
	},

	// ---- Brazil ----
	// Both Brazilian rules take negatedPT: "no" is "em + o" in Portuguese ("Vaga no regime
	// CLT"), never a refusal.
	// CLT, the labour code's employment: pandapé, empregos, sólides and catho carry it,
	// as "contratação CLT", "regime CLT", "CLT (Efetivo)", or a title tag.
	{slug: "clt", label: "CLT", market: "Brazil",
		patterns: []*regexp.Regexp{
			re(`(?:regime|contrata[çc][ãa]o|contrato|vagas?|modelo|modalidade|tipo\s+de\s+contrato|v[íi]nculo|efetivo|registro|carteira assinada)\s*(?:de\s+|em\s+|:\s*|\(\s*|[-–]\s*)?(?:trabalho\s+)?clt` + E),
			re(B + `clt\s*(?:\(\s*efetivo|efetivo|tempor[áa]rio|\+)`),
			re(B + `clt` + either + `pj` + E),
			re(B + `pj` + either + `clt` + E),
		},
		titlePatterns: []*regexp.Regexp{re(`^\s*clt\s*[-–|:]`), re(`[-–(|]\s*clt\s*\)?\s*$`)},
		negation:      negatedPT,
	},
	// PJ, a contractor's own company. The corporate-client segment is rejected.
	{slug: "pj", label: "PJ", market: "Brazil",
		patterns: []*regexp.Regexp{
			re(`(?:regime|contrata[çc][ãa]o|contrato|vagas?|modelo|modalidade|tipo\s+de\s+contrato|v[íi]nculo)\s*(?:de\s+|como\s+|:\s*|\(\s*|[-–]\s*)?pj` + E),
			re(`prestador(?:\(a\))?\s+de\s+servi[çc]os\s*\(?\s*pj`),
			re(`(?:contrata[çc][ãa]o|contratad[oa]|atua[çc][ãa]o)\s+(?:como\s+)?pessoa\s+jur[íi]dica`),
			re(B + `pj` + either + `clt` + E),
			re(B + `clt` + either + `pj` + E),
		},
		titlePatterns: []*regexp.Regexp{
			re(`[-–(|]\s*pj\s*\)?\s*\.?\s*$`), re(`^\s*pj\s*[-–|:]`), re(`\(\s*pj\s*\)`),
			// "Vendedor PJ", "Recrutador(a) Freelancer PJ", "REMOTO PJ" — a seller's or a
			// recruiter's own company; "Gerente de Negócios PJ" serves corporate clients.
			re(`(?:vendedor|representante|consultor|recrutador|remoto|home office|freelancer)\pL*(?:\(a\))?\s+pj` + E),
		},
		rejectBefore: re(`(?:segmento|carteira|clientes?|associados|empresas|contas?|cr[ée]dito|neg[óo]cios|relacionamento|atendimento|varejo)\s*$`),
		negation:     negatedPT,
	},

	// ---- Russia and the CIS ----
	// The labour code: trudvsem states "оформление по ТК РФ" on most of what it
	// carries. HR's knowledge of the code is the duty.
	{slug: "tk", label: "Трудовой договор (ТК)", market: "Russia & CIS",
		patterns: []*regexp.Regexp{
			re(`(?:оформлени\pL*|трудоустройств\pL*|работа|работаешь|официальн\pL*)\s+(?:[\pL.]+\s+){0,4}?(?:по|согласно|в\s+соответствии\s+с)\s+тк` + E),
			re(B + `(?:по\s+)?трудов(?:ой|ому)\s+договор(?:у)?` + E),
			re(B + `тк\s*(?:рф|рк|рб)?` + either + `(?:гпх|самозанят|ип` + E + `)`),
		},
		rejectBefore: re(ruDuty),
	},
	{slug: "gph", label: "ГПХ", market: "Russia & CIS",
		patterns: []*regexp.Regexp{
			re(`(?:договор[ау]?|по|оформлени\pL*\s+по)\s+гпх` + E),
			re(B + `гпх` + either), re(either + `гпх` + E),
			re(`\(\s*гпх\s*\)`),
			re(B + `гражданско-правов(?:ой|ому)\s+договор` + E),
		},
		rejectBefore: re(`(?:договоров|договоры|договорам|договорами)\s*$|` + ruDuty),
	},
	{slug: "samozanyaty", label: "Самозанятый", market: "Russia & CIS",
		patterns: []*regexp.Regexp{
			re(`(?:оформлени\pL*|работ\pL*|сотрудничеств\pL*|договор\pL*)\s+(?:\pL+\s+){0,2}?(?:как|в\s+качестве|через|по|с)\s+(?:статус\pL*\s+)?самозанят`),
			re(`самозанят\pL*` + either + `(?:ип` + E + `|гпх|тк)`),
			re(`(?:ип|гпх|тк\s*рф)` + either + `самозанят`),
			re(B + `статус\pL*\s+самозанят`),
		},
		rejectBefore: re(ruDuty),
		rejectAfter:  ruClientsAfter,
	},
	{slug: "ip", label: "ИП", market: "Russia & CIS",
		patterns: []*regexp.Regexp{
			re(`(?:оформлени\pL*|работ\pL*|сотрудничеств\pL*|договор\pL*)\s+(?:\pL+\s+){0,2}?(?:как|в\s+качестве|через|по|с)\s+ип` + E),
			re(B + `ип` + either + `(?:самозанят|гпх)`),
			re(`(?:самозанят\pL*|гпх)` + either + `ип` + E),
			re(B + `статус\pL*\s+ип` + E),
		},
		rejectBefore: re(ruDuty),
		rejectAfter:  ruClientsAfter,
	},

	// ---- Germany ----
	// Freelance: mostly lecturers ("Dozent (freiberuflich)"); the bare noun is as often
	// a client group ("Handwerker, Freiberufler und vermögende Privatpersonen").
	{slug: "freiberuflich", label: "Freiberuflich", market: "Germany",
		patterns: []*regexp.Regexp{
			re(`auf\s+freiberuflicher\s+basis`),
			re(B + `(?:als|eine?n?|für)\s+freiberufliche[rn]?(?:\s*/\s*r)?\s+\pL+`),
			re(`\(\s*freiberuflich\s*\)`),
			re(B + `freiberufliche\s+(?:tätigkeit|mitarbeit|zusammenarbeit|basis)`),
		},
		titlePatterns: []*regexp.Regexp{re(`freiberuf`)},
	},
	// Agency employment: the contract-type field and "im Rahmen der Arbeitnehmerüber-
	// lassung" about the position; an agency describing its own portfolio is not one.
	{slug: "arbeitnehmerueberlassung", label: "Arbeitnehmerüberlassung", market: "Germany",
		patterns: []*regexp.Regexp{
			re(`(?:vertragsart|anstellungsart|beschäftigungsart|art\s+der\s+anstellung)\s*:?\s*[^.;]{0,30}arbeitnehmerüberlassung`),
			re(`\(\s*arbeitnehmerüberlassung\s*\)`),
			re(`(?:stelle|position|tätigkeit|anstellung|einsatz|beschäftigung)\s+(?:ist|erfolgt|im|in)\s+(?:\pL+\s+){0,3}?arbeitnehmerüberlassung`),
			re(`im\s+rahmen\s+(?:der|einer)\s+arbeitnehmerüberlassung\s+(?:mit\s+option|bei|beim|für|zu|an)`),
			re(B + `in\s+arbeitnehmerüberlassung`),
		},
	},

	// ---- Netherlands ----
	{slug: "zzp", label: "ZZP", market: "Netherlands",
		patterns: []*regexp.Regexp{
			re(B + `(?:als|op)\s+(?:een\s+)?zzp(?:['’]?er|[- ]?basis)?` + E),
			re(B + `zzp[- ]?basis` + E),
			re(B + `zzp\s+(?:or|of)\s+(?:own company|eigen|bv|loondienst|detachering)`),
			re(`(?:freelance|freelancer)\s*\(\s*zzp`),
			re(`\(\s*zzp\s*\)`),
		},
		titlePatterns: []*regexp.Regexp{re(B + `zzp` + E)},
	},

	// ---- France ----
	{slug: "auto_entrepreneur", label: "Auto-entrepreneur", market: "France",
		patterns: []*regexp.Regexp{
			re(`statut\s+(?:d['’]\s*)?(?:auto|micro)[- ]?entrepreneur`),
			re(`(?:en\s+tant\s+qu['’]\s*|vous\s+êtes\s+|êtes[- ]vous\s+|etes\s+)(?:auto|micro)[- ]?entrepreneur`),
			re(`\(\s*(?:auto|micro)[- ]?entrepreneur\s*\)`),
		},
		titlePatterns: []*regexp.Regexp{re(`(?:auto|micro)[- ]?entrepreneur(?:e?s)?` + E)},
		rejectBefore:  frRefusal,
	},
	{slug: "portage_salarial", label: "Portage salarial", market: "France",
		patterns:     []*regexp.Regexp{re(`(?:en|ou|et|via|par|au)\s+portage\s+salarial`)},
		rejectBefore: frRefusal,
	},

	// ---- Spain and Argentina ----
	// Autónomo: a self-employed driver, salesperson or professional; "trabajo autónomo"
	// is working on one's own, and is not here.
	{slug: "autonomo", label: "Autónomo", market: "Spain",
		patterns: []*regexp.Regexp{
			re(`(?:como|en\s+r[ée]gimen\s+de|r[ée]gimen\s+de|alta\s+(?:como|de)|dado\s+de\s+alta\s+como|darse\s+de\s+alta\s+como|contrato\s+de|cuota\s+de)\s+aut[óo]nom[oa]s?` + E),
			re(B + `(?:trabajador|comercial|conductor|colaborador|profesional|repartidor|agente)(?:e?s)?(?:\s*/\s*a)?\s+aut[óo]nom[oa]s?` + E),
		},
		rejectBefore: esDuty,
	},
	// Monotributo: nearly every posting naming it asks the candidate to have it.
	{slug: "monotributo", label: "Monotributo", market: "Argentina",
		patterns:     []*regexp.Regexp{re(B + `monotribut(?:o|ista)` + E)},
		rejectBefore: esDuty,
	},
}
