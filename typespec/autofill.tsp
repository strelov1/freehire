import "@typespec/http";
import "@typespec/openapi";

using Http;
using TypeSpec.OpenAPI;

namespace Freehire;

// The caller's canonical form fields, all empty bar the account email when no source
// states a contact. Keys are the profile's own and the set grows with it.
/**
 * Projected from your CV header, with the account email as a fallback — what the
 * extension writes into application forms.
 */
@summary("Your contact details in canonical autofill fields.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #{
      first_name: "Ilya",
      last_name: "Strelov",
      email: "you@example.com",
      phone: "+1…",
      linkedin: "https://linkedin.com/in/you",
      portfolio: "https://ilya.dev",
    },
  },
})
@tag("Autofill")
@route("/me/autofill-profile")
@get
op getAutofillProfile(): Data<unknown> | Problem;

// Fills the form in the caller's own browser through the browser-tool channel. Paid AI
// route (unmetered). Only the extension's own Bearer session is admitted; anything else
// is a 403. The report names what was filled, deferred and left unmapped.
/**
 * Runs the autofill agent over the browser-tool wire — your extension on one end, the
 * agent on the other, routed strictly within your own channel. Requires the extension to
 * be connected, and authenticates only as the extension itself — a session cookie or an
 * API key gets a 403, since this call writes into whatever form your browser has open,
 * not just reads it.
 */
@summary("Drive your own browser to fill an application form.")
@useAuth(SessionOrKey)
@extension("x-audience", "extension")
@opExample(#{
  returnType: #{
    data: #{
      filled: 11,
      skipped: 2,
    },
  },
})
@tag("Autofill")
@route("/me/autofill/run")
@post
op runAutofill(): Data<unknown> | Problem;
