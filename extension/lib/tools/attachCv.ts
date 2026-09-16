/**
 * Attaching an already-tailored CV to a page's upload field, via the Chrome
 * debugging protocol — the only script-reachable path onto `input[type=file]`
 * (see openspec/changes/extension-attach-tailored-cv/design.md). Pure pieces
 * live here; the actual `chrome.debugger`/`chrome.downloads` calls are thin
 * glue in background.ts, per extension/AGENTS.md's "test the logic, not the
 * transport".
 */

import type { FramedUpload } from '../protocol';

/**
 * The JS source CDP's `Runtime.evaluate` runs, inside the upload's own frame, to
 * resolve the exact `<input type="file">` `extractUploads` found — scoped by the
 * same `form` index `FramedUpload.form` already carries (-1 for a question
 * standing outside any `<form>`, as Ashby renders it), so this names "which
 * control" the same way the rest of the wire's `form` addressing does.
 *
 * Filters out a disabled or hidden `input[type="file"]`, mirroring
 * `extractUploads`'s own filter (`lib/form.ts`) as literal JS source — this string is
 * evaluated in the PAGE's execution context via CDP, so it cannot import or call that
 * function directly. Matching it matters: `extractUploads` decided a single reachable
 * upload exists, and an unfiltered `querySelector` here could instead land on an
 * earlier hidden/disabled file input a widget keeps in the DOM for its own reasons,
 * writing the CV into a node nobody detected while the real field stays empty.
 *
 * Evaluates to `null` rather than throwing when the numbered form does not exist on
 * the page any more, or no undisabled/visible file input remains inside it — the
 * caller reports that as a failure, not this expression.
 */
export function uploadInputExpression(form: number): string {
  const scope = form >= 0 ? `document.querySelectorAll('form')[${form}]` : 'document';
  const isHidden = `(el) => (el.hidden || el.closest('[hidden]')) || ` +
    `(typeof el.checkVisibility === 'function' && !el.checkVisibility())`;
  return (
    `(() => { const s = ${scope}; if (!s) return null; ` +
    `const isHidden = ${isHidden}; ` +
    `return Array.from(s.querySelectorAll('input[type="file"]'))` +
    `.find((el) => !el.disabled && !isHidden(el)) ?? null; })()`
  );
}

/** Base64-encodes bytes, chunked so `String.fromCharCode` never spreads a huge array at
 *  once — a rendered CV is small, but there is no reason to risk the call-stack limit. */
export function base64FromArrayBuffer(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer);
  const chunkSize = 0x8000;
  let binary = '';
  for (let i = 0; i < bytes.length; i += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(i, i + chunkSize));
  }
  return btoa(binary);
}

/** A PDF's bytes as a `data:` URL — `chrome.downloads.download` takes a URL, not bytes. */
export function pdfDataUrl(buffer: ArrayBuffer): string {
  return `data:application/pdf;base64,${base64FromArrayBuffer(buffer)}`;
}

/**
 * The slice of `chrome.downloads` this module needs, so `resolveDownloadedPath` is testable
 * with a fake rather than a real download. The real implementation (`realDownloadsAPI` in
 * `entrypoints/background.ts`) is thin glue over `chrome.downloads` itself, per
 * extension/AGENTS.md's "test the logic, not the transport".
 */
export interface DownloadsAPI {
  download(options: { url: string; filename: string; conflictAction: 'uniquify' }): Promise<number>;
  search(query: { id: number }): Promise<Array<{ state?: string; filename?: string }>>;
  onChanged: {
    addListener(cb: (delta: { id: number; state?: { current?: string } }) => void): void;
    removeListener(cb: (delta: { id: number; state?: { current?: string } }) => void): void;
  };
}

/**
 * Starts the download and resolves with the absolute path Chrome saved it to, once it
 * completes — `DOM.setFileInputFiles` needs a real filesystem path, not the bytes this
 * module started from. Bounded by `timeoutMs`, because "ask where to save each file" turns
 * a silent completion into a native Save dialog this code cannot see (design.md, Risks).
 */
