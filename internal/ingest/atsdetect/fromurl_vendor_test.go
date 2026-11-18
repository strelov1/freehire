package atsdetect

import (
	"testing"
)

// TestFromURLHarriApplyLinks feeds real Harri apply links (harri.com/<brand slug>/job/<id>-,
// collected 2026-10-20) through FromURL. Every slug here resolved to a live brand through the
// platform's slug lookup, so each is the board the adapter crawls.
func TestFromURLHarriApplyLinks(t *testing.T) {
	cases := []struct{ url, board string }{
		{"https://harri.com/gk-beo1m4ztp4bs/job/2839063-", "gk-beo1m4ztp4bs"},
		{"https://harri.com/bicos/job/2793867-", "bicos"},
		{"https://harri.com/jollibeegroupna/job/2842103-", "jollibeegroupna"},
		{"https://harri.com/brindisakitchenscareers/job/2836139-", "brindisakitchenscareers"},
		{"https://harri.com/sa-asian-concepts-llc-dba-jollibee/job/2829329-", "sa-asian-concepts-llc-dba-jollibee"},
		{"https://harri.com/ghaijobs/job/2843117-", "ghaijobs"},
		{"https://harri.com/5star/job/2841805-", "5star"},
		{"https://harri.com/the-arts-club-38/job/2840208-", "the-arts-club-38"},
		{"https://harri.com/wendysukcareers/job/2835499-", "wendysukcareers"},
		{"https://harri.com/kgk-enterprises-dba-papa-john-s-us/job/2838358-", "kgk-enterprises-dba-papa-john-s-us"},
		{"https://harri.com/kjsdhflkcxnckjxhfnkjdhxfncdkgvjnrhcfjxhmkhjsdfc/job/2842490-", "kjsdhflkcxnckjxhfnkjdhxfncdkgvjnrhcfjxhmkhjsdfc"},
	}
	for _, c := range cases {
		provider, board, ok := FromURL(c.url)
		if !ok || provider != "harri" || board != c.board {
			t.Errorf("FromURL(%q) = (%q, %q, %v), want (harri, %q, true)", c.url, provider, board, ok, c.board)
		}
	}
}

