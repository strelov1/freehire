<script lang="ts">
  import { resolve } from '$app/paths';
  import { Globe } from '@lucide/svelte';
  import {
    companyBadges,
    companyDescription,
    companyFacts,
    companyLocations,
    companySocials,
  } from '#lib/companyDetails.js';
  import type { Company } from '#lib/types.js';
  import { Badge, CountryFlag, ProviderIcon } from '#lib/ui/index.js';
  import CountryFlagStack from './CountryFlagStack.svelte';

  // The employer behind a posting, as the job page's second tab. Deliberately NOT
  // server-rendered: inlining a company's summary into every one of its postings would
  // put hundreds of near-identical pages in competition with /companies/<slug>, which
  // is the page that should rank for it. The job page's server-rendered link to that
  // page (JobView's company row) is what carries the crawlable path between the two.
  //
  // Renders the same facts as the company page's sidebar cards, but laid out for a wide
  // column instead of a narrow one: no card chrome, the facts as a row of columns rather
  // than a stacked definition list, and the summary given the full width. The shared
  // derivations live in #lib/companyDetails so the two layouts cannot disagree about
  // what the company actually has.
  //
  // Rendered only for a company that has details to show (JobView reads it and offers
  // the tab on `hasCompanyDetails`), so there is no loading or empty state here.
  let { company, slug, name }: { company: Company; slug: string; name: string } = $props();

  const badges = $derived(companyBadges(company));
  const facts = $derived(companyFacts(company));
  const description = $derived(companyDescription(company));
  const socials = $derived(companySocials(company));
  const offices = $derived(companyLocations(company));

  // The summary is clamped until asked for — a few hundred words of company boilerplate
  // should not push the link to their other roles off the screen. The toggle appears only
  // when the text actually overflows, measured against the clamped height once the
  // paragraph is in the DOM (the same treatment CompanyAbout gives it in the sidebar).
  let expanded = $state(false);
  let clampable = $state(false);
  let para = $state<HTMLParagraphElement>();

  $effect(() => {
    const el = para;
    if (!el || expanded) return;
    const measure = () => (clampable = el.scrollHeight > el.clientHeight + 1);
    measure();
    // Re-measure on resize rather than only on mount. This column is viewport-driven —
    // the same summary fits on a desktop and overflows at a phone width — so a single
    // measurement leaves the toggle missing from text that has since outgrown the clamp.
    // A late web-font swap moves the line count too, without the element ever resizing.
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    return () => observer.disconnect();
  });
</script>

<!-- The company's own page holds the rest of what the visitor came to this tab for. -->
{#snippet companyLink()}
  <a class="text-sm font-medium text-primary hover:underline" href={resolve('/companies/[slug]', { slug })}>
    All jobs at {name} →
  </a>
{/snippet}

<div class="flex flex-col gap-6">
  {#if badges.length || socials.length}
    <div class="flex flex-wrap items-center justify-between gap-x-4 gap-y-3">
      {#if badges.length}
        <ul class="flex flex-wrap gap-1.5">
          {#each badges as badge (badge)}
            <li><Badge variant="secondary">{badge}</Badge></li>
          {/each}
        </ul>
      {/if}

      <!-- The company's own links, as marks. nofollow for the same reason the Apply
           CTA carries it (JobView): these destinations are whatever the importer
           recorded, and the catalogue never vetted them — a followed link from every
           posting a company has open is exactly what an SEO submission is after.
           The icon carries no text, so each link is named for a screen reader. -->
      <ul class="flex flex-wrap items-center gap-1">
        {#each socials as social (social.key)}
          <li>
            <!-- eslint-disable svelte/no-navigation-without-resolve -- the company's own off-site link, scheme-checked in companySocials; not an internal route -->
            <a
              href={social.href}
              target="_blank"
              rel="nofollow noopener noreferrer"
              aria-label={social.label}
              title={social.label}
              class="inline-flex size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
            >
              {#if social.key === 'website'}
                <Globe class="size-4" aria-hidden="true" />
              {:else}
                <ProviderIcon provider={social.key} />
              {/if}
            </a>
            <!-- eslint-enable svelte/no-navigation-without-resolve -->
          </li>
        {/each}
      </ul>
    </div>
  {/if}

  <!-- Facts as columns rather than rows: in a column this wide a right-aligned
       definition list strands each value an inch from its own term. Two columns on a
       phone, four once there is room; `auto-cols` is not used because a company with
       only two facts should not stretch them across the whole width. -->
  {#if facts.length}
    <dl class="grid grid-cols-2 gap-x-8 gap-y-5 sm:grid-cols-4">
      {#each facts as fact (fact.term)}
        <div class="flex min-w-0 flex-col gap-1">
          <dt class="text-xs font-medium text-muted-foreground">
            {fact.term}
          </dt>
          <dd class="flex min-w-0 items-center gap-1.5 text-sm font-medium">
            {#if fact.flag}<CountryFlag code={fact.flag} label={fact.value} class="shrink-0" />{/if}
            <span class="truncate" title={fact.value}>{fact.value}</span>
          </dd>
        </div>
      {/each}
    </dl>
  {/if}

  <!-- Offices, as an overlapping flag cluster. Its own row rather than a fifth column
       in the grid above: Meta lists 35 countries, which no fixed-width column holds.
       `link` is off — these are the employer's sites, and sending a reader to the
       jobs filter for a country would promise roles there that may not exist. -->
  {#if offices.length}
    <div class="flex flex-col gap-2">
      <p class="text-xs font-medium text-muted-foreground">Offices</p>
      <CountryFlagStack codes={offices} max={12} />
    </div>
  {/if}

  {#if description}
    <div class="flex flex-col gap-2 border-t border-border pt-6 first:border-t-0 first:pt-0">
      <p
        bind:this={para}
        class={['whitespace-pre-wrap text-sm leading-relaxed text-muted-foreground', !expanded && 'line-clamp-5']}
      >
        {description}
      </p>
      {#if clampable || expanded}
        <button
          type="button"
          class="self-start text-sm font-medium text-primary hover:underline"
          aria-expanded={expanded}
          onclick={() => (expanded = !expanded)}
        >
          {expanded ? 'Show less' : 'Show more'}
        </button>
      {/if}
    </div>
  {/if}

  <div>{@render companyLink()}</div>
</div>
