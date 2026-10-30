<script lang="ts">
  import { Clock, MapPin, User } from '@lucide/svelte';
  import type { CandidateCard } from '#lib/generated/contracts.js';
  import { countryLabel, skillLabel } from '#lib/facets.js';
  import { talentHeading } from '#lib/talentCard.js';
  import { Avatar, Chip, CountryFlag } from '#lib/ui/index.js';
  import CandidateCardSections from './CandidateCardSections.svelte';

  // One Talent Network card, opened up: heading, where somebody is, and the career below it
  // (CandidateCardSections, shared with an interview buddy's page, which draws its own
  // header). What only the catalogue knows — where somebody is, what they are open to, a
  // blurred photo — arrives as optional props.

  let {
    card,
    photoSrc,
    place = '',
    zone,
    country,
    specializations = [],
  }: {
    card: CandidateCard;
    photoSrc?: string;
    place?: string;
    zone?: string;
    country?: string;
    specializations?: string[];
  } = $props();

  const heading = $derived(talentHeading(card.seniority, card.category, 'Candidate'));
</script>

<!-- Two columns from lg: the skills panel on the left at a fixed width, the card on the
right. The panel follows the card in the DOM so a narrow screen reads the card first. -->
<div class="flex w-full flex-col gap-4 lg:flex-row lg:gap-6">
  <div class="flex min-w-0 flex-col gap-6 lg:flex-1">
    <div class="flex items-start gap-4">
      <!-- A Talent Network member's uploaded photo, when present, is shown here — but never
      the original: that route serves a strongly, irreversibly blurred rendering computed
      server-side on every request (internal/candidate/headshot.Blur). Anonymity is still
      the point, so no "name" is passed to Avatar — a member without a photo falls through
      to the same plain silhouette, never a colour/initials avatar, which would invent an
      identity marker this card is not supposed to carry. -->
      <Avatar src={photoSrc} size="lg" class="size-14 shrink-0 bg-secondary" fallbackIcon={personIcon} />
      {#snippet personIcon()}
        <User class="size-6" aria-hidden="true" />
      {/snippet}
      <div class="flex min-w-0 flex-1 flex-col gap-1">
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

    <CandidateCardSections {card} {specializations} emptyMessage="This candidate has joined the network but has not published anything yet." />
  </div>

  {#if card.skills.length}
    <aside class="order-last w-full shrink-0 lg:order-first lg:w-80">
      <div class="sticky top-20 flex flex-col gap-3 rounded-xl border border-border bg-card p-4">
        <p class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">Skills</p>
        <div class="flex flex-wrap gap-1.5">
          {#each card.skills as skill (skill)}
            <Chip>{skillLabel(skill)}</Chip>
          {/each}
        </div>
      </div>
    </aside>
  {/if}
</div>
