/**
 * A dialog component imported on the click that first opens it, so the pages that never
 * open it keep it out of their module graph. `loading` covers the import, so a caller can
 * disable the control that opened it: a click that looks ignored invites a second one.
 */
export class LazyDialog<C> {
  component = $state.raw<C | null>(null);
  open = $state(false);
  loading = $state(false);
  /** The last import failed (offline, or a deploy swapped the chunk away). */
  failed = $state(false);

  constructor(private readonly load: () => Promise<{ default: C }>) {}

  /** Import the component if it is not loaded yet, then open it. A failed import leaves
   *  it closed with `failed` set, so the next click retries. */
  show = async () => {
    this.failed = false;
    if (!this.component) {
      this.loading = true;
      try {
        this.component = (await this.load()).default;
      } catch {
        this.failed = true;
        return;
      } finally {
        this.loading = false;
      }
    }
    this.open = true;
  };

  close = () => {
    this.open = false;
  };
}
