<script lang="ts">
  // Two thumbs on one track. Each thumb is its own native range input, stacked over a
  // drawn track, so each keeps the browser's keyboard and screen-reader behaviour for
  // free; the inputs ignore the pointer and only their thumbs take it, so a press
  // lands on the thumb under it rather than on whichever input is stacked on top.
  //
  // The thumbs never cross, and stop one step apart rather than meeting: two thumbs on
  // the same value sit exactly on top of each other, and only the upper one can then
  // be grabbed.
  //
  // `valueText` turns a thumb's number into what it means ("No ceiling", "120,000")
  // for aria-valuetext; without it a screen reader announces the raw number, which at
  // the ends of the track is a sentinel rather than a bound.
  let {
    min,
    max,
    step,
    lo,
    hi,
    loLabel,
    hiLabel,
    onLo,
    onHi,
    valueText,
  }: {
    min: number;
    max: number;
    step: number;
    lo: number;
    hi: number;
    loLabel: string;
    hiLabel: string;
    onLo: (n: number) => void;
    onHi: (n: number) => void;
    valueText?: (n: number, thumb: 'lo' | 'hi') => string;
  } = $props();

  const clamp = (n: number) => Math.min(Math.max(n, min), max);
  const pct = (n: number) => ((clamp(n) - min) / (max - min)) * 100;

  // A thumb's centre travels from half a thumb in from one end to half a thumb in from
  // the other, not edge to edge — so the fill and the track press both map through that
  // inset, or the fill overshoots each thumb and a tap lands a few px off.
  const THUMB_HALF = 12;
  const at = (n: number) => `calc(${THUMB_HALF}px + (100% - ${2 * THUMB_HALF}px) * ${pct(n) / 100})`;

  // Past the middle the lower thumb goes on top. Two thumbs parked at the right end
  // could otherwise not be pulled apart: the upper one is on top there and has
  // nowhere left to go.
  const loOnTop = $derived(lo > (min + max) / 2);

  function onInput(el: HTMLInputElement, which: 'lo' | 'hi') {
    commit(el, which, Number(el.value));
  }

  function commit(el: HTMLInputElement, which: 'lo' | 'hi', v: number) {
    const next = clamp(which === 'lo' ? Math.min(v, hi - step) : Math.max(v, lo + step));
    // The browser has already drawn the thumb where the pointer went, and a clamped
    // value equal to the current prop will not redraw it — so put it back by hand.
    if (next !== v) el.value = String(next);
    (which === 'lo' ? onLo : onHi)(next);
  }

  // A press on the bare track moves the nearer thumb there. The inputs ignore the
  // pointer everywhere but their thumbs (see the top), so without this a tap beside a
  // thumb did nothing at all — on a phone, where the thumb is a small target, that is
  // most taps. Pointer-only by nature: the keyboard already has the inputs themselves.
  let loEl = $state<HTMLInputElement>();
  let hiEl = $state<HTMLInputElement>();
  function onTrackDown(e: PointerEvent) {
    // A press on a thumb arrives with the input as its target; the browser drags it.
    if (e.target instanceof HTMLInputElement || !loEl || !hiEl) return;
    const box = (e.currentTarget as HTMLElement).getBoundingClientRect();
    if (box.width <= 2 * THUMB_HALF) return;
    const raw = min + ((e.clientX - box.left - THUMB_HALF) / (box.width - 2 * THUMB_HALF)) * (max - min);
    const v = clamp(min + Math.round((raw - min) / step) * step);
    // The nearer thumb; outside the pair that is always the one on that side.
    const which = v - lo <= hi - v ? 'lo' : 'hi';
    const el = which === 'lo' ? loEl : hiEl;
    // Keeps the press from moving focus to the page, so the focus() below sticks.
    e.preventDefault();
    el.value = String(v);
    commit(el, which, v);
    el.focus();
  }
</script>

<!-- h-6: the thumbs' hit area is 24px tall (see the thumb styles), so the box is too. -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="relative h-6" onpointerdown={onTrackDown}>
  <div class="absolute inset-x-0 top-1/2 h-1 -translate-y-1/2 rounded-full bg-input"></div>
  <div
    class="absolute top-1/2 h-1 -translate-y-1/2 rounded-full bg-primary"
    style:left={at(lo)}
    style:right="calc(100% - {at(hi)})"
  ></div>
  <input
    type="range"
    {min}
    {max}
    {step}
    value={lo}
    aria-label={loLabel}
    aria-valuetext={valueText?.(lo, 'lo')}
    bind:this={loEl}
    class:top={loOnTop}
    oninput={(e) => onInput(e.currentTarget, 'lo')}
  />
  <input
    type="range"
    {min}
    {max}
    {step}
    value={hi}
    aria-label={hiLabel}
    aria-valuetext={valueText?.(hi, 'hi')}
    bind:this={hiEl}
    oninput={(e) => onInput(e.currentTarget, 'hi')}
  />
</div>

<style>
  input {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    margin: 0;
    appearance: none;
    background: transparent;
    pointer-events: none;
  }
  input.top {
    z-index: 1;
  }
  input:focus-visible {
    outline: none;
  }
  input::-webkit-slider-runnable-track {
    background: transparent;
  }
  input::-moz-range-track {
    background: transparent;
  }
  /* The thumb is 24px — WCAG 2.2's minimum target — but draws the same 16px dot it
     always did: the dot (12px of primary, a 2px background-coloured ring, a 1px border
     line) is painted by a radial gradient, and the rest of the box is transparent hit
     area. A border and box-shadow cannot do that, since both grow the visible disc. */
  input {
    --thumb-ring: var(--border);
    --thumb-ring-end: 9px;
    /* One value for both engines' thumbs (a selector list mixing -webkit- and -moz-
       pseudo-elements is dropped whole). It resolves on the input, so it follows the
       focus-visible ring below. */
    --thumb-dot: radial-gradient(
      circle closest-side,
      var(--primary) 6px,
      var(--background) 6px 8px,
      var(--thumb-ring) 8px var(--thumb-ring-end),
      transparent var(--thumb-ring-end)
    );
  }
  input:focus-visible {
    --thumb-ring: var(--ring);
    --thumb-ring-end: 11px;
  }
  input::-webkit-slider-thumb {
    appearance: none;
    width: 24px; /* 2 × THUMB_HALF: the script maps taps in px */
    height: 24px;
    border: 0;
    border-radius: 9999px;
    background: var(--thumb-dot);
    cursor: pointer;
    pointer-events: auto;
  }
  input::-moz-range-thumb {
    width: 24px; /* 2 × THUMB_HALF: the script maps taps in px */
    height: 24px;
    border: 0;
    border-radius: 9999px;
    background: var(--thumb-dot);
    cursor: pointer;
    pointer-events: auto;
  }
</style>
