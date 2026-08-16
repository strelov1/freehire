<script lang="ts">
  import { resolve } from '$app/paths';
  import { api } from '$lib/api';
  import { hasCompanyDetails } from '$lib/companyDetails';
  import type { Company } from '$lib/types';
  import { EmptyState, Skeleton } from '$lib/ui';
  import CompanyAbout from './CompanyAbout.svelte';
  import CompanyFacts from './CompanyFacts.svelte';

  // The employer behind a posting, as the job page's second tab. Deliberately NOT
  // server-rendered: inlining a company's summary into every one of its postings would
  // put hundreds of near-identical pages in competition with /companies/<slug>, which
  // is the page that should rank for it. The job page's server-rendered link to that
  // page (JobView's company row) is what carries the crawlable path between the two.
  //
  // `active` is the tab's selected state, not its visibility — the panel stays mounted
  // either way so the tab's aria-controls always resolves. The fetch is what waits.
  let { slug, name, active }: { slug: string; name: string; active: boolean } = $props();

  let company = $state.raw<Company | null>(null);
  let failed = $state(false);
  // A plain variable, not $state: the effect below writes it, and a reactive read
  // would make the effect its own dependency. Nothing renders from it.
  let requested = false;

  // Fetch once, on the first activation. `limit=1` is the endpoint's floor — the API
  // clamps it to at least 1, so a company-only read is not expressible and the single
  // job that comes back is discarded (the same note companies/[slug]/+page.server.ts
  // carries). The parent keys this component on the company slug, so navigating to a
  // job at another company remounts it and resets the guard; there is no stale-company
  // case to handle here.
  $effect(() => {
    if (!active || requested) return;
    requested = true;
    api
      .getCompany(slug, 1, 0)
      .then((r) => (company = r.company))
      .catch(() => (failed = true));
  });

  // In flight: activated, and neither answer has arrived. Derived rather than a third
  // piece of state, which would have to be cleared on both the success and the failure
  // path and could disagree with them if either were ever missed.
  const pending = $derived(active && company == null && !failed);
  const empty = $derived(company != null && !hasCompanyDetails(company));
</script>

<!-- Offered in every terminal state, empty and failed included, so the click is never a
     dead end: the company's own page holds what the visitor came to this tab for. -->
{#snippet companyLink()}
  <a
    class="text-sm font-medium text-primary hover:underline"
    href={resolve('/companies/[slug]', { slug })}
  >
    All jobs at {name} →
  </a>
{/snippet}

{#if pending}
  <div class="flex flex-col gap-4">
    <Skeleton class="h-40 w-full rounded-xl" />
    <Skeleton class="h-28 w-full rounded-xl" />
  </div>
{:else if failed}
  <EmptyState
    title="Couldn't load company details."
    variant="destructive"
    action={companyLink}
  />
{:else if empty}
  <EmptyState title="We don't have details on {name} yet." variant="muted" action={companyLink} />
{:else if company}
  <div class="flex flex-col gap-4">
    <CompanyFacts {company} />
    <CompanyAbout {company} />
    <div>{@render companyLink()}</div>
  </div>
{/if}
