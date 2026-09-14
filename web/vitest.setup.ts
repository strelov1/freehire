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
