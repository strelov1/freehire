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

using Http;

@service(#{ title: "freehire API" })
@server("https://freehire.me/api/v1", "Production")
namespace Freehire;
