// Keyboard state for a typeahead whose input owns a listbox — the ARIA combobox pattern.
// Focus never leaves the input: the highlighted row is announced through
// aria-activedescendant, so the input's delayed blur-close never races a keyboard pick.
// The host keeps its own query, fetching and debouncing; this holds only what the
// keyboard and the open/closed state need, so two pickers do not carry two copies of it.
export class ComboboxNav<T> {
  open = $state(false);
  // -1 means nothing is highlighted: Enter then falls through to the surrounding form.
  active = $state(-1);
  readonly listId: string;
  readonly #items: () => readonly T[];
  readonly #loading: () => boolean;
  readonly #pick: (item: T) => void;

  constructor(opts: { id: string; items: () => readonly T[]; loading: () => boolean; pick: (item: T) => void }) {
    this.listId = `${opts.id}-list`;
    this.#items = opts.items;
    this.#loading = opts.loading;
    this.#pick = opts.pick;
  }

  // The list is drawn only while there is something to show in it, and aria-expanded
  // must say the same thing the screen does.
  get listOpen(): boolean {
    return this.open && (this.#items().length > 0 || this.#loading());
  }

  get activeDescendant(): string | undefined {
    return this.listOpen && this.active >= 0 ? this.optionId(this.active) : undefined;
  }

  optionId = (i: number) => `${this.listId}-${i}`;

  close = () => {
    this.open = false;
    this.active = -1;
  };

  onKeydown = (e: KeyboardEvent) => {
    // Mid-composition (CJK/IME) the arrows and Enter belong to the IME.
    if (e.isComposing) return;
    const items = this.#items();
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      if (items.length === 0) return;
      e.preventDefault();
      // An arrow reopens a list the visitor dismissed with Escape.
      this.open = true;
      const last = items.length - 1;
      if (e.key === 'ArrowDown') this.#highlight(this.active < last ? this.active + 1 : 0);
      else this.#highlight(this.active > 0 ? this.active - 1 : last);
      return;
    }
    if (!this.listOpen) return;
    const active = this.active >= 0 ? items[this.active] : undefined;
    if (e.key === 'Enter' && active !== undefined) {
      e.preventDefault(); // pick, don't submit the enclosing form
      this.#pick(active);
      return;
    }
    if (e.key === 'Escape') {
      // Keep the typed text; only the list closes — and not a dialog around it.
      e.preventDefault();
      e.stopPropagation();
      this.close();
    }
  };

  #highlight(i: number) {
    this.active = i;
    document.getElementById(this.optionId(i))?.scrollIntoView?.({ block: 'nearest' });
  }
}
