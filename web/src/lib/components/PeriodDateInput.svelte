<script lang="ts">
  import { Input } from '$lib/ui';
  import type { PeriodDate } from '$lib/types';

  // A work-history/CV period boundary: pick from the browser's native month calendar or
  // type the digits directly (both native `type="month"` behaviors), or fall back to a
  // bare year when the CV/candidate never states a month — `type="month"` cannot express
  // "year only" at all, so that precision needs its own control, not a blank month.
  let {
    value = $bindable(),
    placeholder = '',
  }: { value?: PeriodDate; placeholder?: string } = $props();

  let yearOnly = $state(value?.month === undefined && value?.year !== undefined);

  // "YYYY-MM", the exact string <input type="month"> reads and writes — computed rather
  // than stored, so switching modes or an external change to `value` never drifts out of
  // sync with the control.
  const monthInputValue = $derived(
    value?.year !== undefined && value.month !== undefined
      ? `${value.year}-${String(value.month).padStart(2, '0')}`
      : '',
  );

  function onMonthInput(e: Event) {
    const raw = (e.currentTarget as HTMLInputElement).value; // "YYYY-MM" or "" (cleared)
    if (!raw) {
      value = undefined;
      return;
    }
    // Direct Number(...) calls, not a destructured .map(Number) result — under
    // noUncheckedIndexedAccess, destructuring an array injects `| undefined` per
    // index regardless of how the array was built, which PeriodDate's exact optional
    // month? then rejects.
    const parts = raw.split('-');
    value = { year: Number(parts[0]), month: Number(parts[1]) };
  }

  function onYearInput(e: Event) {
    const raw = (e.currentTarget as HTMLInputElement).value;
    const year = raw ? Number(raw) : NaN;
    value = Number.isFinite(year) ? { year } : undefined;
  }

  // Toggling drops the month: the two controls have no shared in-between state to
  // preserve, and a stale month surviving a switch to "year only" would round-trip back
  // as soon as the candidate toggled again.
  function toggleYearOnly() {
    yearOnly = !yearOnly;
    value = value?.year !== undefined ? { year: value.year } : undefined;
  }
</script>

<div class="flex flex-col gap-1">
  {#if yearOnly}
    <Input
      type="number"
      inputmode="numeric"
      min="1900"
      max="2100"
      class="w-full text-sm"
      {placeholder}
      value={value?.year !== undefined ? String(value.year) : ''}
      oninput={onYearInput}
    />
  {:else}
    <Input
      type="month"
      class="w-full text-sm"
      {placeholder}
      value={monthInputValue}
      oninput={onMonthInput}
    />
  {/if}
  <button
    type="button"
    class="self-start text-xs text-muted-foreground underline-offset-2 hover:underline"
    onclick={toggleYearOnly}
  >
    {yearOnly ? 'I know the month' : "I don't remember the month"}
  </button>
</div>
