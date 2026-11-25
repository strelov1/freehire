import "@typespec/http";
import "@typespec/openapi";

using Http;
using TypeSpec.OpenAPI;

namespace Freehire;

// The payloads a verified employer sends to publish and edit its own vacancies (employer.tsp
// holds the routes that take them).

/** `type` is "text", "boolean" or "select"; `options` are a select's choices. */
model EmployerQuestion {
  label?: string;
  type?: string;
  required?: boolean;
  options?: string[];
}

/** `title` is required, and `url` too unless `apply_mode` is "freehire". */
model EmployerVacancyCreate {
  apply_mode?: string;
  questions?: EmployerQuestion[];
  url?: string;
  title?: string;
  location?: string;
  remote?: boolean;
  description?: string;
  posted_at?: utcDateTime | null;
  skills?: string[];
  regions?: string[];
  cities?: string[];
  work_mode?: string;
  employment_type?: string;
  seniority?: string;
  salary_min?: int64 | null;
  salary_max?: int64 | null;
  salary_currency?: string;
  salary_period?: string;
}

/** An absent or null field is kept. */
model EmployerVacancyPatch {
  apply_mode?: string | null;
  questions?: EmployerQuestion[] | null;
  title?: string | null;
  location?: string | null;
  remote?: boolean | null;
  description?: string | null;
  posted_at?: utcDateTime | null;
}
