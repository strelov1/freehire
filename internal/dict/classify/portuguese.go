package classify

import "regexp"

// matchPortugueseCategory resolves the category of a lowercased title the Spanish
// table left unresolved. A blind alias answers categoryNone; the caller translates it.
func matchPortugueseCategory(lower string) string {
	return matchOrdered(foldSpanish(lower), portugueseCategoryTable)
}

// portugueseSeniorityTable lists the Portuguese grade aliases, whole-word phrases on the
// folded title, in precedence order. It runs after the Spanish grade table, which
// already reads the trainee grades ("estágio", "estagiário", "aprendiz"), so a trainee
// outranks every grade below, as it does in Spanish.
//
// The accented grades "Sênior" and "Júnior" are the common Brazilian spelling, and the
// main table reads neither: "sênior" does not contain "senior". The folded title does.
//
// "Coordenador" and "chefe" lead, as the Spanish "jefe" does: in a Brazilian career
// ladder the coordinator sits above the specialist and below the manager. "Supervisor"
// is left alone — it is spelled the same in Spanish and English, whose tables state no
// grade for it, and this table also reads their titles. "Gerente" states no grade, as
// "manager" states none, and a plain "diretor" none, as a plain "director" none; only
// the general, executive and financial directors are the board.
var portugueseSeniorityTable = []aliasEntry{
	{"diretor geral", "c_level"},
	{"diretora geral", "c_level"},
	{"diretor executivo", "c_level"},
	{"diretora executiva", "c_level"},
	{"diretor presidente", "c_level"},
	{"diretora presidente", "c_level"},
	{"diretor financeiro", "c_level"},
	{"diretora financeira", "c_level"},
	{"coordenador", "lead"},
	{"coordenadora", "lead"},
	{"coordenadores", "lead"},
	{"chefe", "lead"},
	{"chefes", "lead"},
	// The grades in the main table's order: "Analista Pleno/Sênior" keeps its upper
	// bound, as "Mid/Senior" does.
	{"senior", "senior"},
	{"pleno", "middle"},
	{"plena", "middle"},
	{"junior", "junior"},
}

// assistantOfPortugueseLead is the assistant OF a lead or a director — "Assistente de
// Coordenação", "Auxiliar da Chefia", "Secretária da Diretoria Executiva", "Subchefe" —
// which states no grade of its own. It is cut before the Portuguese grade match.
var assistantOfPortugueseLead = regexp.MustCompile(`\b(?:(?:assistente|auxiliar|adjunt[oa]|secretari[oa])\s+(?:(?:d[oa]s?|a[os]?|ao)\s+)?(?:chef[ei]a?|coordenador[a]?|diretor[a]?\s+(?:geral|executiv[oa]|financeir[oa]|presidente)|diretoria)|subchefe|subcoordenador[a]?)\b`)

// brazilianGradeLadder reads the grade a Brazilian career ladder writes after the head
// and field: "Analista Fiscal PL", "Analista Financeiro II", "Técnico em Automação I".
// "PL" (pleno) and the Roman rungs say nothing on their own — "PL" is Poland and
// PL/SQL, "II" a product generation — so they count only after a Portuguese desk head
// within the same clause. I is the junior rung, II the middle and III the senior, the
// ladder the Brazilian employers in the crawl share.
var brazilianGradeLadder = regexp.MustCompile(`\b(?:analista|assistente|auxiliar|tecnic[oa]|especialista|desenvolvedor[a]?|engenheir[oa]|programador[a]?|consultor[a]?|operador[a]?|advogad[oa]|comprador[a]?|contador[a]?|projetista|atendente|agente|executiv[oa]|coordenador[a]?)\b[^|()\-–,:]*?[\s/](pl|iii|ii|i)(?:\s*$|\s*[|()\-–,/:]|\s+(?:remoto|presencial|hibrido|home|temporario|\d))`)

// plSQL is Oracle's language, whose "PL" is not the grade: "Analista de PL/SQL".
var plSQL = regexp.MustCompile(`\bpl[\s/-]?sql\b`)

// brazilianRung maps a brazilianGradeLadder capture to its grade.
var brazilianRung = map[string]string{"pl": "middle", "i": "junior", "ii": "middle", "iii": "senior"}

// matchPortugueseSeniority resolves the grade of a lowercased title the other tables
// left unresolved.
func matchPortugueseSeniority(lower string) string {
	folded := assistantOfPortugueseLead.ReplaceAllString(foldSpanish(lower), " ")
	if s := matchOrdered(folded, portugueseSeniorityTable); s != "" {
		return s
	}
	if m := brazilianGradeLadder.FindStringSubmatch(plSQL.ReplaceAllString(folded, "plsql")); m != nil {
		return brazilianRung[m[1]]
	}
	return ""
}
