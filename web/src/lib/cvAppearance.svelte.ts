// The personal defaults a NEW CV starts with — template, typography, page margins.
// Nothing here writes to any CV: saving only seeds what the next base CV is created
// with, and an existing CV keeps its own appearance.
//
// A module rather than per-page state because the Template and Typography tabs are two
// routes over ONE record: the API reads and writes all three fields together, so a pane
// that owned its own copy would re-fetch on every tab switch and drop whatever the other
// pane had edited but not yet saved. Held here, the edits survive the switch and one
// Save from either tab writes the whole record.

import { api, ApiError } from './api';
import type { Margins, Style } from './generated/contracts';
import type { CvFont } from './cv';

let status = $state<'loading' | 'ready' | 'error'>('loading');
let loadError = $state<string | null>(null);
let fonts = $state.raw<CvFont[]>([]);

let templateId = $state('');
let style = $state<Style>({});
let margins = $state<Margins>({ top: 0.5, right: 0.5, bottom: 0.5, left: 0.5 });

let saving = $state(false);
let saveError = $state<string | null>(null);
let saved = $state(false);
let savedTimer: ReturnType<typeof setTimeout> | undefined;

let loaded = false;

export const cvAppearance = {
  get status() {
    return status;
  },
  get loadError() {
    return loadError;
  },
  get fonts() {
    return fonts;
  },
  get saving() {
    return saving;
  },
  get saveError() {
    return saveError;
  },
  get saved() {
    return saved;
  },
  // Written by the panes' controls (`bind:`), which is why these carry setters.
  get templateId() {
    return templateId;
  },
  set templateId(value: string) {
    templateId = value;
  },
  get style() {
    return style;
  },
  set style(value: Style) {
    style = value;
  },
  get margins() {
    return margins;
  },
  set margins(value: Margins) {
    margins = value;
  },
};

/** Reads the defaults once per session. Both panes call it on mount; the second call is
 *  a no-op, so switching tabs neither refetches nor discards an unsaved edit. */
export async function ensureCvAppearanceLoaded(): Promise<void> {
  if (loaded) return;
  loaded = true;
  status = 'loading';
  try {
    const [defaults, fontList] = await Promise.all([
      api.getCvAppearanceDefaults(),
      api.listCvFonts(),
    ]);
    templateId = defaults.template_id;
    style = defaults.style;
    margins = defaults.margins;
    fonts = fontList;
    status = 'ready';
  } catch (e) {
    // Let the next visit try again rather than leaving the section permanently broken.
    loaded = false;
    loadError = e instanceof ApiError ? e.message : 'Could not load your appearance defaults.';
    status = 'error';
  }
}

/** Writes all three fields, whichever tab asked. The record is one row; a pane that sent
 *  only its own slice would clear the other's. */
export async function saveCvAppearance(): Promise<void> {
  saving = true;
  saveError = null;
  saved = false;
  try {
    const result = await api.setCvAppearanceDefaults({ template_id: templateId, style, margins });
    templateId = result.template_id;
    style = result.style;
    margins = result.margins;
    saved = true;
    clearTimeout(savedTimer);
    savedTimer = setTimeout(() => (saved = false), 2000);
  } catch (e) {
    saveError = e instanceof ApiError ? e.message : 'Could not save your appearance defaults.';
  } finally {
    saving = false;
  }
}
