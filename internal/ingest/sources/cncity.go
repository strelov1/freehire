package sources

import "strings"

// cnCityPlace is a city as a Chinese careers site names it, in the English our location
// dictionary reads, with the country it is in.
type cnCityPlace struct{ name, country string }

// cnCities maps the Chinese city names the mainland careers sites state (Alibaba, Meituan) to
// English, because internal/dict/location reads no Chinese: "北京" resolves no city and no
// country, while "Beijing" resolves both. The list is not a gazetteer; a name it lacks keeps its
// Chinese spelling and states no country rather than being assumed Chinese, since the same sites list
// Tokyo and Madrid.
var cnCities = map[string]cnCityPlace{
	// Mainland China.
	"北京": {"Beijing", "cn"}, "上海": {"Shanghai", "cn"}, "杭州": {"Hangzhou", "cn"},
	"深圳": {"Shenzhen", "cn"}, "广州": {"Guangzhou", "cn"}, "成都": {"Chengdu", "cn"},
	"西安": {"Xi'an", "cn"}, "武汉": {"Wuhan", "cn"}, "苏州": {"Suzhou", "cn"},
	"南京": {"Nanjing", "cn"}, "郑州": {"Zhengzhou", "cn"}, "重庆": {"Chongqing", "cn"},
	"合肥": {"Hefei", "cn"}, "厦门": {"Xiamen", "cn"}, "长沙": {"Changsha", "cn"},
	"青岛": {"Qingdao", "cn"}, "南昌": {"Nanchang", "cn"}, "济南": {"Jinan", "cn"},
	"东莞": {"Dongguan", "cn"}, "南通": {"Nantong", "cn"}, "沈阳": {"Shenyang", "cn"},
	"南宁": {"Nanning", "cn"}, "福州": {"Fuzhou", "cn"}, "天津": {"Tianjin", "cn"},
	"宁波": {"Ningbo", "cn"}, "无锡": {"Wuxi", "cn"}, "石家庄": {"Shijiazhuang", "cn"},
	"哈尔滨": {"Harbin", "cn"}, "嘉兴": {"Jiaxing", "cn"}, "张家口": {"Zhangjiakou", "cn"},
	"温州": {"Wenzhou", "cn"}, "太原": {"Taiyuan", "cn"}, "佛山": {"Foshan", "cn"},
	"珠海": {"Zhuhai", "cn"}, "常州": {"Changzhou", "cn"}, "乌兰察布": {"Ulanqab", "cn"},
	"义乌": {"Yiwu", "cn"}, "中卫": {"Zhongwei", "cn"}, "三亚": {"Sanya", "cn"},
	"银川": {"Yinchuan", "cn"}, "贵阳": {"Guiyang", "cn"}, "泉州": {"Quanzhou", "cn"},
	"兰州": {"Lanzhou", "cn"}, "昆明": {"Kunming", "cn"}, "呼和浩特": {"Hohhot", "cn"},
	"长春": {"Changchun", "cn"}, "中山": {"Zhongshan", "cn"}, "湛江": {"Zhanjiang", "cn"},
	"西宁": {"Xining", "cn"}, "廊坊": {"Langfang", "cn"}, "海口": {"Haikou", "cn"},
	"乌鲁木齐": {"Urumqi", "cn"}, "惠州": {"Huizhou", "cn"}, "芜湖": {"Wuhu", "cn"},
	"汕头": {"Shantou", "cn"}, "洛阳": {"Luoyang", "cn"}, "扬州": {"Yangzhou", "cn"},
	"徐州": {"Xuzhou", "cn"}, "绍兴": {"Shaoxing", "cn"}, "湖州": {"Huzhou", "cn"},
	"昆山": {"Kunshan", "cn"}, "大连": {"Dalian", "cn"}, "烟台": {"Yantai", "cn"},
	"潍坊": {"Weifang", "cn"}, "保定": {"Baoding", "cn"}, "桂林": {"Guilin", "cn"},
	// Hong Kong and Macao are named with a "中国" prefix and are their own codes here.
	"中国香港": {"Hong Kong", "hk"}, "香港": {"Hong Kong", "hk"}, "香港特别行政区": {"Hong Kong", "hk"},
	"中国澳门": {"Macau", "mo"}, "澳门": {"Macau", "mo"},
	"中国台湾": {"Taipei", "tw"},
	// Abroad.
	"新加坡": {"Singapore", "sg"}, "东京": {"Tokyo", "jp"}, "大阪": {"Osaka", "jp"},
	"首尔": {"Seoul", "kr"}, "吉隆坡": {"Kuala Lumpur", "my"}, "曼谷": {"Bangkok", "th"},
	"雅加达": {"Jakarta", "id"}, "迪拜": {"Dubai", "ae"}, "利雅得": {"Riyadh", "sa"},
	"伦敦": {"London", "gb"}, "巴黎": {"Paris", "fr"}, "马德里": {"Madrid", "es"},
	"法兰克福": {"Frankfurt", "de"}, "多伦多": {"Toronto", "ca"}, "墨西哥城": {"Mexico City", "mx"},
	"巴西利亚": {"Brasilia", "br"}, "圣保罗": {"Sao Paulo", "br"}, "西雅图": {"Seattle", "us"},
	"森尼韦尔": {"Sunnyvale", "us"}, "旧金山": {"San Francisco", "us"}, "硅谷": {"Silicon Valley", "us"}, "华盛顿哥伦比亚特区": {"Washington, DC", "us"},
}

// cnCity resolves a Chinese city name, with or without the 市 ("city") suffix the sites
// sometimes add ("兰州市", "义乌市").
func cnCity(name string) (cnCityPlace, bool) {
	name = strings.TrimSpace(name)
	if p, ok := cnCities[name]; ok {
		return p, true
	}
	p, ok := cnCities[strings.TrimSuffix(name, "市")]
	return p, ok
}

// cnPlaces turns a posting's Chinese city names into its Location string and its countries:
// each city in English where known, in its own spelling where not, and a country only for a
// city the map places.
func cnPlaces(cities []string) (string, []string) {
	var names, countries []string
	seen := map[string]bool{}
	for _, c := range cities {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		p, ok := cnCity(c)
		if !ok {
			names = append(names, c)
			continue
		}
		names = append(names, p.name)
		if !seen[p.country] {
			seen[p.country] = true
			countries = append(countries, p.country)
		}
	}
	return distinctJoin(names, "; ", strings.TrimSpace), countries
}
