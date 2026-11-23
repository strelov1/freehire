package skilltag

import (
	"slices"
	"strings"
	"testing"
)

// A currency code beside an amount is money, not a skill: PHP is also the Philippine
// peso and CAD the Canadian dollar (freehire#4980). A code beside a bare version
// number ("PHP 8", "Laravel 10 PHP 8.2") is still the language.
func TestParse_CurrencyCodeBesideAnAmountIsNotASkill(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   []string
		absent []string
	}{
		{"peso range per month", "Salary: PHP 42,600-54,200/month. Python welcome.",
			[]string{"python"}, []string{"php"}},
		{"dollar range per hour", "Pay: 5.50-7.00 CAD per hour. Python welcome.",
			[]string{"python"}, []string{"cad"}},
		{"the reported posting does not look technical", "Remote customer support rep. Log every call in the CRM. Pay is 5.50-7.00 CAD per hour (approx. PHP 42,600-54,200/month).",
			nil, []string{"cad", "crm", "php"}},
		{"title-case peso", "Php 25,000 monthly. Python welcome.", []string{"python"}, []string{"php"}},
		{"symbol before the amount", "Rate: $25 CAD/hour, paid in CAD $1,200 per week. Python welcome.",
			[]string{"python"}, []string{"cad"}},
		{"peso sign", "₱42,600 PHP a month. Python welcome.", []string{"python"}, []string{"php"}},
		{"thousands suffix", "CAD 80k-95k. Python welcome.", []string{"python"}, []string{"cad"}},
		{"plain amount with a period", "Paid 18 CAD an hour. Python welcome.", []string{"python"}, []string{"cad"}},
		{"cents", "Starting at PHP 85.50. Python welcome.", []string{"python"}, []string{"php"}},
		{"a version before the price", "Laravel 10 PHP 42,600/month", []string{"laravel"}, []string{"php"}},

		{"php version", "PHP 8, Laravel", []string{"laravel", "php"}, nil},
		{"php minor version", "Laravel 10 PHP 8.2 and MySQL", []string{"laravel", "mysql", "php"}, nil},
		{"php version range", "Upgrade from PHP 7.4-8.3 in Symfony", []string{"php", "symfony"}, nil},
		{"cad drafting", "CAD drafting with SolidWorks", []string{"cad", "solidworks"}, nil},
		{"code with no amount", "Salary in CAD, stack in PHP.", []string{"cad", "php"}, nil},
		{"years of php", "5 years PHP experience", []string{"php"}, nil},
		{"cve id is not a price", "Triage CVE 2024-12345 in our Kubernetes fleet", []string{"kubernetes", "vulnerability-management"}, nil},
		{"a year before the code", "SolidWorks 2024 CAD drafting", []string{"cad", "solidworks"}, nil},
		{"ungrouped price", "PHP 2500 per head is not a period, PHP 42600 is a price. Python welcome.",
			[]string{"python"}, []string{"php"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Parse(c.in)
			for _, w := range c.want {
				if !slices.Contains(got, w) {
					t.Errorf("Parse(%q) = %v, missing %q", c.in, got, w)
				}
			}
			for _, a := range c.absent {
				if slices.Contains(got, a) {
					t.Errorf("Parse(%q) = %v, must NOT contain %q", c.in, got, a)
				}
			}
		})
	}
}

// Every ISO 4217 code that a skill alias also spells must be in currencyCodes, or the
// money it names beside an amount tags a skill. A new alias that collides fails here,
// so each collision is a decision: list the code, or exempt it below with the reason.
func TestCurrencyCodesCoverEveryAliasCollision(t *testing.T) {
	notMoney := map[string]string{
		// The Cape Verdean escudo is all but absent from job postings, while a
		// vulnerability id written "CVE 2024-12345" would read as a price range.
		"cve": "vulnerability ids outnumber escudo salaries",
	}
	iso := strings.Fields(`AED AFN ALL AMD ANG AOA ARS AUD AWG AZN BAM BBD BDT BGN BHD BIF BMD BND
		BOB BRL BSD BTN BWP BYN BZD CAD CDF CHF CLP CNY COP CRC CUP CVE CZK DJF DKK DOP DZD EGP ERN
		ETB EUR FJD FKP GBP GEL GHS GIP GMD GNF GTQ GYD HKD HNL HTG HUF IDR ILS INR IQD IRR ISK JMD
		JOD JPY KES KGS KHR KMF KPW KRW KWD KYD KZT LAK LBP LKR LRD LSL LYD MAD MDL MGA MKD MMK MNT
		MOP MRU MUR MVR MWK MXN MYR MZN NAD NGN NIO NOK NPR NZD OMR PAB PEN PGK PHP PKR PLN PYG QAR
		RON RSD RUB RWF SAR SBD SCR SDG SEK SGD SHP SLE SLL SOS SRD SSP STN SVC SYP SZL THB TJS TMT
		TND TOP TRY TTD TWD TZS UAH UGX USD UYU UZS VES VND VUV WST XAF XCD XOF XPF YER ZAR ZMW ZWL`)
	for _, code := range iso {
		lower := strings.ToLower(code)
		_, word := wordAliases[lower]
		_, acronym := sharedAcronyms[code]
		phrase := slices.ContainsFunc(phraseAliases, func(p phraseAlias) bool { return strings.EqualFold(p.alias, code) })
		if (word || acronym || phrase) && !slices.Contains(currencyCodes, lower) && notMoney[lower] == "" {
			t.Errorf("skill alias %q is also the currency code %s; add it to currencyCodes", lower, code)
		}
	}
}
