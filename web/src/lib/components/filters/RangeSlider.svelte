<script lang="ts">
  // Two thumbs on one track. Each thumb is its own native range input, stacked over a
  // drawn track, so each keeps the browser's keyboard and screen-reader behaviour for
  // free; the inputs ignore the pointer and only their thumbs take it, so a press
  // lands on the thumb under it rather than on whichever input is stacked on top.
  //
  // The thumbs never cross, and stop one step apart rather than meeting: two thumbs on
  // the same value sit exactly on top of each other, and only the upper one can then
  // be grabbed.
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
  } = $props();

  const clamp = (n: number) => Math.min(Math.max(n, min), max);
  const pct = (n: number) => ((clamp(n) - min) / (max - min)) * 100;

  // Past the middle the lower thumb goes on top. Two thumbs parked at the right end
  // could otherwise not be pulled apart: the upper one is on top there and has
  // nowhere left to go.
  const loOnTop = $derived(lo > (min + max) / 2);

  function onInput(el: HTMLInputElement, which: 'lo' | 'hi') {
    const v = Number(el.value);
    const next = clamp(which === 'lo' ? Math.min(v, hi - step) : Math.max(v, lo + step));
    // The browser has already drawn the thumb where the pointer went, and a clamped
    // value equal to the current prop will not redraw it — so put it back by hand.
    if (next !== v) el.value = String(next);
    (which === 'lo' ? onLo : onHi)(next);
  }
</script>

<div class="relative h-5">
  <div class="absolute inset-x-0 top-1/2 h-1 -translate-y-1/2 rounded-full bg-input"></div>
  <div
    class="absolute top-1/2 h-1 -translate-y-1/2 rounded-full bg-primary"
    style:left="{pct(lo)}%"
    style:right="{100 - pct(hi)}%"
  ></div>
  <input
    type="range"
    {min}
    {max}
    {step}
    value={lo}
    aria-label={loLabel}
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
  input::-webkit-slider-thumb {
    appearance: none;
    width: 1rem;
    height: 1rem;
    border: 2px solid var(--background);
    border-radius: 9999px;
    background: var(--primary);
    box-shadow: 0 0 0 1px var(--border);
    cursor: pointer;
    pointer-events: auto;
  }
  input::-moz-range-thumb {
    width: 1rem;
    height: 1rem;
    border: 2px solid var(--background);
    border-radius: 9999px;
    background: var(--primary);
    box-shadow: 0 0 0 1px var(--border);
    cursor: pointer;
    pointer-events: auto;
  }
  input:focus-visible::-webkit-slider-thumb {
    box-shadow: 0 0 0 3px var(--ring);
  }
  input:focus-visible::-moz-range-thumb {
    box-shadow: 0 0 0 3px var(--ring);
  }
</style>