// TestFromURLHCSHiring pins real apply links to symplr career centers (sampled 2026-10-20):
// every <tenant>.hcshiring.com/jobs/<id> link names its tenant as the hcshiring board, and the
// vendor's other products, which share no API with it, stay unrecognised.
func TestFromURLHCSHiring(t *testing.T) {
	boards := []struct{ url, board string }{
		{"https://capricommunities.hcshiring.com/jobs/TugN1ntScEij8OCB6I_Ezw", "capricommunities"},
		{"https://midwesthealth.hcshiring.com/jobs/7qxWmZzGVkKV65Juq1AJmg", "midwesthealth"},
		{"https://homewood.hcshiring.com/jobs/QipljehazEeuiHgKt-hX6Q", "homewood"},
		{"https://hawkeyecare.hcshiring.com/jobs/uOXXNZ8LBUWtzWh1NK3MrA", "hawkeyecare"},
		{"https://adfinitashealth.hcshiring.com/jobs/b8bT4Kwb0kGilISu7frcbA", "adfinitashealth"},
		{"https://sandyridgerehab.hcshiring.com/jobs/S8dfJO8TA0W7C7Gu2H5UlA", "sandyridgerehab"},
		{"https://riverviewlodgeal.hcshiring.com/jobs/_YMF6czAe0yKvrkSlh0NqA", "riverviewlodgeal"},
		{"https://orchardhealthandrehabilitation.hcshiring.com/jobs/PC6BEtLDg0GzxcouNRN0VA", "orchardhealthandrehabilitation"},
		{"https://paddingtonplacecottages.hcshiring.com/jobs/E4r5Pd4AFEarHTai6-l77Q", "paddingtonplacecottages"},
		{"https://birchhillhs.hcshiring.com/jobs/KLN--9GXqEW8LNqAiWqp2A", "birchhillhs"},
		{"https://regencymorristown.hcshiring.com/jobs/x3-ViUISqkimatByP1GJKA", "regencymorristown"},
		{"https://well5as.hcshiring.com/jobs/nZPRrHO_1UaArr4-Il5CzA", "well5as"},
		{"https://myrtlepoint.hcshiring.com/jobs/hWdmukUkFUCp5gEI6RUP_Q", "myrtlepoint"},
		{"https://chestnut.hcshiring.com/jobs/lT4J6VKsb02SfTYgkmkhQA", "chestnut"},
		{"https://howardvillage.hcshiring.com/jobs/ANwpKMNmR0S4WjoeJsgt-g", "howardvillage"},
		{"https://wisconsinrapidshs.hcshiring.com/jobs/hO31qdIjNESM2pYG6haCDA", "wisconsinrapidshs"},
		{"https://goldensands.hcshiring.com/jobs/6t_OchT_sE-vdMQKYfJUqQ", "goldensands"},
		{"https://traditionsofhershey.hcshiring.com/jobs/YFoBcXpOeE23j39SH3Wq7Q", "traditionsofhershey"},
		{"https://calveriahartville.hcshiring.com/jobs/c8E9SO2-EEGEOzuxQhhDVA", "calveriahartville"},
		{"https://regencypark.hcshiring.com/jobs/h0Qlrs3slkePTXuxRW1-XQ", "regencypark"},
		{"https://countrylaneicf.hcshiring.com/jobs/pFV0DM_ONUiRDqDQYirYGw", "countrylaneicf"},
		{"https://soldiersgrovehs.hcshiring.com/jobs/uQKX74IrGE2YwAMTNLpqVQ", "soldiersgrovehs"},
		{"https://alamar.hcshiring.com/jobs/CB45JUXffkWVPGBlBT5cNQ", "alamar"},
		{"https://harringtonparkhealthandrehab.hcshiring.com/jobs/V8XzEs2tdkOW5FDwCovQow", "harringtonparkhealthandrehab"},
		{"https://bolingreenhealthandrehabilitation.hcshiring.com/jobs/wKyNItlptkKsQw6SdZH6bg", "bolingreenhealthandrehabilitation"},
		{"https://solarishealthcarelakezephyr.hcshiring.com/jobs/k8vUK4WLd0Gsfm3R1duH3Q", "solarishealthcarelakezephyr"},
		{"https://kensington.hcshiring.com/jobs/5R4PGRmW5kehEodflpoljA", "kensington"},
		{"https://agewellsolvere.hcshiring.com/jobs/eBs-usikvE-Ojl8-ton2kQ", "agewellsolvere"},
		{"https://capricommunitiesaccount.hcshiring.com/jobs/I0JmSDI2LkOLb5Vjj1d8kg", "capricommunitiesaccount"},
		{"https://tampalakes.hcshiring.com/jobs/kBdbVUxqo0O79kJ9xm7Lxw", "tampalakes"},
	}
	for _, c := range boards {
		provider, board, ok := FromURL(c.url)
		if !ok || provider != "hcshiring" || board != c.board {
			t.Errorf("FromURL(%q) = (%q, %q, %v), want (hcshiring, %q, true)", c.url, provider, board, ok, c.board)
		}
	}

	for _, u := range []string{
		"https://valleywise.hctsportals.com/jobs/2237498",
		"https://jobs-pediatricassociates.hctsportals.com/jobs/2224904",
		"https://pm.healthcaresource.com/CS/rrh/#/job/2474",
		"https://pm.healthcaresource.com/CS/exeterhospital/#/job/13541",
	} {
		if provider, board, ok := FromURL(u); ok {
			t.Errorf("FromURL(%q) = (%q, %q), want unrecognised: another symplr product", u, provider, board)
		}
	}
}
