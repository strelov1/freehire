<script lang="ts">
  import { Clock, MapPin, User } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { countryLabel, skillLabel } from '#lib/facets.js';
  import type { CatalogueMember } from '#lib/generated/contracts.js';
  import { CATEGORY_LABELS, titleCase } from '#lib/labels.js';
  import { talentHeading, talentPlace } from '#lib/talentCard.js';
  import { Avatar, Card, Chip, CountryFlag } from '#lib/ui/index.js';

  // One member of the public Talent Network catalogue.
  //
  // There is no name and no employer to render — the payload carries none, by
  // construction (internal/candidate/talentnetwork/card.go). So most of the card is
  // built from what a dictionary vouched for: what they do, how long they have done it,
  // what with, and roughly where.
  //
  // The photo is the one exception, and it is not a placeholder pretending to be a
  // person — it is the actual member, blurred past recognition server-side on every
  // request (internal/candidate/headshot.Blur), the same rendering the profile page
  // shows. `has_photo` is checked first so a member with none never fires the request
  // GetPhoto would only 404 for — most of the catalogue, in practice.

  let { member }: { member: CatalogueMember } = $props();

  const card = $derived(member.card);

  const heading = $derived(talentHeading(card.seniority, card.category, 'Candidate'));
  const { country, zone, place } = $derived(talentPlace(member));

  // Eight is what fits on one line at the narrowest card width without wrapping into a
  // block that outweighs everything else on it. The rest are counted, not hidden — a
  // card that silently truncated would understate the candidate.
  const SHOWN_SKILLS = 8;
  const shownSkills = $derived(card.skills.slice(0, SHOWN_SKILLS));
  const extraSkills = $derived(Math.max(0, card.skills.length - SHOWN_SKILLS));
</script>

<!-- Density and hover match JobRow: the two lists sit on the same chrome and a visitor
     moves between them, so a card that breathed differently would read as a different
     product. The hover states are on the CARD rather than only on the heading link,
     because the whole row is what a pointer aims at. -->
<Card class="flex gap-4 p-4 transition hover:border-brand hover:bg-accent">
  <!-- display:contents so this wrapper (needed only to keep the snippet below from being
  read as a prop passed to Card) does not itself become a flex item. Avatar falls back to
  fallbackIcon on its own whenever src is absent, so has_photo needs no branch here — only
  which src it gets. -->
  <div class="contents">
    <Avatar
      src={member.has_photo ? `/api/v1/talent/${member.handle}/photo` : undefined}
      size="md"
      class="mt-0.5 size-11 shrink-0 bg-secondary"
      fallbackIcon={personIcon}
    />
    {#snippet personIcon()}
      <User class="size-5" aria-hidden="true" />
    {/snippet}
  </div>

  <div class="flex min-w-0 flex-1 flex-col gap-2">
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

    {#if member.specializations.length}
      <!-- What they said they are OPEN TO, which is the row the catalogue filters on. It
      sits above the skills because it is the forward-looking half of the card: the skills
      and roles below say where somebody has been. -->
      <p class="text-sm text-muted-foreground">
        Open to
        <span class="text-foreground">
          {member.specializations.map((s) => CATEGORY_LABELS[s] ?? titleCase(s)).join(', ')}
        </span>
      </p>
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
