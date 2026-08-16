<script lang="ts">
  import type { Company } from '$lib/types';
  import { companyBadges, companyFacts } from '$lib/companyDetails';
  import { CountryFlag } from '$lib/ui';

  // The company's scalar facts as a self-contained card, shown in the jobs sidebar
  // (desktop), as a fallback card under the header (mobile, where the sidebar is
  // hidden), and in the job page's Company tab. Present-only: renders nothing when the
  // company has no facts, so the wrapper never leaves an empty box.
  //
  // The derivations themselves live in $lib/companyDetails, since the Company tab has
  // to ask the same "is there anything here?" question before it draws a heading.
  let { company }: { company: Company } = $props();

  const badges = $derived(companyBadges(company));
  const facts = $derived(companyFacts(company));
</script>

{#if facts.length || badges.length}
  <div class="rounded-xl border border-border bg-card p-4">
    <p class="mb-3 text-xs font-semibold uppercase tracking-wide text-muted-foreground">Company facts</p>
    {#if badges.length}
      <div class="mb-3 flex flex-wrap gap-1.5">
        {#each badges as badge (badge)}
          <span class="rounded-full bg-muted px-2 py-0.5 text-xs font-medium text-foreground">{badge}</span>
        {/each}
      </div>
    {/if}
    {#if facts.length}
      <dl class="grid grid-cols-[auto_1fr] items-baseline gap-x-3 gap-y-2 text-sm">
        {#each facts as fact (fact.term)}
          <dt class="text-muted-foreground">{fact.term}</dt>
          <dd class="flex items-center justify-end gap-1.5 text-right font-medium">
            {#if fact.flag}<CountryFlag code={fact.flag} label={fact.value} class="text-base" />{/if}{fact.value}
          </dd>
        {/each}
      </dl>
    {/if}
  </div>
{/if}
