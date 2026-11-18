import { SvelteURLSearchParams } from 'svelte/reactivity';
import { pushState, replaceState } from '$app/navigation';
import { resolve } from '$app/paths';
import type { BoardItem } from '#lib/board.js';
import type { MyJob } from '#lib/types.js';

/**
 * The tracking board's drawer: which application it shows and the history entry that gives
 * it a shareable URL. A Closed drop opens it requiring an outcome choice.
 */
export class BoardDrawer {
  item = $state.raw<BoardItem | null>(null);
  pendingOutcome = $state(false);
  // Set while the drawer is the one a deep link (/my/tracking/[id]) opened, so closing
  // it can swap that URL for the board's own instead of leaving it in the address bar.
  onDeepLink = $state(false);
  // Whether the open drawer has its own history entry (pushed by open). Only then does
  // Back close it, and only then does closing it go back rather than forward. A drawer
  // opened by a Closed drop has no entry, and a deep-linked one IS the entry the visitor
  // arrived on — Back from either leaves the page, as it should.
  #inHistory = false;

  /** `reload` resyncs the board: closing on a pending Closed drop reverts it. */
  constructor(
    private readonly view: () => 'board' | 'list',
    private readonly reload: () => Promise<void>,
  ) {}

  open = (item: MyJob) => {
    this.item = item as BoardItem;
    this.pendingOutcome = false;
    // Give the open application its own shareable URL without a full navigation.
    // Keyed by the row id, so an application whose posting was removed gets one too.
    // The query rides along so the filtered rows behind the drawer stay filtered.
    const id = (item as BoardItem).id;
    this.#inHistory = true;
    if (this.view() === 'list') {
      const params = new SvelteURLSearchParams(location.search);
      params.set('application', id);
      // eslint-disable-next-line svelte/no-navigation-without-resolve -- the route is resolved before its query is added
      pushState(resolve('/my/tracking/list') + `?${params}`, { trackingItem: id });
    } else {
      // eslint-disable-next-line svelte/no-navigation-without-resolve -- the route is resolved before its query is added
      pushState(resolve('/my/tracking/[id]', { id }) + location.search, { trackingItem: id });
    }
  };

  /** The deep link's application, opened once the board is built. */
  openDeepLinked(item: MyJob) {
    this.item = item as BoardItem;
    this.pendingOutcome = false;
    this.onDeepLink = true;
  }

  /** A Closed drop: outcome unknown until the user picks one. */
  askOutcome(item: BoardItem) {
    this.item = item;
    this.pendingOutcome = true;
  }

  /** Close the drawer without touching the URL — Back has already moved it. */
  dismiss = () => {
    if (this.pendingOutcome) {
      // Closing without choosing an outcome reverts the Closed drop.
      this.pendingOutcome = false;
      void this.reload();
    }
    this.item = null;
  };

  close = () => {
    const hadEntry = this.#inHistory;
    this.#inHistory = false;
    this.dismiss();
    if (hadEntry) {
      // Undo open's push rather than pushing the list URL on top of it: a second entry is
      // what left Back walking through closed drawers, and going back also returns to the
      // exact view and filters the drawer was opened from.
      history.back();
    } else if (this.onDeepLink) {
      // Arrived on a deep link: swap the per-application URL for the board's own.
      this.onDeepLink = false;
      const params = new SvelteURLSearchParams(location.search);
      params.delete('application');
      const base = this.view() === 'list' ? resolve('/my/tracking/list') : resolve('/my/tracking');
      // eslint-disable-next-line svelte/no-navigation-without-resolve -- both possible base routes above are resolved
      replaceState(base + (params.size ? `?${params}` : ''), {});
    }
  };

  /**
   * Back and Forward over the drawer's history entry. SvelteKit restores page.state with
   * the entry, so the open application is read from it rather than kept only in the
   * component — a URL that changed while the drawer stayed put was the bug. `find` looks
   * the id up on the board.
   */
  follow(id: string | undefined, find: (id: string) => BoardItem | undefined) {
    if (id) {
      if (this.item?.id !== id) {
        const found = find(id);
        if (!found) return;
        this.item = found;
        this.pendingOutcome = false;
      }
      this.#inHistory = true;
    } else if (this.#inHistory) {
      this.#inHistory = false;
      this.dismiss();
    }
  }
}
