<script lang="ts">
  import { Clock, MapPin, User } from '@lucide/svelte';
  import { page } from '$app/state';
  import Seo from '#lib/components/Seo.svelte';
  import { countryLabel, skillLabel } from '#lib/facets.js';
  import { CATEGORY_LABELS, CERTIFICATION_LABELS, EDUCATION_LEVEL_LABELS, titleCase } from '#lib/labels.js';
  import { breadcrumbJsonLd, jsonLdScript } from '#lib/seo.js';
  import { talentHeading, talentPlace } from '#lib/talentCard.js';
  import { Avatar, Breadcrumbs, Card, Chip, CountryFlag } from '#lib/ui/index.js';
  import type { PageData } from './$types';

  // One member's public card. The same anonymised payload the catalogue list carries,
  // with the work history opened up: what each role was, when, and what it was built
  // with. No name, no employer, no prose — the payload has none.

  let { data }: { data: PageData } = $props();

  const member = $derived(data.member);
  const card = $derived(member.card);

  const heading = $derived(talentHeading(card.seniority, card.category, 'Candidate'));
  const { country, zone, place } = $derived(talentPlace(member));

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/talent/${member.handle}`);
  // The one breadcrumb trail feeding both the visible nav and the structured data below —
  // a single array so the two can never disagree about what the trail is.
  const breadcrumbItems = $derived([{ name: 'Talent Network', href: '/talent' }, { name: heading }]);
  const jsonLd = $derived(
    jsonLdScript([
      breadcrumbJsonLd(
        breadcrumbItems.map((item) => ({
          name: item.name,
          url: item.href ? `${origin}${item.href}` : canonical,
        })),
      ),
    ]),
  );

  /** A role's period, from the two structured dates. "Present" for a role that has not
   *  ended — the backend reports an unset end as current, so an open-ended row here is a
   *  current job rather than a missing date. */
  function period(role: (typeof card.roles)[number]): string {
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
  function roleHeading(role: (typeof card.roles)[number]): string {
    return role.title?.trim() || talentHeading(role.seniority, role.category, 'Role');
  }

  /** An education entry's chip text: "Bachelor's degree · 2019", or just the level
   *  when no year resolved. Never the institution — the card never carries one. */
  function educationLabel(entry: NonNullable<typeof card.education>[number]): string {
    const level = entry.level
      ? (EDUCATION_LEVEL_LABELS[entry.level] ?? titleCase(entry.level))
      : '';
    return entry.year ? `${level} · ${entry.year.year}` : level;
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
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-5 py-6 sm:px-4">
  <Breadcrumbs items={breadcrumbItems} class="mb-4" />

  <div class="flex w-full flex-col gap-4 lg:grid lg:grid-cols-[20rem_minmax(0,1fr)] lg:gap-x-6 lg:gap-y-4">
    <div
      class="flex min-w-0 flex-col gap-6 {card.skills.length ? 'lg:col-start-2' : 'lg:col-span-2'}"
    >
      <div class="flex items-start gap-4">
        <!-- A member's uploaded photo, when present, is shown here — but never the original:
        this route serves a strongly, irreversibly blurred rendering computed server-side on
        every request (internal/candidate/headshot.Blur). Anonymity is still the point, so no
        "name" is passed to Avatar — a member without a photo falls through to the same plain
        silhouette this page has always shown, never a colour/initials avatar, which would
        invent an identity marker this card is not supposed to carry. -->
        <Avatar
          src="/api/v1/talent/{member.handle}/photo"
          size="lg"
          class="size-14 shrink-0 bg-secondary"
          fallbackIcon={personIcon}
        />
        {#snippet personIcon()}
          <User class="size-6" aria-hidden="true" />
        {/snippet}
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
        <p class="text-sm text-muted-foreground">
          This candidate has joined the network but has not published anything yet.
        </p>
      {/if}

      <!-- There used to be a disclaimer here, restating what the projection withholds and
           what it publishes verbatim. It was addressed to the wrong reader: a visitor has
           no decision to make about it, and the person who does — the candidate — is told
           in full on /my/talent-network, before joining. Keeping a second copy on a page
           nobody acts from bought nothing and was one more string to keep true. -->
    </div>

    {#if card.skills.length}
      <aside class="order-last w-full shrink-0 lg:order-none lg:col-start-1 lg:row-start-1">
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
</div>
