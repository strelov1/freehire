<script module lang="ts">
  /**
   * id of the tab button for `tab` within the strip driving `panelId`. The panel lives at the
   * call site, so it needs this to point `aria-labelledby` back at its own tab — exported so
   * the convention lives in one place instead of being retyped as a magic string.
   */
  export function tabStripId(panelId: string, tab: string): string {
    return `${panelId}-tab-${tab}`;
  }
</script>

<script lang="ts" generics="T extends string">
  import { cn } from './cn.js';

  // A horizontal tab strip that survives a narrow viewport. Past its own width the row
  // scrolls rather than wrapping: a flex row with nowhere to put the overflow first squeezes
  // labels down to min-content (so a two-word label breaks mid-label) and then spills the
  // remainder past the container's edge.
  //
  // Owns the tablist semantics as well — arrow-key movement over a roving tabindex — because
  // `role="tablist"` is a promise to assistive tech that the group is one widget stepped
  // through with the arrows. Hand-rolled copies of this row could not each keep that promise;
  // one component can.
  //
  // Distinct from `Tabs`: that primitive is a pill/segmented control with no overflow
  // handling, for a small fixed set of options. This one is for a row that can outgrow its
  // container.
  let {
    tabs,
    active,
    onSelect,
    label,
    panelId,
    class: extra,
  }: {
    tabs: readonly { id: T; label: string }[];
    active: T;
    onSelect: (id: T) => void;
    /** Names the strip for assistive tech, e.g. "Profile sections". */
    label: string;
    /** id of the `role="tabpanel"` element these tabs drive. */
    panelId: string;
    class?: string;
  } = $props();

  let strip = $state<HTMLElement | null>(null);
  let buttons = $state<(HTMLElement | null)[]>([]);
  const activeIndex = $derived(tabs.findIndex((t) => t.id === active));

  // Whether either end is out of view. Measured rather than assumed: an unconditional fade
  // would paint a phantom edge-shadow on a row that already fits.
  let atStart = $state(true);
  let atEnd = $state(true);

  function measure() {
    if (!strip) return;
    atStart = strip.scrollLeft <= 1;
    // 1px of slack — fractional layout widths leave scrollLeft just shy of its true maximum,
    // so an exact comparison never reports the end as reached.
    atEnd = strip.scrollLeft >= strip.scrollWidth - strip.clientWidth - 1;
  }

  // Re-measure on any resize of the strip or of a label inside it. The children matter on
  // their own: a late web-font swap changes label widths, and so scrollWidth, without ever
  // resizing the strip itself.
  $effect(() => {
    const el = strip;
    if (!el) return;
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    for (const child of el.children) observer.observe(child);
    return () => observer.disconnect();
  });

  // Fade whichever edge has content beyond it. A mask rather than a gradient overlay so the
  // strip needs no knowledge of the surface it was dropped onto — it fades its own content to
  // transparent, and reads correctly both on the page background and inside a card.
  const maskStyle = $derived.by(() => {
    if (atStart && atEnd) return undefined;
    const from = atStart ? 'black 0' : 'transparent 0, black 1.5rem';
    const to = atEnd ? 'black 100%' : 'black calc(100% - 1.5rem), transparent 100%';
    return `mask-image: linear-gradient(to right, ${from}, ${to})`;
  });

  // Keep the active tab in view. Not cosmetic: a call site's own empty states can link back
  // to a sibling tab, so the selection can change without the user having touched the strip,
  // and the newly-active tab would otherwise stay parked off-screen.
  $effect(() => {
    const el = buttons[activeIndex];
    if (!strip || !el) return;
    // Scroll the strip itself instead of calling scrollIntoView, which would also drag the
    // page vertically whenever the strip sits below the fold.
    const tab = el.getBoundingClientRect();
    const view = strip.getBoundingClientRect();
    if (tab.right > view.right) strip.scrollLeft += tab.right - view.right;
    else if (tab.left < view.left) strip.scrollLeft -= view.left - tab.left;
  });

  // Arrows move the selection and activate as they go — the automatic-activation pattern,
  // which suits panels that are already rendered client-side and cost nothing to show.
  // Home/End jump to the ends. Focus follows, since a roving tabindex leaves only the
  // selected tab reachable by Tab.
  function onKeydown(event: KeyboardEvent) {
    let next: number;
    switch (event.key) {
      case 'ArrowRight':
        next = (activeIndex + 1) % tabs.length;
        break;
      case 'ArrowLeft':
        next = (activeIndex - 1 + tabs.length) % tabs.length;
        break;
      case 'Home':
        next = 0;
        break;
      case 'End':
        next = tabs.length - 1;
        break;
      default:
        return;
    }
    const target = tabs[next];
    if (!target) return; // only reachable with an empty `tabs`, where there is nothing to move to.
    event.preventDefault();
    onSelect(target.id);
    buttons[next]?.focus();
  }
</script>

<div class={cn('relative', extra)}>
  <div
    bind:this={strip}
    onscroll={measure}
    role="tablist"
    aria-label={label}
    style={maskStyle}
    class="flex gap-4 overflow-x-auto border-b border-border sm:gap-5 [-ms-overflow-style:none] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
  >
    {#each tabs as t, i (t.id)}
      <button
        bind:this={buttons[i]}
        type="button"
        role="tab"
        id={tabStripId(panelId, t.id)}
        aria-selected={t.id === active}
        aria-controls={panelId}
        tabindex={t.id === active ? 0 : -1}
        onclick={() => onSelect(t.id)}
        onkeydown={onKeydown}
        class="-mb-px shrink-0 whitespace-nowrap border-b-2 px-1 pb-2.5 text-sm font-medium transition-colors {t.id ===
        active
          ? 'border-brand text-foreground'
          : 'border-transparent text-muted-foreground hover:text-foreground'}"
      >
        {t.label}
      </button>
    {/each}
  </div>
</div>
