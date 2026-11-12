import "@typespec/http";

using Http;

namespace Freehire;

/**
 * The caller's canonical form fields, all empty bar the account email when no source
 * states a contact. Keys are the profile's own and the set grows with it.
 */
@tag("Autofill")
@route("/me/autofill-profile")
@get
op getAutofillProfile(): Data<unknown> | Problem;

/**
 * Fills the form in the caller's own browser through the browser-tool channel. Paid AI
 * route (unmetered). Only the extension's own Bearer session is admitted; anything else
 * is a 403. The report names what was filled, deferred and left unmapped.
 */
@tag("Autofill")
@route("/me/autofill/run")
@post
op runAutofill(): Data<unknown> | Problem;
