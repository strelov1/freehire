<script lang="ts">
  // The three facet selects of the publish form: work mode, employment type, seniority.
  import { EMPLOYMENT_TYPE_OPTIONS, SENIORITY_OPTIONS, WORK_MODE_OPTIONS } from '#lib/facets.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { cn } from '#lib/ui/index.js';
  import { messages } from './EmployerView.messages';

  let {
    workMode = $bindable(''),
    employmentType = $bindable(''),
    seniority = $bindable(''),
  }: { workMode?: string; employmentType?: string; seniority?: string } = $props();

  const s = $derived(t(messages, locale()));
  const selectClass =
    'h-9 w-full rounded-lg border border-input bg-transparent px-3 text-sm transition-colors focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring dark:bg-input/30';
</script>

{#snippet field(label: string, value: string, options: { value: string; label: string }[], set: (v: string) => void)}
  <label class="flex flex-1 flex-col gap-1">
    <span class="text-sm font-medium">{label}</span>
    <select {value} onchange={(e) => set(e.currentTarget.value)} class={cn(selectClass)}>
      <option value="">—</option>
      {#each options as o (o.value)}
        <option value={o.value}>{o.label}</option>
      {/each}
    </select>
  </label>
{/snippet}

{@render field(s.fieldWorkMode, workMode, WORK_MODE_OPTIONS, (v) => (workMode = v))}
{@render field(s.fieldEmploymentType, employmentType, EMPLOYMENT_TYPE_OPTIONS, (v) => (employmentType = v))}
{@render field(s.fieldSeniority, seniority, SENIORITY_OPTIONS, (v) => (seniority = v))}
