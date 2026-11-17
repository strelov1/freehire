// The workspace's PDF download. The file is fetched here rather than opened by the browser:
// only then is there a response to tell a delivered PDF from a failed one, and only a delivered
// one is an export (see outcome.ts).
export class PdfExport {
  readonly #url: () => string;
  readonly #flush: () => Promise<void>;
  readonly #onExported: () => void;
  readonly #hasGaps: () => boolean;

  busy = $state(false);
  error = $state('');
  /** The stop before a PDF that still has template slots in it. */
  confirmOpen = $state(false);

  constructor(deps: { url: () => string; flush: () => Promise<void>; onExported: () => void; hasGaps: () => boolean }) {
    this.#url = deps.url;
    this.#flush = deps.flush;
    this.#onExported = deps.onExported;
    this.#hasGaps = deps.hasGaps;
  }

  /** The Download link's click: a CV with gaps stops at the confirmation, anything else is
   *  fetched here. A modified click (new tab, save as) is left to the browser. */
  guard = (e: MouseEvent) => {
    const gaps = this.#hasGaps();
    if (!gaps && (e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey)) return;
    e.preventDefault();
    if (gaps) this.confirmOpen = true;
    else void this.run();
  };

  run = async () => {
    if (this.busy) return;
    this.busy = true;
    this.error = '';
    try {
      // The server renders the saved CV, so an edit still on the autosave timer goes first.
      await this.#flush();
      const res = await fetch(this.#url());
      const blob = await res.blob();
      // A 200 can still be a sign-in page the session expiry redirected to: it must be a PDF.
      if (!res.ok || !res.headers.get('content-type')?.includes('application/pdf') || blob.size === 0) {
        throw new Error(`pdf ${res.status}`);
      }
      const href = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = href;
      link.download = 'cv.pdf';
      link.click();
      setTimeout(() => URL.revokeObjectURL(href), 1000);
      this.#onExported();
    } catch {
      this.error = 'Could not download the PDF. Please try again.';
    } finally {
      this.busy = false;
    }
  };
}
