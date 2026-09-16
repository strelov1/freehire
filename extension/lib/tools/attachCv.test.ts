import { describe, it, expect, vi } from 'vitest';
import {
  uploadInputExpression,
  base64FromArrayBuffer,
  pdfDataUrl,
  resolveDownloadedPath,
  pickAttachableUpload,
  classifyAttachError,
  TOP_FRAME,
  type DownloadsAPI,
} from './attachCv';
import type { FramedUpload } from '../protocol';

describe('uploadInputExpression', () => {
  it('scopes to the numbered form when the upload sits inside one', () => {
    expect(uploadInputExpression(2)).toBe(
      "(document.querySelectorAll('form')[2])?.querySelector('input[type=\"file\"]') ?? null",
    );
  });

  it('scopes to the document when the upload stands outside any form (form -1, as Ashby renders it)', () => {
    expect(uploadInputExpression(-1)).toBe("(document)?.querySelector('input[type=\"file\"]') ?? null");
  });

  it('scopes to form index 0 distinctly from the no-form case', () => {
    expect(uploadInputExpression(0)).toBe(
      "(document.querySelectorAll('form')[0])?.querySelector('input[type=\"file\"]') ?? null",
    );
  });
});

describe('base64FromArrayBuffer / pdfDataUrl', () => {
  function bytesOf(s: string): ArrayBuffer {
    return Uint8Array.from(s, (c) => c.charCodeAt(0)).buffer;
  }

  it('encodes bytes as base64', () => {
    expect(base64FromArrayBuffer(bytesOf('hi'))).toBe('aGk=');
  });

  it('wraps the base64 as a PDF data URL', () => {
    expect(pdfDataUrl(bytesOf('hi'))).toBe('data:application/pdf;base64,aGk=');
  });
});

describe('resolveDownloadedPath', () => {
  type Delta = { id: number; state?: { current?: string } };

  function fakeDownloadsAPI(id = 1): DownloadsAPI & { emit: (delta: Delta) => void } {
    let listener: ((delta: Delta) => void) | null = null;
    return {
      download: vi.fn(async () => id),
      search: vi.fn(async () => [{ state: 'in_progress' }]),
      onChanged: {
        addListener: vi.fn((cb: (delta: Delta) => void) => {
          listener = cb;
        }),
        removeListener: vi.fn(() => {
          listener = null;
        }),
      },
      emit(delta: Delta) {
        listener?.(delta);
      },
    };
  }

  it('resolves via an onChanged event buffered before the download id was known', () => {
    // `download()` is an async mock, so its `.then` is deferred — this `emit` lands while
    // `id` is still null, exactly the race a `data:` URL small enough to finish instantly
    // can trigger in the real chrome.downloads API.
    const api = fakeDownloadsAPI();
    api.search = vi.fn(async () => [{ state: 'complete', filename: '/Users/x/Downloads/cv.pdf' }]);
    const pending = resolveDownloadedPath(api, 'data:application/pdf;base64,x', 'cv.pdf', 1000);
    api.emit({ id: 1, state: { current: 'complete' } });
    return expect(pending).resolves.toBe('/Users/x/Downloads/cv.pdf');
  });

  it('resolves via a live onChanged event once the id is known', async () => {
    const api = fakeDownloadsAPI();
    api.search = vi.fn(async () => [{ state: 'complete', filename: '/Users/x/Downloads/cv.pdf' }]);
    const pending = resolveDownloadedPath(api, 'data:application/pdf;base64,x', 'cv.pdf', 1000);
    await Promise.resolve(); // let `download()`'s `.then` resolve `id` first
    await Promise.resolve();
    api.emit({ id: 1, state: { current: 'complete' } });
    await expect(pending).resolves.toBe('/Users/x/Downloads/cv.pdf');
  });

  it('ignores an onChanged delta for a different download id', async () => {
    const api = fakeDownloadsAPI(7);
    api.search = vi.fn(async () => [{ state: 'complete', filename: '/Users/x/Downloads/cv.pdf' }]);
    const pending = resolveDownloadedPath(api, 'data:application/pdf;base64,x', 'cv.pdf', 1000);
    api.emit({ id: 999, state: { current: 'complete' } });
    api.emit({ id: 7, state: { current: 'complete' } });
    await expect(pending).resolves.toBe('/Users/x/Downloads/cv.pdf');
  });

  it('rejects when the download is interrupted', async () => {
    const api = fakeDownloadsAPI();
    const pending = resolveDownloadedPath(api, 'data:application/pdf;base64,x', 'cv.pdf', 1000);
    api.emit({ id: 1, state: { current: 'interrupted' } });
    await expect(pending).rejects.toThrow('interrupted');
  });

  it('times out waiting for completion', async () => {
    vi.useFakeTimers();
    try {
      const api = fakeDownloadsAPI();
      const pending = resolveDownloadedPath(api, 'data:application/pdf;base64,x', 'cv.pdf', 1000);
      const assertion = expect(pending).rejects.toThrow(/Downloads/);
      await vi.advanceTimersByTimeAsync(1000);
      await assertion;
    } finally {
      vi.useRealTimers();
    }
  });

  it('cleans up its timer and listener when download() itself rejects', async () => {
    const api = fakeDownloadsAPI();
    api.download = vi.fn(async () => {
      throw new Error('quota exceeded');
    });
    const pending = resolveDownloadedPath(api, 'data:application/pdf;base64,x', 'cv.pdf', 1000);
    await expect(pending).rejects.toThrow('quota exceeded');
    // A listener left registered here would still react to a later, unrelated download's
    // events — removeListener not firing on this path was exactly the leak.
    expect(api.onChanged.removeListener).toHaveBeenCalledTimes(1);
  });
});

describe('pickAttachableUpload', () => {
  const upload = (frame: number, form = 0): FramedUpload => ({ form, frame });

  it('picks the one reachable (top-frame) upload', () => {
    expect(pickAttachableUpload([upload(TOP_FRAME)])).toEqual({ kind: 'found', upload: upload(TOP_FRAME) });
  });

  it('reports none when the page offers no upload at all', () => {
    expect(pickAttachableUpload([])).toEqual({ kind: 'none' });
  });

  it('reports unreachable when every upload lives outside the top frame', () => {
    // The "site-with-iframe" Greenhouse variant: a real upload exists, but only inside a
    // frame this action's unscoped Runtime.evaluate cannot address.
    expect(pickAttachableUpload([upload(2), upload(3)])).toEqual({ kind: 'unreachable' });
  });

  it('reports ambiguous when more than one reachable upload exists (résumé + cover letter)', () => {
    expect(pickAttachableUpload([upload(TOP_FRAME, 0), upload(TOP_FRAME, 1)])).toEqual({
      kind: 'ambiguous',
      count: 2,
    });
  });

  it('picks the reachable one over an unreachable one, rather than reporting ambiguous', () => {
    expect(pickAttachableUpload([upload(2), upload(TOP_FRAME)])).toEqual({
      kind: 'found',
      upload: upload(TOP_FRAME),
    });
  });
});

describe('classifyAttachError', () => {
  it("gives DevTools-open a plain-language message, so 'attach' failing is actionable", () => {
    expect(classifyAttachError('Another debugger is already attached to the tab with id: 123.')).toMatch(
      /devtools/i,
    );
  });

  it('passes through an error it does not recognize', () => {
    expect(classifyAttachError('some other chrome.debugger failure')).toBe('some other chrome.debugger failure');
  });
});
