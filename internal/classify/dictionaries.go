package classify

// seniorityOrder lists seniority aliases in precedence order (most specific /
// highest rank first), so matchOrdered returns the stronger grade when a title
// carries several. seniorityAliases maps each alias to its enrich.SeniorityValues
// canonical. Aliases are lowercase; multi-word and hyphenated forms are explicit.
var seniorityOrder = []string{
	"head of", "chief", "cto", "cpo", "ceo", "vp", "vice president", "директор", "руководитель",
	"principal",
	"staff",
	"lead", "ведущий", "тимлид", "teamlead", "team lead",
	"senior", "sr", "sr.", "старший", "синьор", "сеньор",
	"middle", "mid", "mid-level", "mid level", "средний", "мидл",
	"junior", "jr", "jr.", "младший", "джуниор", "джун",
	"intern", "internship", "trainee", "стажёр", "стажер", "стажировка",
}

var seniorityAliases = map[string]string{
	"head of": "c_level", "chief": "c_level", "cto": "c_level", "cpo": "c_level",
	"ceo": "c_level", "vp": "c_level", "vice president": "c_level",
	"директор": "c_level", "руководитель": "c_level",
	"principal": "principal",
	"staff":     "staff",
	"lead":      "lead", "ведущий": "lead", "тимлид": "lead", "teamlead": "lead", "team lead": "lead",
	"senior": "senior", "sr": "senior", "sr.": "senior",
	"старший": "senior", "синьор": "senior", "сеньор": "senior",
	"middle": "middle", "mid": "middle", "mid-level": "middle", "mid level": "middle",
	"средний": "middle", "мидл": "middle",
	"junior": "junior", "jr": "junior", "jr.": "junior",
	"младший": "junior", "джуниор": "junior", "джун": "junior",
	"intern": "intern", "internship": "intern", "trainee": "intern",
	"стажёр": "intern", "стажер": "intern", "стажировка": "intern",
}

// categoryOrder lists category aliases in precedence order — multi-word and more
// specific terms first, so "data analyst" wins over a bare "data" and "fullstack"
// is not shadowed by "backend"/"frontend". categoryAliases maps each to its
// enrich.CategoryValues canonical.
var categoryOrder = []string{
	"full stack", "full-stack", "fullstack", "фулстек", "фуллстак",
	"data engineer", "data engineering", "дата-инженер", "инженер данных",
	"data scientist", "data science", "data scien", "дата-сайентист",
	"data analyst", "data analytics", "аналитик данных", "data аналитик",
	// Classic ML and explicitly ML-carrying combined forms first, so a mixed
	// "ML/AI Engineer" resolves to ml_ai before the bare AI terms below can claim it.
	"machine learning", "deep learning", "ml engineer", "ml/ai", "ai/ml",
	// AI-application terms (RAG/agents/LLM/prompt/applied AI) → ai_engineering.
	"generative ai", "genai", "llm engineer", "prompt engineer", "applied ai", "rag engineer", "ai engineer", "llm",
	"devops", "девопс", "platform engineer", "infrastructure engineer", "cloud engineer", "system administrator", "sysadmin",
	"sre", "site reliability",
	"network engineer", "network engineering", "network administrator", "сетевой инженер", "сетевой администратор",
	"backend", "back-end", "back end", "бэкенд", "бекенд",
	"frontend", "front-end", "front end", "фронтенд", "фронт",
	"mobile", "android", "ios", "мобильный", "мобильная", "мобильных",
	"qa", "quality assurance", "tester", "test engineer", "test automation", "sdet", "тестировщик", "тестирование",
	"security", "infosec", "appsec", "cybersecurity", "cyber security", "безопасность", "безопасности", "кибербезопасность",
	"embedded", "firmware", "встраиваемые", "встраиваемых",
	"blockchain", "блокчейн",
	"hardware", "fpga",
	"solutions architect", "software architect", "enterprise architect", "cloud architect", "architect", "архитектор",
	"designer", "design", "ux", "ui", "дизайнер", "дизайн",
	"product manager", "product owner", "продакт", "продукт-менеджер",
	"project manager", "delivery manager", "program manager", "programme manager", "scrum master", "scrum-master", "проджект", "проект-менеджер", "скрам-мастер", "скрам мастер",
	"engineering manager", "team manager",
	"marketing", "smm", "маркетолог", "маркетинг",
	"seo", "search engine optimization", "social media", "контент-маркетолог", "copywriter", "content writer", "brand manager", "public relations",
	"sales", "account executive", "business development", "account manager", "sdr", "bdr", "продажи", "продаж", "продажам",
	"support", "customer success", "customer service", "help desk", "поддержка", "поддержки", "техподдержка", "техподдержки",
	// Bare "manager" resolves last so a functional prefix wins ("Sales Manager"
	// → sales, "Support Manager" → support); a manager title with no function
	// ("Operations Manager") falls through to management.
	"manager", "менеджер",
	"analyst", "аналитик",
}

