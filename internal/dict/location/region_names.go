package location

// nameToRegion resolves macro-region names (and explicit open-anywhere markers)
// directly to a region code, for tokens that name an area rather than a country.
var nameToRegion = map[string]string{
	"europe": "eu", "eu": "eu", "europa": "eu",
	"apac": "apac", "asia": "apac", "asia pacific": "apac", "asia-pacific": "apac",
	"north america": "north_america",
	"latam":         "latam", "latin america": "latam", "south america": "latam",
	"mena": "mena", "middle east": "mena",
	"africa": "africa",
	"cis":    "cis", "central asia": "cis",
	// Open-anywhere markers, multilingual. A bare "remote" stays geography-less
	// (work mode only); these are explicit "the whole world" phrasings.
	"anywhere": "global", "worldwide": "global", "global": "global",
	"international": "global", "international remote": "global", "globally": "global",
	"remote anywhere": "global", "world wide": "global", "world-wide": "global",
	"everywhere": "global", "fully distributed": "global",
	"по всему миру": "global", "весь мир": "global",
	"en todo el mundo": "global", "todo el mundo": "global",
	"em todo o mundo": "global", "weltweit": "global",
}
