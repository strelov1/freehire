<script lang="ts">
  // "How many years have you worked?" — pre-filled from the CV's own total-years figure
  // when there is one, so the candidate confirms a fact already on their résumé instead of
  // counting it up again.
  //
  // A slider rather than a bracket list: brackets would be a second, coarser vocabulary to
  // keep in step with nothing, and the figure is stored as a plain number either way. Zero
  // is a real answer ("less than a year"), which is why the save path carries an explicit
  // "the candidate set this" flag — see resume.Owned.TotalYearsSet.
  interface Props {
    value: number | null;
    /** What the CV extract computed, shown as the reason the field is pre-filled. Null when
     *  no CV was uploaded or it stated no total.
     *
     *  Named `fromCv` rather than the obvious `derived`: a local binding by that name makes
     *  every `$derived(...)` in this file parse as a store subscription on it, which is a
     *  compile error here and would be a silent runtime surprise in a file where it typed. */
    fromCv: number | null;
    onChange: (years: number) => void;
  }

  let { value, fromCv, onChange }: Props = $props();

  const MAX_YEARS = 40;

  // Null means unanswered; the slider still has to sit somewhere, so it rests at the
  // derived figure (or 0) without that resting position counting as an answer.
  const sliderValue = $derived(value ?? fromCv ?? 0);
  const prefilled = $derived(value === null && fromCv !== null);

  function label(years: number): string {
    if (years === 0) return 'Less than a year';
    if (years >= MAX_YEARS) return `${MAX_YEARS}+ years`;
    return years === 1 ? '1 year' : `${years} years`;
  }
</script>

<h2 class="text-xl font-semibold tracking-tight">How much experience do you have?</h2>
<p class="mt-1 text-sm text-muted-foreground">Paid work only — leave out internships and freelance.</p>

<div class="mt-6">
  <div class="mb-3 flex items-baseline justify-between gap-3">
    <span class="text-2xl font-semibold tabular-nums">{label(sliderValue)}</span>
    {#if prefilled}
      <span class="text-xs text-muted-foreground">From your CV — change it if it's off</span>
    {/if}
  </div>
  <input
    type="range"
    min="0"
    max={MAX_YEARS}
    step="1"
    value={sliderValue}
    oninput={(e) => onChange(e.currentTarget.valueAsNumber)}
    aria-label="Years of experience"
    aria-valuetext={label(sliderValue)}
    class="w-full accent-brand"
  />
  <div class="mt-1 flex justify-between text-xs text-muted-foreground">
    <span>0</span>
    <span>{MAX_YEARS}+</span>
  </div>
</div>
