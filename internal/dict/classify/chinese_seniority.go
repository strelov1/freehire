package classify

// chineseSeniorityTable lists the grade words of a Chinese title, highest first.
// Traditional 高級 is deliberately absent: Taiwanese titles use it for luxury as often
// as for grade (高級飯店房務員 cleans the rooms of a luxury hotel), while mokahr's
// Simplified 高级 is the grade prefix of a corporate ladder.
var chineseSeniorityTable = []aliasEntry{
	{"首席", "principal"},
	{"資深", "senior"},
	{"资深", "senior"},
	{"高级", "senior"},
	{"中級", "middle"},
	{"中级", "middle"},
	{"初級", "junior"},
	{"初级", "junior"},
	{"初階", "junior"},
	{"初阶", "junior"},
	{"實習", "intern"},
	{"实习", "intern"},
	{"應屆", "junior"},
	{"应届", "junior"},
	{"管培生", "junior"},
	{"管理培训生", "junior"},
}
