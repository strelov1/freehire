<script lang="ts">
  import type { CandidateCard } from '#lib/generated/contracts.js';
  import { skillLabel } from '#lib/facets.js';
  import { CATEGORY_LABELS, CERTIFICATION_LABELS, EDUCATION_LEVEL_LABELS, titleCase } from '#lib/labels.js';
  import { talentHeading } from '#lib/talentCard.js';
  import { Card, Chip } from '#lib/ui/index.js';

  // The body of an anonymous candidate card — what they are open to, the shape of a career,
  // what each role was built with, education and certifications. The Talent Network page
  // (CandidateCardView) and an interview buddy's page each draw their own header and render
  // talentnetwork.CandidateCard's body here, so the two read a career the same way.
  let {
    card,
    specializations = [],
    emptyMessage,
  }: { card: CandidateCard; specializations?: string[]; emptyMessage: string } = $props();

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

<div class="flex flex-col gap-6">
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
</div>
