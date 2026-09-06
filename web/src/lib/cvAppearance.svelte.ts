// The personal defaults a NEW CV starts with — template, typography, page margins.
// Nothing here writes to any CV: saving only seeds what the next base CV is created
// with, and an existing CV keeps its own appearance.
//
// One store rather than per-page state because the Template and Typography tabs are two
// routes over ONE record: the API reads and writes all three fields together, so a pane
// that owned its own copy would refetch on every tab switch and drop whatever the other
// pane had edited but not yet saved. Held here, the edits survive the switch and one
// Save from either tab writes the whole record.
//
// A UserResource (see userResource.svelte.ts) because that is what makes it per-USER:
// the base registers the instance so the sign-out sweep drops it, which is the only
// thing standing between user A's template and user B signing in on the same tab.

import { api, ApiError } from '$lib/api';
import { UserResource } from '$lib/userResource.svelte';
import type { Margins, Style } from '$lib/generated/contracts';
import type { CvAppearanceDefaults, CvFont } from '$lib/cv';

const BLANK_MARGINS: Margins = { top: 0.5, right: 0.5, bottom: 0.5, left: 0.5 };

/** What one load fetches: the defaults themselves plus the font list the type controls
 *  offer. Two calls, one resource — a pane needs both before it can render anything. */
type Loaded = { defaults: CvAppearanceDefaults; fonts: CvFont[] };

class CvAppearanceStore extends UserResource<Loaded> {
  // Bound by the panes' controls, which is why these carry setters where this repo's
  // other stores expose getters and named mutators: `bind:` needs somewhere to write.
  #templateId = $state('');
  #style = $state<Style>({});
  #margins = $state<Margins>({ ...BLANK_MARGINS });
  #fonts = $state.raw<CvFont[]>([]);

  // The base treats a failed load as "leave the default state", which is right for a
  // read-mostly cache and wrong for a form: a pane showing blank defaults invites a
  // Save that would overwrite the real record with them. So the failure is recorded
  // here and the panes refuse to show controls until a load has actually succeeded.
  #loadFailed = $state(false);

  #saving = $state(false);
  #saveError = $state<string | null>(null);
  #saved = $state(false);
  #savedTimer: ReturnType<typeof setTimeout> | undefined;

  get templateId(): string {
    return this.#templateId;
  }
  set templateId(value: string) {
    this.#templateId = value;
  }

  get style(): Style {
    return this.#style;
  }
  set style(value: Style) {
    this.#style = value;
  }

  get margins(): Margins {
    return this.#margins;
  }
  set margins(value: Margins) {
    this.#margins = value;
  }

  get fonts(): CvFont[] {
    return this.#fonts;
  }
  get loadFailed(): boolean {
    return this.#loadFailed;
  }
  get saving(): boolean {
    return this.#saving;
  }
  get saveError(): string | null {
    return this.#saveError;
  }
  get saved(): boolean {
    return this.#saved;
  }

  protected async load(): Promise<Loaded> {
    this.#loadFailed = false;
    try {
      const [defaults, fonts] = await Promise.all([
        api.getCvAppearanceDefaults(),
        api.listCvFonts(),
      ]);
      return { defaults, fonts };
    } catch (e) {
      this.#loadFailed = true;
      throw e; // the base leaves `loaded` false, so the next visit retries.
    }
  }

  protected apply({ defaults, fonts }: Loaded) {
    this.#templateId = defaults.template_id;
    this.#style = defaults.style;
    this.#margins = defaults.margins;
    this.#fonts = fonts;
  }

  protected clearState() {
    this.#templateId = '';
    this.#style = {};
    this.#margins = { ...BLANK_MARGINS };
    this.#fonts = [];
    this.#loadFailed = false;
    this.clearSaveStatus();
  }

  /** Drops a "Saved."/error left by the other tab. Called when a pane mounts, so the
   *  note belongs to the pane the reader is looking at rather than to the section. */
  clearSaveStatus() {
    clearTimeout(this.#savedTimer);
    this.#saveError = null;
    this.#saved = false;
  }

  /** Writes all three fields, whichever tab asked. The record is one row; a pane that
   *  sent only its own slice would clear the other's. */
  async save(): Promise<void> {
    this.#saving = true;
    this.#saveError = null;
    this.#saved = false;
    try {
      const result = await api.setCvAppearanceDefaults({
        template_id: this.#templateId,
        style: this.#style,
        margins: this.#margins,
      });
      this.apply({ defaults: result, fonts: this.#fonts });
      this.markLoaded();
      this.#saved = true;
      clearTimeout(this.#savedTimer);
      this.#savedTimer = setTimeout(() => (this.#saved = false), 2000);
    } catch (e) {
      this.#saveError = e instanceof ApiError ? e.message : 'Could not save your appearance defaults.';
    } finally {
      this.#saving = false;
    }
  }
}

export const cvAppearance = new CvAppearanceStore();
