package sources

import "strings"

// twPlaces maps the Traditional-Chinese place names the Taiwanese careers sites state (ASUS,
// Foxconn, Quanta, Realtek) to English, because internal/dict/location reads no Chinese: "新竹"
// resolves nothing while "Hsinchu" resolves the city and the country. cnCities serves the
// mainland sites and is Simplified (台湾, 东京), so it cannot be reused for these.
//
// The list is the names seen on those sites' live catalogues (2026-10-25), not a gazetteer. A
// country is stated as its English name, a city or campus as the place our dictionary knows.
// Every key is spelled with 台, never 臺: the sites use both for the same word (ASUS writes 臺北,
// Realtek 台北), and twPlace folds the variant before looking it up.
var twPlaces = map[string]cnCityPlace{
	// Countries, as a site's country column or a "country / city" pair's first half names them.
	"台灣": {"Taiwan", "tw"}, "日本": {"Japan", "jp"}, "新加坡": {"Singapore", "sg"},
	"加拿大": {"Canada", "ca"}, "韓國": {"South Korea", "kr"}, "阿聯": {"United Arab Emirates", "ae"},
	"香港": {"Hong Kong", "hk"}, "西班牙": {"Spain", "es"}, "越南": {"Vietnam", "vn"},
	"菲律賓": {"Philippines", "ph"}, "印度": {"India", "in"}, "美國": {"United States", "us"},
	"墨西哥": {"Mexico", "mx"}, "德國": {"Germany", "de"}, "泰國": {"Thailand", "th"},
	"大陸": {"China", "cn"}, "中國": {"China", "cn"},
	// Taiwanese cities, and the campuses the sites name instead of a city.
	"台北": {"Taipei", "tw"}, "新北": {"New Taipei", "tw"}, "新竹": {"Hsinchu", "tw"},
	"台南": {"Tainan", "tw"}, "桃園": {"Taoyuan", "tw"}, "嘉義": {"Chiayi", "tw"},
	"台中": {"Taichung", "tw"}, "高雄": {"Kaohsiung", "tw"},
	"台北市中山區": {"Taipei", "tw"}, "新竹科學園區": {"Hsinchu", "tw"}, "台南科學園區": {"Tainan", "tw"},
	// Quanta's two campuses: the Linkou R&D centre (New Taipei) and the Hwa Ya plant (Taoyuan).
	"林口研發中心": {"New Taipei", "tw"}, "林口華亞廠": {"Taoyuan", "tw"},
	// Cities abroad.
	"東京": {"Tokyo", "jp"}, "首爾": {"Seoul", "kr"}, "杜拜": {"Dubai", "ae"},
	"萬錦市": {"Markham", "ca"}, "巴賽隆納": {"Barcelona", "es"}, "胡志明市": {"Ho Chi Minh City", "vn"},
	"巴石市": {"Pasig", "ph"},
}

// twPlace resolves one Traditional-Chinese place name, folding 臺 to 台 first.
func twPlace(name string) (cnCityPlace, bool) {
	p, ok := twPlaces[strings.ReplaceAll(strings.TrimSpace(name), "臺", "台")]
	return p, ok
}

// twLocation turns a posting's place names, broadest first ("台灣", "台北"), into its Location
// string and its country: the names in English where known, in their own spelling where not,
// joined narrowest first ("Taipei, Taiwan"), and the country of the narrowest name the map places.
// A name the map lacks states no country rather than being assumed Taiwanese, since these sites
// list Tokyo and Monterrey beside Hsinchu.
func twLocation(names ...string) (string, []string) {
	var parts []string
	country := ""
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if p, ok := twPlace(n); ok {
			parts = append(parts, p.name)
			country = p.country
			continue
		}
		parts = append(parts, n)
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	var countries []string
	if country != "" {
		countries = []string{country}
	}
	return distinctJoin(parts, ", ", strings.TrimSpace), countries
}
