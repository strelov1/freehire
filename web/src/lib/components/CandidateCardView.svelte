<script lang="ts">
  import type { Snippet } from 'svelte';
  import { Clock, MapPin, User } from '@lucide/svelte';
  import type { CandidateCard } from '#lib/generated/contracts.js';
  import { countryLabel, skillLabel } from '#lib/facets.js';
  import { CATEGORY_LABELS, CERTIFICATION_LABELS, EDUCATION_LEVEL_LABELS, titleCase } from '#lib/labels.js';
  import { talentHeading } from '#lib/talentCard.js';
  import { Avatar, Card, Chip, CountryFlag } from '#lib/ui/index.js';

  // One anonymous candidate card, opened up: heading, the shape of a career, and what each
  // role was built with. The Talent Network page and an interview buddy's page both render
  // talentnetwork.CandidateCard, so both render it here. What only the catalogue knows —
  // where somebody is, what they are open to, a blurred photo — arrives as optional props;
  // `actions` sits beside the heading and `children` closes the main column.

  let {
    card,
    photoSrc,
    place = '',
    zone,
    country,
    specializations = [],
    emptyMessage = 'This candidate has joined the network but has not published anything yet.',
    subtitle,
    headingFallback = 'Candidate',
    skillsSide = 'left',
    actions,
    children,
  }: {
    card: CandidateCard;
    photoSrc?: string;
    place?: string;
    zone?: string;
    country?: string;
    specializations?: string[];
    emptyMessage?: string;
    /** A small muted line under the heading (a buddy's handle). The heading itself stays
     *  the dictionary one — a handle is how to address somebody, not what they do. */
    subtitle?: string;
    /** The heading when neither dictionary value resolves. */
    headingFallback?: string;
    /** The catalogue keeps skills in a left rail; a buddy's page lists them as a section of
     *  the main column, which keeps the page at full width. */
    skillsSide?: 'left' | 'inline';
    actions?: Snippet;
    children?: Snippet;
  } = $props();

  const heading = $derived(talentHeading(card.seniority, card.category, headingFallback));

  /** A role's period, from the two structured dates. "Present" for a role that has not
   *  ended — the backend reports an unset end as current, so an open-ended row here is a
   *  current job rather than a missing date. */
  function period(role: CandidateCard['roles'][number]): string {
    const from = role.start ? String(role.start.year) : '';
    const to = role.current || !role.end ? 'Present' : String(role.end.year);
    if (!from) return to;
    return `${from} — ${to}`;
  }

  // The candidate's own words when the card carries them — the one field it publishes
  // verbatim, and the reason this section reads as a career rather than as a column of
  // "Senior · Backend". The dictionary label is the fallback, not the default: a title is
  // absent only when the extraction found none, or when it was blanked upstream for
  // carrying a redaction placeholder. A row with neither still gets a neutral label,
  // because dropping it would make a work history look shorter than it is.
  function roleHeading(role: CandidateCard['roles'][number]): string {
    return role.title?.trim() || talentHeading(role.seniority, role.category, 'Role');
  }

  /** An education entry's chip text: "Bachelor's degree · 2019", or just the level
   *  when no year resolved. Never the institution — the card never carries one. */
  function educationLabel(entry: NonNullable<CandidateCard['education']>[number]): string {
    const level = entry.level
      ? (EDUCATION_LEVEL_LABELS[entry.level] ?? titleCase(entry.level))
      : '';
    return entry.year ? `${level} · ${entry.year.year}` : level;
  }
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
        {#if subtitle}
          <p class="text-sm text-muted-foreground">{subtitle}</p>
        {/if}
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
      {#if actions}
        <div class="flex shrink-0 items-center gap-2">{@render actions()}</div>
      {/if}
    </div>

    {#if specializations.length}
      <!-- What they said they are OPEN TO, which is the row the catalogue filters on, and
      the forward-looking half of the page: the skills and roles below say where somebody
      has been. -->
      <section class="flex flex-col gap-2">
        <h2 class="text-sm font-medium">Open to</h2>
        <div class="flex flex-wrap gap-1.5">
          {#each specializations as spec (spec)}
            <Chip variant="secondary">{CATEGORY_LABELS[spec] ?? titleCase(spec)}</Chip>
          {/each}
        </div>
      </section>
    {/if}

    {#if skillsSide === 'inline' && card.skills.length}
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

    {#if card.education?.length}
      <section class="flex flex-col gap-2">
        <h2 class="text-sm font-medium">Education</h2>
        <div class="flex flex-wrap gap-1.5">
          <!-- Keyed by index for the same reason Experience is: no id, and two entries can
          share a level and year. -->
          {#each card.education as entry, i (i)}
            <Chip>{educationLabel(entry)}</Chip>
          {/each}
        </div>
      </section>
    {/if}

    {#if card.certifications?.length}
      <section class="flex flex-col gap-2">
        <h2 class="text-sm font-medium">Certifications</h2>
        <div class="flex flex-wrap gap-1.5">
          {#each card.certifications as cert (cert)}
            <Chip>{CERTIFICATION_LABELS[cert] ?? titleCase(cert)}</Chip>
          {/each}
        </div>
      </section>
    {/if}

    {#if !card.skills.length && !card.roles.length && !card.education?.length && !card.certifications?.length}
      <p class="text-sm text-muted-foreground">{emptyMessage}</p>
    {/if}

    {@render children?.()}
  </div>

  {#if skillsSide === 'left' && card.skills.length}
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
