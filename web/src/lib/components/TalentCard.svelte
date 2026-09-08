<script lang="ts">
  import { Clock, MapPin, User } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { countryLabel, skillLabel } from '$lib/facets';
  import type { CatalogueMember } from '$lib/generated/contracts';
  import { CATEGORY_LABELS, SENIORITY_LABELS } from '$lib/labels';
  import { countryOfTimezone } from '$lib/timezoneCountry';
  import { Card, Chip, CountryFlag } from '$lib/ui';

  // One member of the public Talent Network catalogue.
  //
  // There is no name, no photo and no employer to render — the payload carries none, by
  // construction (internal/candidate/talentnetwork/card.go). So the card is built from
  // what a dictionary vouched for: what they do, how long they have done it, what with,
  // and roughly where. The generic person icon is the avatar; anything else here would
  // be a placeholder pretending to be a person.

  let { member }: { member: CatalogueMember } = $props();

  const card = $derived(member.card);

  // The heading, assembled from two dictionary values rather than quoted from a CV.
  // Either half can be absent — classify never guesses — so a member whose title
  // resolved to nothing still gets a heading rather than an empty line.
  const grade = $derived(card.seniority ? (SENIORITY_LABELS[card.seniority] ?? card.seniority) : '');
  const discipline = $derived(card.category ? (CATEGORY_LABELS[card.category] ?? card.category) : '');
  const heading = $derived([grade, discipline].filter(Boolean).join(' ') || 'Candidate');

  // The country comes from the TIMEZONE, not from the city: the zone is machine-precise
  // and generated from tzdata, whereas turning a city name into a country would need a
  // gazetteer this repo does not have. Undefined for Etc/* and UTC, which name an offset
  // rather than a place.
  const country = $derived(countryOfTimezone(member.timezone));

  // The city half of an IANA zone, underscores restored: 'America/New_York' reads as
  // 'New York'. The region prefix is dropped because the city already implies it.
  const zone = $derived(member.timezone?.split('/').pop()?.replace(/_/g, ' '));

  const place = $derived(member.cities.length ? member.cities.join(', ') : '');

  // Eight is what fits on one line at the narrowest card width without wrapping into a
  // block that outweighs everything else on it. The rest are counted, not hidden — a
  // card that silently truncated would understate the candidate.
  const SHOWN_SKILLS = 8;
  const shownSkills = $derived(card.skills.slice(0, SHOWN_SKILLS));
  const extraSkills = $derived(Math.max(0, card.skills.length - SHOWN_SKILLS));
</script>

<Card class="flex gap-4 p-5">
  <div
    class="mt-0.5 flex size-11 shrink-0 items-center justify-center rounded-full bg-secondary text-muted-foreground"
  >
    <User class="size-5" aria-hidden="true" />
  </div>

  <div class="flex min-w-0 flex-1 flex-col gap-3">
    <div class="flex flex-wrap items-baseline justify-between gap-2">
      <h2 class="text-base font-semibold">
        <!-- The whole heading is the link, so the target is large and the link text says
        what it leads to. -->
        <a class="hover:underline" href={resolve('/talent/[handle]', { handle: member.handle })}>
          {heading}
        </a>
      </h2>
      {#if card.total_years}
        <span class="text-sm text-muted-foreground">
          {card.total_years}
          {card.total_years === 1 ? 'year' : 'years'} of experience
        </span>
      {/if}
    </div>

    {#if country || zone || place}
      <div class="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-muted-foreground">
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
    {/if}

    {#if shownSkills.length}
      <div class="flex flex-wrap gap-1.5">
        <!-- Keyed by the skill itself: it is the value the chip renders, and it is unique
        within a card because the projection canonicalises through a set. -->
        {#each shownSkills as skill (skill)}
          <Chip>{skillLabel(skill)}</Chip>
        {/each}
        {#if extraSkills}
          <span class="self-center text-xs text-muted-foreground">+{extraSkills} more</span>
        {/if}
      </div>
    {/if}
  </div>
</Card>
