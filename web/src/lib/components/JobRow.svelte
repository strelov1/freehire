<script lang="ts">
  import type { Snippet } from 'svelte';
  import { resolve } from '$app/paths';
  import { Bookmark, Eye, EyeOff } from '@lucide/svelte';
  import { companyLogoUrl } from '$lib/logo';
  import CountryFlagStack from './CountryFlagStack.svelte';
  import JobMatchBar from './JobMatchBar.svelte';
  import { api } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { cardTags, cardTagsFromCard, formatSalary } from '$lib/enrichment';
  import { computeClientMatch, matchTeaser, resolveMatchState } from '$lib/jobMatch';
  import { profileStore } from '$lib/profile.svelte';
  import { metaDescription } from '$lib/seo';
  import type { Job, JobCard } from '$lib/types';
  import { Badge, EntityLogo } from '$lib/ui';
  import { supersedesReality } from '$lib/ghost';
  import CredentialBadge from './CredentialBadge.svelte';
  import BackerBadge from './BackerBadge.svelte';
  import { credentialBadges } from '$lib/credentials';
  import GhostBadge from './GhostBadge.svelte';
  import RealityBadge from './RealityBadge.svelte';
  import { timeAgo } from '$lib/utils';
  import { hasViewed } from '$lib/viewedJobs.svelte';
  import { isSaved, markSaved, markUnsaved } from '$lib/savedJobs.svelte';
  import { markDismissed, markUndismissed } from '$lib/dismissedJobs.svelte';

  // Single source of truth for how a job appears in any list (jobs list and
  // company detail). The whole card is a link to the job detail.
  //
  // `dimViewed` dims the card when the signed-in user has already viewed this
  // job, so the browse list shows what's been seen. The My Jobs surfaces (where
  // every card is viewed by definition) pass `dimViewed={false}` to opt out.
  // `newTab` opens the job in a new browser tab (used when the card is rendered
  // inside the assistant chat, so the conversation stays open). `compact` tightens
  // the card for the narrow chat column (smaller padding + title, one-line title,
  // no blurb). Both default off so the jobs list / company pages are unchanged.
  // `footer` is an optional actions row rendered inside the card, below the link
  // content (a sibling of the <a>, never nested in it — so its interactive controls
  // don't fight the card's navigation). The hidden list passes an un-hide control here.
  //
  // `onHide` is the feed's hook for the "hide this job" gesture: when set (only the
  // browse feed passes it), the card shows a hover-revealed hide control, and after
  // a successful dismiss it calls back with the slug so the feed can surface an undo
  // affordance. Surfaces that reuse JobRow without it (saved/hidden lists, tracking
  // board, assistant chat) get no hide control — leaving it out scopes the gesture
  // to the feed.
  let {
    job,
    dimViewed = true,
    newTab = false,
    compact = false,
    footer,
    onHide,
  }: {
    // Either the catalogue's full posting or the tracking listing's card. The row draws the
    // same thing from both: the card carries a server-cut blurb where the posting carries the
    // whole description, and the two facet fields a full Job keeps inside `enrichment` arrive
    // flat. Everything the card omits is optional here already.
    job: Job | JobCard;
    dimViewed?: boolean;
    newTab?: boolean;
    compact?: boolean;
    footer?: Snippet;
    onHide?: (slug: string) => void;
  } = $props();

  const isViewed = $derived(dimViewed && hasViewed(job.public_slug));

  const tags = $derived('enrichment' in job ? cardTags(job) : cardTagsFromCard(job));
  // Only the credential subset of job.collections renders here, so the signal row's
  // guard has to test that subset — a job carrying only editorial tags would
  // otherwise open an empty flex row and leave a stray margin under the title.
  const credentials = $derived(credentialBadges(job.collections));
  // A one-line blurb under the title so a card conveys what the job is without
  // opening it. Prefer the clean model-written summary, but only tech jobs are
  // enriched — fall back to a plain-text snippet of the raw (HTML) posting so
  // non-tech jobs still show a description. metaDescription strips the tags.
  const blurb = $derived(
    'enrichment' in job
      ? job.enrichment?.summary || metaDescription(job.description ?? '', 220)
      : metaDescription(job.blurb ?? '', 220),
  );
  // Salary lives in the enrichment, which a card does not carry: the tracking lists never
  // showed it (their rows came from the same listing) and a row without one simply omits it.
  const salary = $derived('enrichment' in job && job.enrichment ? formatSalary(job.enrichment) : null);
  // Top-level `skills` is the served (deterministic-dictionary) facet; the raw
  // `enrichment.skills` is kept in the JSONB and NOT served, so it's always absent
  // here — read the dictionary field so the card's skill chips actually populate.
  const skills = $derived(job.skills ?? []);
  // The two read-time signals. Both are attached by the catalogue's projection and neither
  // has ever been computed for the tracking listing, so a card simply has none — the rows
  // that render cards showed no badge before this change either.
  const reality = $derived('reality' in job ? job.reality : undefined);
  const ghost = $derived('ghost' in job ? job.ghost : undefined);
  // How recently it was posted is a key signal, so it leads the header.
  const posted = $derived(timeAgo(job.posted_at));

  const MAX_SKILLS = 5;
  const shownSkills = $derived(skills.slice(0, MAX_SKILLS));
  const extraSkills = $derived(skills.length - MAX_SKILLS);

  // Card-level profile match, computed entirely in the browser (no per-card request):
  // the exact overlap between this job's skills and the signed-in viewer's profile
  // skills. The job is server-rendered; only the profile hydrates client-side, loaded
  // once and deduped across every card by the store (SSR-safe no-op for guests).
  $effect(() => {
    if (isAuthenticated()) profileStore.ensureLoaded();
  });
  const profileSkills = $derived(profileStore.profile?.skills ?? []);
  const matchState = $derived(
    resolveMatchState({
      jobSkills: skills,
      authenticated: isAuthenticated(),
      profileLoaded: profileStore.loaded,
      profileSkills,
    }),
  );
  // The viewer's skills as a lowercase set, only when there's a real match to show —
  // used to tint each skill chip (a skill you have vs one you're missing). Null in the
  // locked states, where the teaser below decides the tint instead.
  const haveSet = $derived(
    matchState === 'ready' ? new Set(profileSkills.map((s) => s.toLowerCase())) : null,
  );
  const match = $derived(matchState === 'ready' ? computeClientMatch(skills, profileSkills) : null);

  // A viewer with no match to show — signed out, or signed in with no skills — gets the
  // job's deterministic teaser instead of an empty card corner: the same chips and strip,
  // blurred, as an invitation to sign in. Seeded from the slug, so the figures survive
  // hydration and agree with the sidebar block on the job's own page.
  const teaser = $derived(
    matchState === 'guest' || matchState === 'no-profile'
      ? matchTeaser(job.public_slug, skills)
      : null,
  );

  // A chip is red when the viewer's own profile lacks the skill, or — under the teaser —
  // when the teaser marked it missing. Both tints come from the same source as the strip
  // below, so the chips and the percentage can never tell different stories.
  function chipVariant(skill: string): 'brand' | 'missing' {
    if (haveSet) return haveSet.has(skill.toLowerCase()) ? 'brand' : 'missing';
    if (teaser) return teaser.missing.has(skill) ? 'missing' : 'brand';
    return 'brand';
  }

  // Whether the signed-in user has saved this job, read from the shared saved set
  // (loaded once on the browse view). The bookmark reflects this and updates the
  // set on toggle, so every card for the same job stays in sync.
  const saved = $derived(isSaved(job.public_slug));
  // Guards against a double-click racing two requests for the same job.
  let saving = $state(false);

  // Toggle the save mark. Optimistic: flip the shared set first so the bookmark
  // fills instantly, then confirm with the server and roll back on failure. A
  // signed-out click routes to sign-in instead (no auto-save afterwards). The
  // button is an overlay sibling of the card link — not a descendant — so this
  // never triggers the card's navigation.
  async function toggleSave() {
    if (!isAuthenticated()) {
      openAuthDialog('login');
      return;
    }
    if (saving) return;
    saving = true;
    const wasSaved = saved;
    if (wasSaved) markUnsaved(job.public_slug);
    else markSaved(job.public_slug);
    try {
      if (wasSaved) await api.unsaveJob(job.public_slug);
      else await api.saveJob(job.public_slug);
    } catch {
      if (wasSaved) markSaved(job.public_slug);
      else markUnsaved(job.public_slug);
    } finally {
      saving = false;
    }
  }

  // Guards against a double-click firing two dismiss requests for the same job.
  let hiding = $state(false);

  // Hide (dismiss) the job from the feed. Optimistic: mark it hidden in the shared
  // set first so the feed drops the card instantly, then confirm with the server and
  // roll back on failure. A signed-out click routes to sign-in instead. On success we
  // hand the slug to the feed (onHide) so it can surface an undo affordance. Like the
  // save button, this is an overlay sibling of the card link, so it never navigates.
  async function hide() {
    if (!isAuthenticated()) {
      openAuthDialog('login');
      return;
    }
    if (hiding) return;
    hiding = true;
    markDismissed(job.public_slug);
    try {
      await api.dismissJob(job.public_slug);
      onHide?.(job.public_slug);
    } catch {
      markUndismissed(job.public_slug);
    } finally {
      hiding = false;
    }
  }
