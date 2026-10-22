<script lang="ts">
  // Both money questions on one screen, over ONE currency and ONE period selector. Asking
  // them separately would mean asking twice which currency the candidate thinks in, and the
  // two figures are only worth having if they are comparable.
  //
  // They go to different stores, which is invisible here and deliberate: what you WANT is a
  // screening answer an employer eventually sees on an application form, what you EARN is a
  // survey answer nobody outside this product ever does.
  //
  // Each slider yields one exact integer, not a range: the screening answer holds one
  // number, the jobs filter's salary floor takes one number, and an ATS form asks for one.
  import { INCOME_CURRENCIES, INCOME_PERIODS, incomeScale, rescaleIncome } from '#lib/surveyOptions.js';

  interface Props {
    currentIncome: number | null;
    desiredSalary: number | null;
    currency: string;
    period: string;
    onCurrentIncomeChange: (amount: number) => void;
    onDesiredSalaryChange: (amount: number) => void;
    onCurrencyChange: (currency: string) => void;
    onPeriodChange: (period: string) => void;
  }

  let {
    currentIncome,
    desiredSalary,
    currency,
    period,
    onCurrentIncomeChange,
    onDesiredSalaryChange,
    onCurrencyChange,
    onPeriodChange,
  }: Props = $props();

  const scale = $derived(incomeScale(currency, period));

  function retarget(nextCurrency: string, nextPeriod: string) {
    const current = rescaleIncome(currentIncome, currency, period, nextCurrency, nextPeriod);
    const desired = rescaleIncome(desiredSalary, currency, period, nextCurrency, nextPeriod);
    if (current !== null && current !== currentIncome) onCurrentIncomeChange(current);
    if (desired !== null && desired !== desiredSalary) onDesiredSalaryChange(desired);
  }

  // An unanswered slider rests at the bottom without that resting position counting as an
  // answer. Zero is the same non-answer — the save drops it, as both endpoints reject it — so
  // it reads "Not saying" too, which also makes dragging back to the start the way to take an
  // answer back.
  function amountLabel(amount: number | null): string {
    if (amount === null || amount <= 0) return 'Not saying';
    const formatted = amount.toLocaleString();
    return amount >= scale.max ? `${formatted}+ ${currency}` : `${formatted} ${currency}`;
  }
</script>

<h2 class="text-xl font-semibold tracking-tight">What are you earning, and what are you after?</h2>
<p class="mt-1 text-sm text-muted-foreground">
  Take-home, after tax. We use the second figure to stop showing you jobs that pay under it — the
  first one just helps us understand who we're building for.
</p>

<div class="mt-5 flex flex-wrap items-center gap-2">
  <label class="sr-only" for="money-currency">Currency</label>
  <select
    id="money-currency"
    value={currency}
    onchange={(e) => {
      const next = e.currentTarget.value;
      retarget(next, period);
      onCurrencyChange(next);
    }}
    class="rounded-lg border border-input bg-card px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring"
  >
    {#each INCOME_CURRENCIES as c (c.value)}
      <option value={c.value}>{c.label}</option>
    {/each}
  </select>
  <label class="sr-only" for="money-period">Period</label>
  <select
    id="money-period"
    value={period}
    onchange={(e) => {
      const next = e.currentTarget.value;
      retarget(currency, next);
      onPeriodChange(next);
    }}
    class="rounded-lg border border-input bg-card px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring"
  >
    {#each INCOME_PERIODS as p (p.value)}
      <option value={p.value}>{p.label}</option>
    {/each}
  </select>
</div>

<div class="mt-6">
  <div class="mb-2 flex items-baseline justify-between gap-3">
    <span class="text-sm font-medium">What you earn now</span>
    <span class="text-lg font-semibold tabular-nums">{amountLabel(currentIncome)}</span>
  </div>
  <input
    type="range"
    min="0"
    max={scale.max}
    step={scale.step}
    value={currentIncome ?? 0}
    oninput={(e) => onCurrentIncomeChange(e.currentTarget.valueAsNumber)}
    aria-label="What you earn now"
    aria-valuetext={amountLabel(currentIncome)}
    class="w-full accent-brand"
  />
</div>

<div class="mt-6">
  <div class="mb-2 flex items-baseline justify-between gap-3">
    <span class="text-sm font-medium">What you're looking for</span>
    <span class="text-lg font-semibold tabular-nums">{amountLabel(desiredSalary)}</span>
  </div>
  <input
    type="range"
    min="0"
    max={scale.max}
    step={scale.step}
    value={desiredSalary ?? 0}
    oninput={(e) => onDesiredSalaryChange(e.currentTarget.valueAsNumber)}
    aria-label="What you're looking for"
    aria-valuetext={amountLabel(desiredSalary)}
    class="w-full accent-brand"
  />
</div>
