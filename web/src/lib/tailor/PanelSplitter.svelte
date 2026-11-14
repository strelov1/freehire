<script lang="ts">
  import { clampWidth, keyResizeWidth, PANEL_MAX_WIDTH, PANEL_MIN_WIDTH } from '#lib/cvGeometry.js';

  // The tailoring workspace's splitter on the right edge of its left panel (`panel`): drag it,
  // or focus it and use the arrow keys, to set `width`. Hidden below lg and while `collapsed`.
  let {
    width = $bindable(),
    panel,
    collapsed,
  }: { width: number; panel: HTMLElement | undefined; collapsed: boolean } = $props();

  let resizing = false;

  // Width is the cursor's distance from the panel's own left edge.
  function start(e: PointerEvent) {
    resizing = true;
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
  }
  function move(e: PointerEvent) {
    if (!resizing || !panel) return;
    const left = panel.getBoundingClientRect().left;
    width = clampWidth(e.clientX - left, PANEL_MIN_WIDTH, PANEL_MAX_WIDTH);
  }
  // Also the pointercancel handler: a touch the browser takes back must end the drag. A
  // cancelled pointer has already lost its capture, hence the check.
  function stop(e: PointerEvent) {
    resizing = false;
    const el = e.currentTarget as HTMLElement;
    if (el.hasPointerCapture(e.pointerId)) el.releasePointerCapture(e.pointerId);
  }
  // The panel sits on the left, so moving the splitter right (ArrowRight) widens it.
  function key(e: KeyboardEvent) {
    const next = keyResizeWidth(width, e.key, 'ArrowRight');
    if (next === null) return;
    e.preventDefault();
    width = next;
  }
</script>

<!-- LEFT SPLITTER: drag, or focus it and use the arrow keys. -->
<!-- A focusable separator is the ARIA window-splitter widget; the lint reads every separator as static. -->
<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
<div
  class={[
    'hidden w-1.5 shrink-0 cursor-col-resize touch-none bg-border/50 transition-colors hover:bg-border focus-visible:bg-ring focus-visible:outline-none',
    collapsed ? 'lg:hidden' : 'lg:block',
  ]}
  role="separator"
  tabindex="0"
  aria-orientation="vertical"
  aria-label="Resize editor panel"
  aria-valuenow={width}
  aria-valuemin={PANEL_MIN_WIDTH}
  aria-valuemax={PANEL_MAX_WIDTH}
  onpointerdown={start}
  onpointermove={move}
  onpointerup={stop}
  onpointercancel={stop}
  onkeydown={key}
></div>