</script>

<!-- The card chrome (border, background, hover) lives on this wrapper, not the <a>,
     so an optional footer row can sit inside the same bordered box as a sibling of
     the link — interactive footer controls never nest inside the navigation <a>.
     `group` lets the hover-revealed hide control fade in on card hover. -->
<div class="group relative rounded-xl border border-border bg-card transition hover:border-brand hover:bg-accent">
<a
  href={resolve('/jobs/[slug]', { slug: job.public_slug })}
  target={newTab ? '_blank' : undefined}
  rel={newTab ? 'noopener' : undefined}
  class={[
    'block hover:opacity-100',
    compact ? 'p-3' : 'p-4',
  ]}
  class:opacity-80={isViewed}
>
  <!-- Company + timestamp rail: a quiet eyebrow that yields the stage to the title.
       The name truncates to a single line, so a long company (e.g. "Veterinary
       Emergency Group (VEG)") keeps the logo centred and the card rhythm even
       instead of wrapping into a ragged multi-line header. -->
  <!-- pr-9 reserves the top-right corner for the save button (an overlay outside
       this link), so the timestamp never slides under it. -->
  <div class="flex items-center justify-between gap-3 pr-9">
    <div class="flex min-w-0 items-center gap-2">
      <EntityLogo
        name={job.company || 'Unknown company'}
        src={companyLogoUrl(job.company) ?? undefined}
        shape="square"
        size="sm"
      />
      <span class="truncate text-sm font-medium text-muted-foreground">
        {job.company || 'Unknown company'}
      </span>
      <!-- Who backed the employer, next to the employer. A fact about the company,
           so it reads with the company name rather than joining the signal row
           below, which carries role-level facts and already wraps on a phone. The
           whole card is a link, so the mark is display-only here. -->
      <BackerBadge collections={job.collections} class="shrink-0" />
    </div>
    <div class="flex shrink-0 items-center gap-1.5 text-muted-foreground">
      {#if isViewed}
        <!-- A quiet "you've seen this" marker, paired with the lighter dim on the card
             so viewed jobs recede without becoming hard to read. -->
        <Eye class="size-3.5" aria-label="Viewed" />
      {/if}
      {#if posted}
        <span class="text-xs tabular-nums">{posted}</span>
      {/if}
    </div>
  </div>

  <!-- The title is the card's hero — a size up from the body with tight leading, so
       the eye lands on the role first. -->
  <h3
    class={[
      'font-semibold leading-snug tracking-tight',
      compact ? 'mt-2 line-clamp-1 text-base' : 'mt-2.5 line-clamp-2 text-lg sm:text-[1.35rem]',
    ]}
  >
    {job.title}
  </h3>

  <!-- Signal row: reality chip + the region/employment facets, grouped under the
       title as quiet outline chips so they read as metadata, not decoration. -->
  {#if reality || tags.length > 0 || job.countries?.length || credentials.length > 0}
    <div class="mt-2 flex flex-wrap items-center gap-1.5">
      <!-- evergreen_posting IS the reality verdict, so showing both chips states one
           fact twice, the second time louder. The ghost chip carries it inside its
           checklist; where ghost is silent, reality renders exactly as before. -->
      {#if supersedesReality(ghost)}
        <GhostBadge {ghost} />
      {:else}
        <RealityBadge {reality} />
      {/if}
      {#each tags as tag (tag)}
        <Badge variant="outline">{tag}</Badge>
      {/each}
      <!-- Register-backed employer credentials (visa-sponsor licences). A fact about
           the employer, so it sits with the metadata chips and carries its own
           disclaimer rather than reading as a promise about this role. -->
      <CredentialBadge collections={job.collections} />
      <!-- Eligible countries as an overlapping flag cluster. Display-only here: the
           whole card is a link, so the flags carry no nested filter links. -->
      {#if job.countries?.length}
        <CountryFlagStack codes={job.countries} max={5} class="ml-0.5 text-base" />
      {/if}
    </div>
  {/if}

  {#if blurb && !compact}
    <p class="mt-2 line-clamp-2 text-sm text-muted-foreground">{blurb}</p>
  {/if}

  <!-- sm:pr-9 (when the hide control is present) reserves the bottom-right corner for
       it — the counterpart to the header's pr-9 for the save button — so the skills
       tail and the salary never slide under the icon. Only from `sm` up: below it the
       salary sits on its own line, well clear of the corner.
       On a phone the salary is ~160px of the ~360px card, so sharing a row with it
       squeezed the chips into a five-line stack. Stacking the two below `sm` gives the
       chips the full width (three lines become two); from `sm` up the original
       chips-left / salary-right row is unchanged. -->
  <div
    class={[
      'mt-3 flex flex-col items-start gap-1.5 sm:flex-row sm:items-end sm:justify-between sm:gap-3',
      onHide && 'sm:pr-9',
    ]}
  >
    <!-- flex-1 (not the default basis:auto) so the chips get every pixel the salary
         doesn't need. With basis:auto the row's width is derived from its max-content
         width, so shorter chips would ironically shrink the row and cause *more*
         wrapping. -->
    <!-- Under the teaser the chips are blurred with the strip below, because their
         green/red tint is part of the same not-yet-computed signal. The blur stops at
         this container so the salary beside it — real information — stays crisp. The
         skill names themselves are left announced to assistive technology: they're the
         job's own facet, and colour was never conveyed there in the first place. -->
    <div
      class={[
        'flex w-full min-w-0 flex-wrap items-center gap-1.5 sm:w-auto sm:flex-1',
        teaser && 'pointer-events-none select-none opacity-90 blur-[1.5px]',
      ]}
    >
      <!-- A long dictionary slug ("event-driven-architecture") would wrap inside its
           own chip and stretch the row to two lines, breaking the card's rhythm on a
           phone. Each chip stays one line and ellipsises past max-w; the full skill is
           in `title` for anyone who needs it. -->
      {#each shownSkills as skill (skill)}
        <Badge variant={chipVariant(skill)} class="max-w-[9rem]">
          <span class="truncate" title={skill}>{skill}</span>
        </Badge>
      {/each}
      {#if extraSkills > 0}
        <span class="text-xs text-muted-foreground">+{extraSkills} skills</span>
      {/if}
    </div>
    {#if salary}
      <span class="shrink-0 text-base font-bold tabular-nums tracking-tight">{salary}</span>
    {/if}
  </div>

  <!-- Card-level profile match: the real client-computed coverage bar for a signed-in
       viewer with a skills profile, the blurred teaser for one without. `blurred` is
       gated on there being no real match too, so the two props cannot contradict each
       other and render a genuine score under a blur. `gutterRight` keeps the percent
       clear of the feed's bottom-right hide control. -->
  <JobMatchBar match={match ?? teaser} blurred={!match && !!teaser} gutterRight={!!onHide} />
</a>

{#if teaser}
  <!-- The blurred strip is hidden from assistive technology, so this is what stands in
       for it — the invitation that would actually get this viewer a real match. It sits
       outside the card link deliberately: `sr-only` is clip-based, not display:none, so
       inside the <a> it would join the link's accessible name and every card in the feed
       would announce a sign-in instruction that the link doesn't carry out. -->
  <span class="sr-only">
    {matchState === 'guest'
      ? 'Sign in to see how this job matches your profile'
      : 'Add your skills to see how this job matches your profile'}
  </span>
{/if}

{#if footer}
  <!-- Optional in-card actions row (e.g. the hidden list's un-hide control, the
       assistant deck's rationale), divided from the content and rendered outside
       the <a> so its controls stay clickable. Its inline padding tracks the card's
       own, or the row would sit 4px out of line in a compact card. -->
  <div class={['border-t border-border py-2.5', compact ? 'px-3' : 'px-4']}>
    {@render footer()}
  </div>
{/if}

<!-- Save toggle: an icon-only overlay in the card's top-right corner. It sits
     outside the <a> (a sibling, not a descendant), so clicking it toggles the
     bookmark without navigating to the job. -->
<button
  type="button"
  onclick={toggleSave}
  disabled={saving}
  aria-pressed={saved}
  aria-label={saved ? 'Remove from saved' : 'Save job'}
  title={saved ? 'Saved' : 'Save'}
  class={[
    'absolute right-2.5 top-2.5 grid size-8 place-items-center rounded-lg transition hover:bg-accent hover:text-brand disabled:pointer-events-none disabled:opacity-50',
    saved ? 'text-brand' : 'text-muted-foreground',
  ]}
>
  <Bookmark class="size-[1.05rem] {saved ? 'fill-current' : ''}" aria-hidden="true" />
</button>

<!-- Hide control: only the browse feed passes `onHide`, so this appears there and
     nowhere else. A quiet icon in the card's bottom-right corner, revealed on hover
     (and on keyboard focus). Touch devices have no hover, so `pointer-coarse` keeps it
     always visible there. It's an overlay sibling of the card link, so hiding never
     navigates. No background plate — the bottom row reserves a pr-9 gutter for it, so
     nothing renders underneath and it reads as a bare icon (mirrors the save button). -->
{#if onHide}
  <button
    type="button"
    onclick={hide}
    disabled={hiding}
    aria-label="Hide this job"
    title="Not interested — hide this job"
    class="absolute bottom-2.5 right-2.5 grid size-8 place-items-center rounded-lg text-muted-foreground opacity-0 transition hover:bg-accent hover:text-foreground focus-visible:opacity-100 group-hover:opacity-100 pointer-coarse:opacity-100 disabled:pointer-events-none disabled:opacity-50"
  >
    <EyeOff class="size-3.5" aria-hidden="true" />
  </button>
{/if}
</div>
