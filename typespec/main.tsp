// The freehire HTTP API contract. Go handlers implement what ogen generates from it
// (internal/api/apigen), so a handler that answers anything else does not compile.
// Routes move here one group at a time; the rest are still hand-written Fiber.
import "@typespec/http";
import "./common.tsp";
import "./email-prefs.tsp";

using Http;

@service(#{ title: "freehire API" })
@server("https://freehire.me/api/v1", "Production")
namespace Freehire;
