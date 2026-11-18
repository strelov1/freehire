package location

// cityOverrides pins a curated display name for aliases where the generated GeoNames
// name (cityDict, from cmd/gen-cities) differs from the clean English form wanted in
// the `cities` facet. Applied over the generated base (override wins); country
// matches the generated entry. Add an entry for a genuine GeoNames gap or spelling to
// pin, not for cities the dataset already covers. Fields are named, not positional:
// an override is hand-asserted, so it stays uncontested by construction.
var cityOverrides = map[string]cityEntry{
	// Croatian and Serbian cities whose alias GeoNames also gives a hamlet abroad, so the
	// alias stated no country (location_bcs.go); the City of Zagreb as its own lines
	// write it; the locative a BCS line puts a city in ("u Beogradu", "Novom Sadu"); and
	// the Cyrillic spelling GeoNames lacks.
	"pula":          {Name: "Pula", Country: "hr"},
	"dubrovnik":     {Name: "Dubrovnik", Country: "hr"},
	"leskovac":      {Name: "Leskovac", Country: "rs"},
	"sombor":        {Name: "Sombor", Country: "rs"},
	"панчево":       {Name: "Pančevo", Country: "rs"},
	"grad zagreb":   {Name: "Zagreb", Country: "hr"},
	"beogradu":      {Name: "Belgrade", Country: "rs"},
	"u beogradu":    {Name: "Belgrade", Country: "rs"},
	"београду":      {Name: "Belgrade", Country: "rs"},
	"у београду":    {Name: "Belgrade", Country: "rs"},
	"zagrebu":       {Name: "Zagreb", Country: "hr"},
	"u zagrebu":     {Name: "Zagreb", Country: "hr"},
	"novom sadu":    {Name: "Novi Sad", Country: "rs"},
	"u novom sadu":  {Name: "Novi Sad", Country: "rs"},
	"новом саду":    {Name: "Novi Sad", Country: "rs"},
	"u nišu":        {Name: "Niš", Country: "rs"},
	"u splitu":      {Name: "Split", Country: "hr"},
	"splitu":        {Name: "Split", Country: "hr"},
	"u rijeci":      {Name: "Rijeka", Country: "hr"},
	"osijeku":       {Name: "Osijek", Country: "hr"},
	"u osijeku":     {Name: "Osijek", Country: "hr"},
	"sarajevu":      {Name: "Sarajevo", Country: "ba"},
	"u sarajevu":    {Name: "Sarajevo", Country: "ba"},
	"podgorici":     {Name: "Podgorica", Country: "me"},
	"u podgorici":   {Name: "Podgorica", Country: "me"},
	"kragujevcu":    {Name: "Kragujevac", Country: "rs"},
	"u kragujevcu":  {Name: "Kragujevac", Country: "rs"},
	"u zadru":       {Name: "Zadar", Country: "hr"},
	"banjoj luci":   {Name: "Banja Luka", Country: "ba"},
	"segedin":       {Name: "Szeged", Country: "hu"}, // the Serbian exonym
	"u banjoj luci": {Name: "Banja Luka", Country: "ba"},
	"frankfurt":     {Name: "Frankfurt", Country: "de"}, // GeoNames: "Frankfurt am Main"
	"cologne":       {Name: "Cologne", Country: "de"},   // GeoNames: "Köln"
	"köln":          {Name: "Cologne", Country: "de"},
	"sevilla":       {Name: "Seville", Country: "es"}, // GeoNames: "Sevilla"
	"seville":       {Name: "Seville", Country: "es"},
	"zurich":        {Name: "Zurich", Country: "ch"},   // GeoNames: "Zürich"
	"montreal":      {Name: "Montreal", Country: "ca"}, // GeoNames: "Montréal"
	"montréal":      {Name: "Montreal", Country: "ca"},
	// Valencia (Venezuela) and Zaragoza outnumber their Spanish namesakes in GeoNames,
	// so without these the curated "valencia"/"zaragoza" → es named no city.
	"valencia":       {Name: "Valencia", Country: "es"},
	"zaragoza":       {Name: "Zaragoza", Country: "es"},
	"madrid capital": {Name: "Madrid", Country: "es"},
	// Mexico City's former name, and also Brasília's district: contested, so it names
	// Mexico City only in a line that states Mexico.
	"distrito federal": {Name: "Mexico City", Country: "mx", Contested: true},
	"bogotá d.c.":      {Name: "Bogotá", Country: "co"},
	// GeoNames lists "Porto" as an alias of Bordeaux, the more populous place, so a
	// "Porto, PT" line named no city at all.
	"porto":       {Name: "Porto", Country: "pt"},
	"oporto":      {Name: "Porto", Country: "pt"},
	"bogota d.c.": {Name: "Bogotá", Country: "co"},
	// The autonomous city of Buenos Aires under the names its own lines use. "CABA" was
	// GeoNames' alias of Chamba, and each of these named the country and no city.
	"caba":                   {Name: "Buenos Aires", Country: "ar"},
	"c.a.b.a.":               {Name: "Buenos Aires", Country: "ar"},
	"c.a.b.a":                {Name: "Buenos Aires", Country: "ar"},
	"capital federal":        {Name: "Buenos Aires", Country: "ar"},
	"ciudad de buenos aires": {Name: "Buenos Aires", Country: "ar"},
	"buenos aires city":      {Name: "Buenos Aires", Country: "ar"},
	// Łódź with one diacritic dropped and the other kept.
	"lódz": {Name: "Łódź", Country: "pl"},
	"łodź": {Name: "Łódź", Country: "pl"},
	"lodź": {Name: "Łódź", Country: "pl"},
	"łodz": {Name: "Łódź", Country: "pl"},
}