export async function resolveDownloadedPath(
  api: DownloadsAPI,
  dataUrl: string,
  filename: string,
  timeoutMs: number,
): Promise<string> {
  return new Promise<string>((resolve, reject) => {
    // The listener attaches before `download()` is even called, and buffers whatever
    // arrives before the id is known — a `data:` URL this small can complete before the
    // `download()` promise itself resolves, and a delta missed here has no second chance:
    // chrome.downloads.onChanged reports a transition once, not a current-state snapshot.
    let settled = false;
    let id: number | null = null;
    const buffered: Array<{ id: number; state?: { current?: string } }> = [];

    const finish = (fn: () => void) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      api.onChanged.removeListener(onChanged);
      fn();
    };

    const timer = setTimeout(
      () =>
        finish(() =>
          reject(new Error('timed out waiting for the download to finish — check your Downloads prompt')),
        ),
      timeoutMs,
    );

    function handle(delta: { id: number; state?: { current?: string } }) {
      if (delta.state?.current === 'complete') {
        finish(() => {
          api.search({ id: delta.id }).then((items) => {
            const item = items[0];
            if (item?.filename) resolve(item.filename);
            else reject(new Error('download completed but reported no file path'));
          }, reject);
        });
      } else if (delta.state?.current === 'interrupted') {
        finish(() => reject(new Error('the download was interrupted')));
      }
    }

    function onChanged(delta: { id: number; state?: { current?: string } }) {
      if (id === null) {
        buffered.push(delta);
        return;
      }
      if (delta.id === id) handle(delta);
    }

    api.onChanged.addListener(onChanged);

    api.download({ url: dataUrl, filename, conflictAction: 'uniquify' }).then(
      (downloadId) => {
        if (settled) return;
        id = downloadId;
        const match = buffered.find((d) => d.id === id);
        if (match) handle(match);
      },
      (err) => finish(() => reject(err)),
    );
  });
}

/** Every Chromium tab's top document — the only frame CDP addressing reaches in this
 *  change (see design.md's cross-origin-iframe risk). Mirrors background.ts's own
 *  `TOP_FRAME`, which the frame-fan-out plumbing already treats the same way. */
export const TOP_FRAME = 0;

/** What `pickAttachableUpload` found, in decreasing order of usability. */
export type UploadPick =
  | { kind: 'found'; upload: FramedUpload }
  /** No upload field anywhere on the page. */
  | { kind: 'none' }
  /**
   * An upload exists, but only inside a frame other than the top one — a cross-origin
   * embedded ATS iframe (the "site-with-iframe" Greenhouse variant) runs in a different
   * render process, and this change's `Runtime.evaluate` call — unscoped, so it runs in
   * the top frame's default execution context — cannot reach a node in a different one.
   */
  | { kind: 'unreachable' }
  /** More than one reachable upload (e.g. résumé + cover letter) — nothing in `Upload`
   *  carries a label to tell them apart, so guessing the first one risks writing the
   *  candidate's CV into a cover-letter field silently. */
  | { kind: 'ambiguous'; count: number };

/**
 * Picks the one upload field this action can actually write into, or says why it can't —
 * refusing rather than guessing, the same rule `fillByLabel`'s `ambiguous`/`wrong_form`
 * outcomes already follow for filling a labelled question (extension/AGENTS.md).
 */
export function pickAttachableUpload(uploads: FramedUpload[]): UploadPick {
  const reachable = uploads.filter((u) => u.frame === TOP_FRAME);
  if (reachable.length === 0) return uploads.length === 0 ? { kind: 'none' } : { kind: 'unreachable' };
  if (reachable.length > 1) return { kind: 'ambiguous', count: reachable.length };
  return { kind: 'found', upload: reachable[0] as FramedUpload };
}

/**
 * Turns `chrome.debugger.attach`'s raw error text into something the person can act on.
 * Chrome allows only one debugger client per tab, so a tab already open in real DevTools
 * (or debugged by another extension) fails `attach` before any banner ever appears —
 * that is the one case worth naming; anything else passes through unchanged.
 */
export function classifyAttachError(message: string): string {
  if (/already attached/i.test(message)) {
    return 'Close DevTools (or any other extension debugging this tab) and try again.';
  }
  return message;
}
