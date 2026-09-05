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
  import { INCOME_CURRENCIES, INCOME_PERIODS, INCOME_STEP, incomeMax } from '$lib/surveyOptions';

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

  const max = $derived(incomeMax(period));

  // An unanswered slider rests at the bottom without that resting position counting as an
  // answer — the page tracks answered-ness separately, and only a slider the candidate
  // actually moved is sent.
  function amountLabel(amount: number | null): string {
    if (amount === null) return 'Not saying';
    const formatted = amount.toLocaleString();
    return amount >= max ? `${formatted}+ ${currency}` : `${formatted} ${currency}`;
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
    onchange={(e) => onCurrencyChange(e.currentTarget.value)}
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
    onchange={(e) => onPeriodChange(e.currentTarget.value)}
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
    {max}
    step={INCOME_STEP}
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
    {max}
    step={INCOME_STEP}
    value={desiredSalary ?? 0}
    oninput={(e) => onDesiredSalaryChange(e.currentTarget.valueAsNumber)}
    aria-label="What you're looking for"
    aria-valuetext={amountLabel(desiredSalary)}
    class="w-full accent-brand"
  />
</div>
