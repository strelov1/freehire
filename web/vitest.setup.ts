import { cleanup } from '@testing-library/svelte';
import { afterEach } from 'vitest';

// Without globals: true testing-library cannot register its own teardown, so
// mounted components would pile up in the document across cases.
afterEach(cleanup);

// jsdom does not implement ResizeObserver at all (same gap
// design-system/vitest.setup.ts stubs). Several components mount-effect against it
// (CvHtmlPreview, JobCompanyPanel, pipeline/SourcesField) — the stub exists so any of
// them can be rendered here without throwing, not because a current `*.spec.ts` needs it.
if (typeof ResizeObserver === 'undefined') {
  class ResizeObserverStub {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  globalThis.ResizeObserver = ResizeObserverStub;
}

// jsdom parses `<dialog>` but implements none of its methods, so `dialog.svelte`'s
// `el.showModal()` throws and nothing built on Dialog can be rendered here at all. The stub
// drives the one piece of state the component and its tests read back — `open` — and leaves
// the rest (focus trapping, inertness, the top layer) to the browser, which is where those
// belong anyway.
if (typeof HTMLDialogElement !== 'undefined' && !HTMLDialogElement.prototype.showModal) {
  HTMLDialogElement.prototype.showModal = function showModal(this: HTMLDialogElement) {
    this.open = true;
  };
  HTMLDialogElement.prototype.show = function show(this: HTMLDialogElement) {
    this.open = true;
  };
  HTMLDialogElement.prototype.close = function close(this: HTMLDialogElement) {
    this.open = false;
    this.dispatchEvent(new Event('close'));
  };
}
