package skilltag

import (
	"regexp"
	"strings"
)

// Curated from Maintenance/Technical Manager postings at Flora Food, Swan Global,
// Bridge Talent, KBR, Applus+ and Nova. These disciplines and plant tools name
// industrial work; none corroborates an ambiguous software word such as "react".
var industrialMaintenancePhraseAliases = []phraseAlias{
	{"sap pm", "sap-pm"}, {"sap plant maintenance", "sap-pm"},
	{"preventive maintenance", "preventive-maintenance"},
	{"preventive and predictive maintenance", "preventive-maintenance"},
	{"preventive & predictive maintenance", "preventive-maintenance"},
	{"preventative maintenance", "preventive-maintenance"},
	{"планово предупредительный ремонт", "preventive-maintenance"},
	{"планово предупредительного ремонта", "preventive-maintenance"},
	{"predictive maintenance", "predictive-maintenance"},
	{"предиктивное обслуживание", "predictive-maintenance"},
	{"предиктивного обслуживания", "predictive-maintenance"},
	{"maintenance planning", "maintenance-planning"},
	{"maintenance scheduling", "maintenance-planning"},
	{"планирование тоир", "maintenance-planning"},
	{"планирования тоир", "maintenance-planning"},
	{"reliability engineering", "reliability-engineering"},
	{"инженерия надежности", "reliability-engineering"},
	{"инженерия надёжности", "reliability-engineering"},
	{"reliability centered maintenance", "reliability-centered-maintenance"},
	{"reliability centred maintenance", "reliability-centered-maintenance"},
	{"обслуживание ориентированное на надежность", "reliability-centered-maintenance"},
	{"обслуживание ориентированное на надёжность", "reliability-centered-maintenance"},
	{"обслуживание, ориентированное на надежность", "reliability-centered-maintenance"},
	{"обслуживание, ориентированное на надёжность", "reliability-centered-maintenance"},
	{"condition monitoring", "condition-monitoring"},
	{"мониторинг технического состояния", "condition-monitoring"},
	{"asset performance management", "asset-performance-management"},
	{"total productive maintenance", "total-productive-maintenance"},
	{"всеобщее обслуживание оборудования", "total-productive-maintenance"},
	{"overall equipment effectiveness", "overall-equipment-effectiveness"},
	{"общая эффективность оборудования", "overall-equipment-effectiveness"},
	{"mean time between failures", "mtbf"},
	{"mean time between failure", "mtbf"},
	{"средняя наработка на отказ", "mtbf"},
	{"mean time to repair", "mttr"},
	{"среднее время восстановления", "mttr"},
	{"rotating equipment", "rotating-equipment"},
	{"вращающееся оборудование", "rotating-equipment"},
	{"вращающегося оборудования", "rotating-equipment"},
	{"hydraulics", "hydraulics"}, {"hydraulic", "hydraulics"}, {"hydraulic systems", "hydraulics"},
	{"гидравлика", "hydraulics"}, {"гидравлических систем", "hydraulics"},
	{"pneumatics", "pneumatics"}, {"pneumatic", "pneumatics"}, {"pneumatic systems", "pneumatics"},
	{"пневматика", "pneumatics"}, {"пневматических систем", "pneumatics"},
	{"shutdown management", "shutdown-management"},
	{"plant shutdowns", "shutdown-management"},
	{"annual shutdowns", "shutdown-management"},
	{"turnaround management", "turnaround-management"},
	{"turnaround programs", "turnaround-management"},
	{"управление остановочными ремонтами", "turnaround-management"},
	{"heating ventilation and air conditioning", "hvac"},
	{"отопление вентиляция и кондиционирование", "hvac"},
	{"mechanical electrical and plumbing", "mep"},
	{"mechanical, electrical and plumbing", "mep"},
	{"mep design coordination", "mep"}, {"mep systems", "mep"},
	{"iso 17025", "iso-17025"}, {"iso/iec 17025", "iso-17025"},
	{"iso iec 17025", "iso-17025"},
	{"equipment calibration", "equipment-calibration"},
	{"калибровка оборудования", "equipment-calibration"},
	// Existing skills: long names and Russian surfaces, without new facet values.
	{"cmms", "cmms"},
	{"computerized maintenance management system", "cmms"},
	{"computerised maintenance management system", "cmms"},
	{"failure mode and effects analysis", "fmea"},
	{"failure modes and effects analysis", "fmea"},
	{"анализ видов и последствий отказов", "fmea"},
	{"анализ коренных причин", "root-cause-analysis"},
	{"анализ первопричин", "root-cause-analysis"},
}

// A site reliability engineering phrase names software operations rather than
// industrial equipment. Mask only that occurrence when reading the industrial skill.
var siteReliabilityEngineeringRE = regexp.MustCompile(`\bsite[-_\s]+reliability[-_\s]+engineering\b`)

var equipmentReliabilityEngineering = compilePhraseMatcher("reliability-engineering", "equipment reliability engineering")

var industrialMaintenanceCategories = map[string]bool{
	"industrial_engineering": true,
	"manufacturing":          true,
	"skilled_trades":         true,
	"operations":             true,
}

// Mask only API occurrences explicitly naming petroleum standards. A separate
// software API requirement in the same posting must remain visible to the parser.
var petroleumAPIRE = regexp.MustCompile(`(?i)\bapi(?:\s*[/,]\s*asme\b|\s+industry certifications?\b)`)
var petroleumInstituteRE = regexp.MustCompile(`(?i)(american petroleum institute\s*\(\s*)api\b`)

func maskIndustrialAPI(text string) string {
	text = stripMarkup(text)
	text = petroleumAPIRE.ReplaceAllStringFunc(text, func(match string) string {
		return strings.Repeat(" ", 3) + match[3:]
	})
	return petroleumInstituteRE.ReplaceAllString(text, "${1}   ")
}
