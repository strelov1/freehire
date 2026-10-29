package location

import "strings"

// Croatian, Serbian, Bosnian and Montenegrin location lines name places in their own
// languages, and the parser read few of those names. Measured 2026-10-29 over the open
// postings placed in hr/rs/ba/me (talentlyft, eures, zohorecruit, smartrecruiters,
// teamtailor, workday and the rest, ~5,800 lines), and over the lines the new
// Serbian and Croatian boards write (infostud, hzz, mojposao): "Srbija", "Crna Gora",
// "Bosna i Hercegovina" and "BiH" stated no country; an infostud posting abroad
// ("Nemačka - Hanover", "Austrija") stated none either, so the board's own fallback
// country would have filed it in Serbia; Workday's ISO-3 export ("SERBIA VIRTUAL, SRB")
// and a Croatian county ("Splitsko-dalmatinska županija") placed nothing; and "Kosovo"
// named the town of Kosovo Polje as the city.
//
// bcsCountryNames are the country names in the BCS languages, in both standards
// (Njemačka/Nemačka, Švicarska/Švajcarska), written with and without diacritics, and in
// Serbian Cyrillic. Absent on purpose: "SAD" (the United States) is the English "sad",
// "Island" (Iceland) the English "island", and "Bosna" alone is also a river and a
// Sarajevo neighbourhood's name in other lines; the long forms carry them.
var bcsCountryNames = map[string]string{
	// The BCS countries themselves, and Workday's ISO-3 codes for them.
	"srbija": "rs", "republika srbija": "rs", "србија": "rs", "република србија": "rs", "srb": "rs",
	"cela srbija": "rs", "cijela srbija": "rs", "teritorija cele srbije": "rs", "цела србија": "rs",
	"cijela hrvatska": "hr", "republika hrvatska": "hr", "хрватска": "hr", "hrv": "hr",
	"bosna i hercegovina": "ba", "bosnia": "ba", "bih": "ba", "b&h": "ba", "босна и херцеговина": "ba",
	"cijela bih": "ba", "cela bih": "ba",
	"crna gora": "me", "црна гора": "me", "mne": "me",
	// The neighbours and the countries the boards post abroad to.
	"словенија":  "si",
	"makedonija": "mk", "sjeverna makedonija": "mk", "severna makedonija": "mk",
	"македонија": "mk", "северна македонија": "mk",
	"albanija": "al", "албанија": "al", "косово": "xk",
	"mađarska": "hu", "madjarska": "hu", "madarska": "hu", "мађарска": "hu",
	"rumunija": "ro", "rumunjska": "ro", "румунија": "ro",
	"bugarska": "bg", "бугарска": "bg",
	"grčka": "gr", "grcka": "gr", "грчка": "gr",
	"nemačka": "de", "nemacka": "de", "njemačka": "de", "njemacka": "de", "немачка": "de",
	"sr nemačka": "de", "sr nemacka": "de", "sr njemačka": "de", "sr njemacka": "de",
	"austrija": "at", "аустрија": "at",
	"švicarska": "ch", "svicarska": "ch", "švajcarska": "ch", "svajcarska": "ch",
	"швајцарска": "ch",
	"italija":    "it", "италија": "it",
	"francuska": "fr", "француска": "fr",
	"španjolska": "es", "spanjolska": "es", "španija": "es", "spanija": "es", "шпанија": "es",
	"nizozemska": "nl", "holandija": "nl", "холандија": "nl",
	"belgija": "be", "белгија": "be",
	"luksemburg": "lu", "луксембург": "lu",
	"irska": "ie", "ирска": "ie",
	"velika britanija": "gb", "ujedinjeno kraljevstvo": "gb", "велика британија": "gb",
	"švedska": "se", "svedska": "se", "шведска": "se",
	"norveška": "no", "norveska": "no", "норвешка": "no",
	"danska": "dk", "данска": "dk",
	"finska": "fi", "финска": "fi",
	"češka": "cz", "ceska": "cz", "чешка": "cz",
	"slovačka": "sk", "slovacka": "sk", "словачка": "sk",
	"poljska": "pl", "пољска": "pl",
	"estonija": "ee", "litva": "lt", "litvanija": "lt",
	"rusija": "ru", "русија": "ru",
	"ukrajina": "ua", "украјина": "ua",
	"turska": "tr", "турска": "tr",
	"kipar": "cy", "кипар": "cy",
	"kanada": "ca", "australija": "au", "аустралија": "au",
	"ujedinjeni arapski emirati": "ae", "уједињени арапски емирати": "ae",
	// Serbia's provinces and regions, Bosnia's entities, Croatia's regions. "Istra" is
	// absent: it is also a Russian town.
	"vojvodina": "rs", "војводина": "rs",
	"republika srpska": "ba", "република српска": "ba",
	"federacija bih": "ba", "federacija bosne i hercegovine": "ba",
	"brčko distrikt": "ba", "brcko distrikt": "ba",
	"dalmacija": "hr", "slavonija": "hr", "kvarner": "hr", "zagorje": "hr",
	"međimurje": "hr", "medjimurje": "hr", "medimurje": "hr",
}

// croatianCounties are Croatia's twenty counties by their adjective, as a location line
// writes them with or without "županija" ("Splitsko-dalmatinska županija",
// "SPLITSKO-DALMATINSKA"). Each is read in both spellings, with the diacritics and
// without.
var croatianCounties = []string{
	"zagrebačka", "krapinsko-zagorska", "sisačko-moslavačka", "karlovačka", "varaždinska",
	"koprivničko-križevačka", "bjelovarsko-bilogorska", "primorsko-goranska",
	"ličko-senjska", "virovitičko-podravska", "požeško-slavonska", "brodsko-posavska",
	"zadarska", "osječko-baranjska", "šibensko-kninska", "vukovarsko-srijemska",
	"splitsko-dalmatinska", "istarska", "dubrovačko-neretvanska", "međimurska",
}

// bcsASCIIFold writes a BCS name without its diacritics, đ as "dj"; bcsPlainFold writes
// đ as a plain "d", the other spelling a line without diacritics uses ("Medimurska").
var (
	bcsASCIIFold = strings.NewReplacer("č", "c", "ć", "c", "š", "s", "ž", "z", "đ", "dj")
	bcsPlainFold = strings.NewReplacer("č", "c", "ć", "c", "š", "s", "ž", "z", "đ", "d")
)

// bcsNotCity are names the city table reads as a town that state no city here:
// "Kosovo" is an alias of Kosovo Polje, so "Pristina, Kosovo" named two cities.
var bcsNotCity = []string{"kosovo", "косово", "dalmacija", "slavonija", "zagorje"}

// init merges the BCS names into nameToCountry and regionNotCity. A name already in
// nameToCountry is a programming error, as in foreignSubdivisionNames.
func init() {
	add := func(name, country string) {
		if existing, dup := nameToCountry[name]; dup {
			panic("location: bcsCountryNames[" + name + "] is already in nameToCountry as " + existing)
		}
		nameToCountry[name] = country
	}
	for name, country := range bcsCountryNames {
		add(name, country)
	}
	for _, county := range croatianCounties {
		names := map[string]bool{}
		for _, name := range []string{county, county + " županija"} {
			names[name] = true
			names[bcsASCIIFold.Replace(name)] = true
			names[bcsPlainFold.Replace(name)] = true
		}
		for name := range names {
			add(name, "hr")
		}
	}
	for _, name := range bcsNotCity {
		regionNotCity[name] = true
	}
}
