<script lang="ts">
  import { Clock, MapPin, User } from '@lucide/svelte';
  import Seo from '$lib/components/Seo.svelte';
  import { countryLabel, skillLabel } from '$lib/facets';
  import { CATEGORY_LABELS, titleCase } from '$lib/labels';
  import { talentHeading, talentPlace } from '$lib/talentCard';
  import { Card, Chip, CountryFlag } from '$lib/ui';
  import type { PageData } from './$types';

  // One member's public card. The same anonymised payload the catalogue list carries,
  // with the work history opened up: what each role was, when, and what it was built
  // with. No name, no employer, no prose — the payload has none.

  let { data }: { data: PageData } = $props();

  const member = $derived(data.member);
  const card = $derived(member.card);

  const heading = $derived(talentHeading(card.seniority, card.category, 'Candidate'));
  const { country, zone, place } = $derived(talentPlace(member));

  /** A role's period, from the two structured dates. "Present" for a role that has not
   *  ended — the backend reports an unset end as current, so an open-ended row here is a
   *  current job rather than a missing date. */
  function period(role: (typeof card.roles)[number]): string {
    const from = role.start ? String(role.start.year) : '';
    const to = role.current || !role.end ? 'Present' : String(role.end.year);
    if (!from) return to;
    return `${from} — ${to}`;
  }

  // A title the dictionary could not place still gets a row, under a neutral label:
  // dropping it would make a work history look shorter than it is.
  function roleHeading(role: (typeof card.roles)[number]): string {
    return talentHeading(role.seniority, role.category, 'Role');
  }
</script>

<Seo
  title="{heading} — freehire Talent Network"
  description="An anonymous candidate profile from freehire's Talent Network."
/>
<svelte:head>
  <!-- A card is a person, and a search engine's cache of one would outlive their decision
  to leave the network. The list page is indexable; an individual card is not. -->
  <meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto flex w-full max-w-3xl flex-col gap-6 px-4 py-8">
  <div class="flex items-start gap-4">
    <div
      class="flex size-14 shrink-0 items-center justify-center rounded-full bg-secondary text-muted-foreground"
    >
      <User class="size-6" aria-hidden="true" />
    </div>
    <div class="flex min-w-0 flex-col gap-1">
      <h1 class="text-2xl font-semibold tracking-tight">{heading}</h1>
      <div class="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-muted-foreground">
        {#if card.total_years}
          <span>{card.total_years} {card.total_years === 1 ? 'year' : 'years'} of experience</span>
        {/if}
        {#if place}
          <span class="flex items-center gap-1.5">
            <MapPin class="size-3.5" aria-hidden="true" />
            {place}
          </span>
        {/if}
        {#if zone}
          <span class="flex items-center gap-1.5">
            <Clock class="size-3.5" aria-hidden="true" />
            {zone}
          </span>
        {/if}
        {#if country}
          <CountryFlag code={country} label={countryLabel(country)} />
        {/if}
      </div>
    </div>
  </div>

  {#if member.specializations.length}
    <!-- What they said they are OPEN TO, which is the row the catalogue filters on, and
    the forward-looking half of the page: the skills and roles below say where somebody
    has been. It is on the list card too — a detail page showing less than the row that
    led to it reads as a page that failed to load. -->
    <section class="flex flex-col gap-2">
      <h2 class="text-sm font-medium">Open to</h2>
      <div class="flex flex-wrap gap-1.5">
        {#each member.specializations as spec (spec)}
          <Chip variant="secondary">{CATEGORY_LABELS[spec] ?? titleCase(spec)}</Chip>
        {/each}
      </div>
    </section>
  {/if}

  {#if card.skills.length}
    <section class="flex flex-col gap-2">
      <h2 class="text-sm font-medium">Skills</h2>
      <div class="flex flex-wrap gap-1.5">
        {#each card.skills as skill (skill)}
          <Chip>{skillLabel(skill)}</Chip>
        {/each}
      </div>
    </section>
  {/if}

  {#if card.roles.length}
    <section class="flex flex-col gap-2">
      <h2 class="text-sm font-medium">Experience</h2>
      <div class="flex flex-col gap-3">
        <!-- Keyed by the index: roles carry no id, and two roles genuinely can share a
        heading and a period (a title the dictionary could not place, twice), so anything
        derived from the content risks a duplicate key — which aborts the whole block. -->
        {#each card.roles as role, i (i)}
          <Card class="flex flex-col gap-2 p-4">
            <div class="flex flex-wrap items-baseline justify-between gap-2">
              <span class="font-medium">{roleHeading(role)}</span>
              <span class="text-sm text-muted-foreground">{period(role)}</span>
            </div>
            {#if role.stack?.length}
              <div class="flex flex-wrap gap-1.5">
                {#each role.stack as tech (tech)}
                  <Chip>{skillLabel(tech)}</Chip>
                {/each}
              </div>
            {/if}
          </Card>
        {/each}
      </div>
    </section>
  {/if}

  {#if !card.skills.length && !card.roles.length}
    <p class="text-sm text-muted-foreground">
      This candidate has joined the network but has not published anything yet.
    </p>
  {/if}

  <p class="text-xs text-muted-foreground">
    Names, employers and contact details are never shown here — the profile is published
    anonymously by the candidate's own choice.
  </p>
</div>
