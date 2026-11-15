import { goto } from '$app/navigation';
import { api } from '#lib/api.js';
import { isAuthenticated } from '#lib/auth.svelte.js';
import { askConfirmTailor } from '#lib/confirmTailorDialog.svelte.js';
import { promptSignIn } from '#lib/signin.js';
import { tailorHref } from '#lib/tailorHref.js';
import type { Job, MatchAnalysisResponse } from '#lib/types.js';

/** The job page's `Tailor my CV`: the cached fit analysis it needs, whether the reader has
 *  a CV to tailor, and the confirm-then-navigate start. Constructed during component
 *  initialisation, since it registers an effect. */
export class JobTailor {
  readonly #job: () => Job;

  // The cached fit analysis, read ONCE for the whole page: the CTA row's `Tailor my CV`
  // button needs it to know whether the reader has a CV at all, and the sidebar's
  // MatchSummary needs the analysis itself. It used to be MatchSummary's own fetch; moving
  // the button up must not buy a second call to the same endpoint, so the read moved with
  // it and the response goes down as a prop.
  //
  // Guarded on the slug the way MatchSummary guarded it, so a slow response for a previous
  // posting cannot overwrite the current one. Never read for a guest: the endpoint is
  // authenticated, and a rejected request per visit from one address is what got real users
  // fail2banned once already.
  analysis = $state.raw<MatchAnalysisResponse | null>(null);

  // Withheld only once the page KNOWS there is no CV. Unknown — a guest, or a read still in
  // flight — still offers the button: the offer is what brings a guest to sign in, and
  // withholding the page's primary CTA until a fetch resolves makes the row jump.
  readonly canTailor = $derived(this.analysis?.has_cv !== false);

  // Tailoring navigates away; the confirmation dialog owns its own fetch of the
  // deterministic skill/requirement check, so this only needs to await the yes/no. The
  // dialog also states what the session costs and what today has left — which is why
  // nothing beside the button does.
  busy = $state(false);

  start = async () => {
    if (!isAuthenticated()) {
      promptSignIn();
      return;
    }
    if (this.busy) return;
    this.busy = true;
    const job = this.#job();
    try {
      const choice = await askConfirmTailor(job.public_slug, `${job.title} at ${job.company}`);
      // eslint-disable-next-line svelte/no-navigation-without-resolve -- tailorHref() wraps resolve('/tailor/[slug]'); the rule can't see through the appended ?base= query
      if (choice) await goto(tailorHref(job.public_slug, choice.baseCvId));
    } finally {
      // Released even on the happy path, where the navigation has already left this page:
      // a rejected `goto` would otherwise leave all THREE copies of the page's primary CTA
      // permanently disabled with no way back. It mattered less when this was one sidebar
      // button; it is the whole call to action now.
      this.busy = false;
    }
  };

  constructor(job: () => Job) {
    this.#job = job;

    $effect(() => {
      const forSlug = this.#job().public_slug;
      this.analysis = null;
      if (!isAuthenticated()) return;
      api
        .getMatchAnalysis(forSlug)
        .then((d) => {
          if (forSlug === this.#job().public_slug) this.analysis = d;
        })
        .catch(() => {});
    });
  }
}
