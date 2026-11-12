// The freehire HTTP API contract. Go handlers implement what ogen generates from it
// (internal/api/apigen), so a handler that answers anything else does not compile.
// Routes move here one group at a time; the rest are still hand-written Fiber.
import "@typespec/http";
import "./common.tsp";
import "./email-prefs.tsp";
import "./talent.tsp";
import "./watchlist.tsp";
import "./lookups.tsp";
import "./me.tsp";
import "./plans.tsp";
import "./dictionary.tsp";
import "./jd.tsp";
import "./forms.tsp";
import "./feedback.tsp";
import "./answer-bank.tsp";
import "./moderation.tsp";
import "./contributions.tsp";
import "./alerts.tsp";
import "./company-feedback.tsp";
import "./company-relations.tsp";
import "./reports.tsp";
import "./timeline.tsp";
import "./profile.tsp";
import "./conversations.tsp";
import "./community.tsp";
import "./buddies.tsp";
import "./stats.tsp";
import "./submissions.tsp";
import "./tracking.tsp";
import "./experience.tsp";
import "./employer.tsp";
import "./referrals.tsp";
import "./billing.tsp";
import "./plan.tsp";
import "./direct-apply.tsp";
import "./jobs.tsp";
import "./companies.tsp";
import "./search.tsp";
import "./resume.tsp";
import "./cv.tsp";
import "./mentorship.tsp";
import "./mail.tsp";
import "./fit.tsp";
import "./autofill.tsp";
import "./search-intent.tsp";
import "./embeddings.tsp";
import "./support.tsp";
import "./assistant.tsp";
import "./account.tsp";

using Http;

@service(#{ title: "freehire API" })
@server("https://freehire.me/api/v1", "Production")
namespace Freehire;
