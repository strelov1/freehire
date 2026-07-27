import { cleanup } from '@testing-library/svelte';
import { afterEach } from 'vitest';

// Without globals: true testing-library cannot register its own teardown, so
// mounted components would pile up in the document across cases.
afterEach(cleanup);

// jsdom gives <dialog> its `open` property but not showModal()/close(). Dialog
// delegates the modal behaviour itself to the platform — the top layer, the
// focus trap, the inert background — so none of that is under test here; these
// two stubs exist only so the component's own effects can run.
if (!HTMLDialogElement.prototype.showModal) {
  HTMLDialogElement.prototype.showModal = function showModal(this: HTMLDialogElement) {
    this.open = true;
  };
  HTMLDialogElement.prototype.close = function close(this: HTMLDialogElement) {
    this.open = false;
    this.dispatchEvent(new Event('close'));
  };
}
