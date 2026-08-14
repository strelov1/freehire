/**
 * Shapes for the in-extension transport: `RuntimeMessage` over chrome.runtime
 * (panel <-> background <-> content), discriminated by `kind`. The chat itself
 * talks to hire's assistant over HTTP (see `lib/assistant/`), not through here.
 */

/** A read of whatever page the user is currently looking at. */
export interface PageSnapshot {
  url: string;
  title: string;
  /** Best-effort primary heading of the page (e.g. a job title). */
  headline: string;
  /** Visible text, trimmed and length-capped. */
  text: string;
}

export type FieldTag = 'input' | 'select' | 'textarea';

/**
 * A serialisable view of one question the page asks (see lib/form.ts). Usually
 * one control, but a question rendered as several checkboxes under a shared
 * `legend` is one field offering their labels as `options` — see `Question`.
 */
export interface FormField {
  /** Position in the page's question list — reported, not used to address it. */
  index: number;
  /**
   * The `<form>` this question answers within, as an index into the frame's
   * forms, or -1 when it stands outside one (Ashby renders its application that
   * way). Pairs with `Upload.form` to tell an application from a signup sharing
   * the page.
   */
  form: number;
  tag: FieldTag;
  type: string;
  /** The question's text: a control's label, or a group's legend. */
  label: string;
  name: string;
  required: boolean;
  /** The current answer; for a group, the options already chosen. */
  value: string;
  /** True for a custom-widget combobox (react-select and friends), which the
   *  simple filler must not write into — see `fillByLabel`. */
  combo: boolean;
  /**
   * The answers on offer: a native `<select>`'s options, a group's option
   * labels, or a combobox's options where it exposes them while closed. A
   * react-select renders its listbox only once opened, so it carries none here
   * and the agent reads them with `combobox.open` + `combobox.options`.
   */
  options?: string[];
}

/**
 * A resume/CV upload the page offers. Never a fill target — a file input's value
 * cannot be set from script — but it is what marks a form as an application
 * rather than a job-alert signup. See `extractUploads`.
 */
export interface Upload {
  /** The `<form>` that owns it, indexed as `FormField.form` is. */
  form: number;
}

/** A control plus the tab frame it lives in; 0 is the top document. */
export interface FramedField extends FormField {
  frame: number;
}

/** An upload plus the tab frame it lives in. */
export interface FramedUpload extends Upload {
  frame: number;
}

/**
 * A value to write into the question carrying `label` (see `fillByLabel`). For a
 * grouped question the value is one of the options it offers, not free text.
 *
 * `frame`/`form` scope the write to the question the fill was planned from —
 * `deterministicAutofill` sets both, carried over from the `FramedField` that
 * justified the fill, so a same-labeled control outside that frame or form is
 * left alone. Undefined for a `fill_simple` tool call, whose label the agent
 * names on its own with no field to read them from; such a fill still matches
 * the first question carrying the label, as before frame/form scoping existed.
 */
export interface LabelFill {
  label: string;
  value: string;
  frame?: number;
  form?: number;
}

/**
 * What became of one `LabelFill`. Every requested fill gets an outcome, so a
 * caller never has to infer failure from a silent absence.
 */
export type FillStatus =
  /** Written, and native input/change dispatched. */
  | 'filled'
  /** No control on the page carries that label. */
  | 'not_found'
  /** A custom-widget combobox — deliberately left alone (deferred capability). */
  | 'deferred_combobox'
  /** No option matching the value: a native <select>'s, or a group's. */
  | 'no_option';

export interface FillOutcome {
  label: string;
  status: FillStatus;
}

/**
 * One step of driving a custom-widget combobox. The wire exposes four separate
 * primitives (`combobox.open` / `.options` / `.select` / `.verify`) because the
 * agent composes them one at a time; inside the extension they travel as one
 * message discriminated by `action`, so addressing a widget across the tab's
 * frames is written once rather than four near-identical times.
 *
 * `value` is the option in play: the one to commit for `select`, the one whose
 * commit is being confirmed for `verify`, and empty for the two that only read.
 */
export interface ComboboxStep {
  action: 'open' | 'options' | 'select' | 'verify';
  label: string;
  value: string;
}

/**
 * What a step reports back. `status` is the step's own vocabulary (see
 * `lib/combobox.ts`); the extra fields travel only for the steps that produce
 * them — `options` for a read, `committed` for a verification.
 */
export interface ComboboxReply {
  status: string;
  options?: string[];
  committed?: string;
}

/** Messages passed inside the extension via chrome.runtime. */
export type RuntimeMessage =
  | { kind: 'GET_PAGE_SNAPSHOT' }
  | { kind: 'PAGE_SNAPSHOT'; snapshot: PageSnapshot }
  | { kind: 'FORM'; fields: FormField[]; uploads: Upload[] }
  // Reading a form and filling it both fan out across the tab's frames in the
  // background relay: an apply form is routinely served from an ATS iframe, and
  // a page carrying any other iframe (a map, an ad) would otherwise be answered
  // by whichever frame replied first — in practice the empty one.
  | { kind: 'GET_FRAMED_FORM' }
  | { kind: 'FRAMED_FORM'; fields: FramedField[]; uploads: FramedUpload[] }
  | { kind: 'FILL_BY_LABEL'; fills: LabelFill[] }
  | { kind: 'FILL_OUTCOMES'; outcomes: FillOutcome[] }
  // Driving a custom-widget combobox: one step, offered to every frame, since
  // only the frame holding the widget can answer for it.
  | { kind: 'COMBOBOX_STEP'; step: ComboboxStep }
  | { kind: 'COMBOBOX_REPLY'; reply: ComboboxReply };

/** An empty snapshot, used when no active tab can be read. */
export function emptySnapshot(): PageSnapshot {
  return { url: '', title: '', headline: '', text: '' };
}
