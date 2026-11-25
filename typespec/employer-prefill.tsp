import "@typespec/http";
import "@typespec/openapi";

using Http;
using TypeSpec.OpenAPI;

namespace Freehire;

/** Reads a vacancy link into draft values for the publish form; writes nothing. A vacancy already in the catalogue returns its stored fields. */
@tag("Employer")
@opExample(#{ parameters: #{ body: #{ url: "https://example.com/jobs/123" } } })
@route("/employer/jobs/prefill")
@post
op prefillEmployerVacancy(@body body: PrefillRequest): Data<SubmissionPrefill> | Problem;
