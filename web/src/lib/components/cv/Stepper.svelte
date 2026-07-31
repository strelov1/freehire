<script lang="ts">
  // A −/value/+ control shared by the margin and font-size settings. Layout only: the caller
  // owns the clamping and rounding (the pure step* helpers in $lib/tailor/geometry), so the two
  // settings cannot drift into different stepping behaviour.
  //
  // `display` is a string rather than a number because a value can be absent: the margin axes
  // show an em dash when their two sides differ, and font size shows the template's own default
  // until the candidate overrides it.
  import { Minus, Plus } from '@lucide/svelte';

  let {
    display,
    label,
    muted = false,
    onstep,
  }: { display: string; label: string; muted?: boolean; onstep: (delta: 1 | -1) => void } = $props();
</script>

<div class="flex w-[7.5rem] items-center rounded-lg border border-input">
  <button
    type="button"
    aria-label="Decrease {label}"
    onclick={() => onstep(-1)}
    class="grid h-8 w-8 shrink-0 place-items-center rounded-l-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
  >
    <Minus class="h-3.5 w-3.5" />
  </button>
  <!-- <output> rather than a span: it carries an implicit status role, so the value is exposed
       and its change announced. An aria-label on a bare span is not reliably read at all. -->
  <output
    class={['flex-1 text-center text-sm tabular-nums', muted ? 'text-muted-foreground' : 'text-foreground']}
    aria-label={label}
  >
    {display}
  </output>
  <button
    type="button"
    aria-label="Increase {label}"
    onclick={() => onstep(1)}
    class="grid h-8 w-8 shrink-0 place-items-center rounded-r-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
  >
    <Plus class="h-3.5 w-3.5" />
  </button>
</div>
