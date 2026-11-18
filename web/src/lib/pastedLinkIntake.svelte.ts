import { api } from '#lib/api.js';
import { isAuthenticated } from '#lib/auth.svelte.js';
import type { PastedJobLink } from '#lib/jobLink.js';
import { runLinkIntake, type LinkIntakeStep } from '#lib/linkIntake.js';

/** HeaderSearch's handling of a pasted vacancy link: look it up, and hand it in when the
 *  catalogue lacks it. `current` is the link the box holds now, which a slow answer is
 *  checked against; `open` goes to a posting. */
export class PastedLinkIntake {
  readonly #current: () => PastedJobLink | null;
  readonly #open: (slug: string) => void;

  busy = $state(false);
  // Where the last run stopped, when it stopped anywhere worth showing. Null while
  // nothing has been asked yet — the row then offers to ask.
  step = $state.raw<LinkIntakeStep | null>(null);

  /** Look this link up, and hand it in if we don't have it. See #lib/linkIntake for the
   *  order and why it is that way round. */
  activate = async (pasted: PastedJobLink) => {
    // One of our own posting pages: the slug is in the path, so asking the API whether
    // we carry a posting we are serving right now would be asking ourselves.
    if (pasted.ownSlug) {
      this.#open(pasted.ownSlug);
      return;
    }
    if (this.busy) return;
    this.busy = true;
    this.step = null;
    const step = await runLinkIntake(pasted.url, {
      find: (u) => api.findJobByUrl(u),
      submit: (u) => api.resolveJobLink(u),
      signedIn: isAuthenticated,
    });
    this.busy = false;
    // The box may have moved on while the intake was out: a paste over the old text, or
    // the text cleared entirely. The answer is about the URL we asked with, so applying
    // it now would navigate to a posting nobody is asking about any more — the same
    // stale-answer hazard the suggestion fetches guard with their AbortController, held
    // off here by comparing the question instead, because this runs from a click rather
    // than from an effect with a cleanup to hang the abort on.
    if (this.#current()?.url !== pasted.url) return;
    if (step.kind === 'open') {
      this.#open(step.slug);
      return;
    }
    this.step = step;
  };

  constructor(current: () => PastedJobLink | null, open: (slug: string) => void) {
    this.#current = current;
    this.#open = open;
  }
}
