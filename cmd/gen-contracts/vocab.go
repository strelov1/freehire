package main

import (
	"sort"
	"strings"
)

// emitVocab renders one closed vocabulary as a frozen value array plus a string-union
// type, e.g. emitVocab("Seniority", "SENIORITY_VALUES", […]) →
//
//	export const SENIORITY_VALUES = ['intern', …] as const;
//	export type Seniority = (typeof SENIORITY_VALUES)[number];
func emitVocab(typeName, constName string, values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = "'" + v + "'"
	}
	var b strings.Builder
	b.WriteString("export const " + constName + " = [" + strings.Join(quoted, ", ") + "] as const;\n")
	b.WriteString("export type " + typeName + " = (typeof " + constName + ")[number];\n")
	return b.String()
}

// emitMap renders a string→string map as a frozen object literal plus a type alias,
// e.g. emitMap("CityCountry", "CITY_COUNTRY_MAP", {…}) →
//
//	export const CITY_COUNTRY_MAP = {
//	  'Amsterdam': 'nl',
//	  …
//	} as const;
//	export type CityCountry = typeof CITY_COUNTRY_MAP;
//
// Keys are emitted in sorted order so the committed output is deterministic despite
// Go's randomized map iteration.
func emitMap(typeName, constName string, m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	if len(keys) == 0 {
		b.WriteString("export const " + constName + " = {} as const;\n")
	} else {
		b.WriteString("export const " + constName + " = {\n")
		for _, k := range keys {
			b.WriteString("  " + quoteTS(k) + ": " + quoteTS(m[k]) + ",\n")
		}
		b.WriteString("} as const;\n")
	}
	b.WriteString("export type " + typeName + " = typeof " + constName + ";\n")
	return b.String()
}

// quoteTS renders a string as a single-quoted TS literal, escaping backslashes and
// single quotes so a value like "N'Djamena" can't break the generated file.
func quoteTS(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
}