var categoryAliases = map[string]string{
	"full stack": "fullstack", "full-stack": "fullstack", "fullstack": "fullstack",
	"фулстек": "fullstack", "фуллстак": "fullstack",
	"data engineer": "data_engineering", "data engineering": "data_engineering",
	"дата-инженер": "data_engineering", "инженер данных": "data_engineering",
	"data scientist": "data_science", "data science": "data_science",
	"data scien": "data_science", "дата-сайентист": "data_science",
	"data analyst": "data_analytics", "data analytics": "data_analytics",
	"аналитик данных": "data_analytics", "data аналитик": "data_analytics",
	"machine learning": "ml_ai", "deep learning": "ml_ai", "ml engineer": "ml_ai",
	"ml/ai": "ml_ai", "ai/ml": "ml_ai",
	"ai engineer": "ai_engineering", "generative ai": "ai_engineering", "genai": "ai_engineering",
	"llm engineer": "ai_engineering", "prompt engineer": "ai_engineering", "applied ai": "ai_engineering",
	"rag engineer": "ai_engineering", "llm": "ai_engineering",
	"devops": "devops", "девопс": "devops",
	"sre": "sre", "site reliability": "sre",
	"network engineer": "network_engineering", "network engineering": "network_engineering",
	"network administrator": "network_engineering",
	"сетевой инженер":       "network_engineering", "сетевой администратор": "network_engineering",
	"backend": "backend", "back-end": "backend", "back end": "backend",
	"бэкенд": "backend", "бекенд": "backend",
	"frontend": "frontend", "front-end": "frontend", "front end": "frontend",
	"фронтенд": "frontend", "фронт": "frontend",
	"mobile": "mobile", "android": "mobile", "ios": "mobile",
	"мобильный": "mobile", "мобильная": "mobile", "мобильных": "mobile",
	"qa": "qa", "quality assurance": "qa", "tester": "qa", "test engineer": "qa",
	"тестировщик": "qa", "тестирование": "qa",
	"security": "security", "infosec": "security", "appsec": "security",
	"cybersecurity": "security", "cyber security": "security",
	"безопасность": "security", "безопасности": "security", "кибербезопасность": "security",
	"embedded": "embedded", "firmware": "embedded", "встраиваемые": "embedded", "встраиваемых": "embedded",
	"blockchain": "blockchain", "блокчейн": "blockchain",
	"hardware": "hardware", "fpga": "hardware",
	"solutions architect": "architecture", "software architect": "architecture",
	"enterprise architect": "architecture", "cloud architect": "architecture",
	"architect": "architecture", "архитектор": "architecture",
	"designer": "design", "design": "design", "ux": "design", "ui": "design",
	"дизайнер": "design", "дизайн": "design",
	"product manager": "product", "product owner": "product",
	"продакт": "product", "продукт-менеджер": "product",
	"project manager": "project_management", "delivery manager": "project_management",
	"program manager": "project_management", "programme manager": "project_management",
	"scrum master": "project_management", "scrum-master": "project_management",
	"проджект": "project_management", "проект-менеджер": "project_management",
	"скрам-мастер": "project_management", "скрам мастер": "project_management",
	"engineering manager": "management", "team manager": "management",
	"manager": "management", "менеджер": "management",
	"marketing": "marketing", "smm": "marketing", "маркетолог": "marketing", "маркетинг": "marketing",
	"seo": "marketing", "search engine optimization": "marketing", "social media": "marketing",
	"контент-маркетолог": "marketing",
	"sales":              "sales", "account executive": "sales",
	"продажи": "sales", "продаж": "sales", "продажам": "sales",
	"support": "support", "поддержка": "support", "поддержки": "support",
	"техподдержка": "support", "техподдержки": "support",
	"analyst": "data_analytics", "аналитик": "data_analytics",
	// role-alias expansion (Lightcast/title diff). Non-tech titles feed the enrichment
	// gate from the title (higher-precision than the description tier); the multi-word
	// "* manager" forms are ordered before bare "manager" so they win their function.
	"business development": "sales", "account manager": "sales", "sdr": "sales", "bdr": "sales",
	"customer success": "support", "customer service": "support", "help desk": "support",
	"copywriter": "marketing", "content writer": "marketing", "brand manager": "marketing", "public relations": "marketing",
	"platform engineer": "devops", "infrastructure engineer": "devops", "cloud engineer": "devops",
	"system administrator": "devops", "sysadmin": "devops",
	"sdet": "qa", "test automation": "qa",
}
