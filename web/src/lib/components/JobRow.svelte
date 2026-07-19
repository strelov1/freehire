<script lang="ts">
  import type { Snippet } from 'svelte';
  import { resolve } from '$app/paths';
  import { Bell, Bookmark, EyeOff, X } from '@lucide/svelte';
  import CompanyLogo from './CompanyLogo.svelte';
  import { api } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { cardTags, formatSalary } from '$lib/enrichment';
  import { metaDescription } from '$lib/seo';
  import type { Job } from '$lib/types';
  import { Badge } from '$lib/ui';
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
  // don't fight the card's navigation). The saved list passes the reminder chip here.
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
    job: Job;
    dimViewed?: boolean;
    newTab?: boolean;
    compact?: boolean;
    footer?: Snippet;
    onHide?: (slug: string) => void;
  } = $props();

  const isViewed = $derived(dimViewed && hasViewed(job.public_slug));

  const tags = $derived(cardTags(job));
  // A one-line blurb under the title so a card conveys what the job is without
  // opening it. Prefer the clean model-written summary, but only tech jobs are
  // enriched — fall back to a plain-text snippet of the raw (HTML) posting so
  // non-tech jobs still show a description. metaDescription strips the tags.
  const blurb = $derived(job.enrichment?.summary || metaDescription(job.description ?? '', 220));
  const salary = $derived(job.enrichment ? formatSalary(job.enrichment) : null);
  // Top-level `skills` is the served (deterministic-dictionary) facet; the raw
  // `enrichment.skills` is kept in the JSONB and NOT served, so it's always absent
  // here — read the dictionary field so the card's skill chips actually populate.
  const skills = $derived(job.skills ?? []);
  // How recently it was posted is a key signal, so it leads the header.
  const posted = $derived(timeAgo(job.posted_at));

  const MAX_SKILLS = 5;
  const shownSkills = $derived(skills.slice(0, MAX_SKILLS));
  const extraSkills = $derived(skills.length - MAX_SKILLS);

  // Whether the signed-in user has saved this job, read from the shared saved set
  // (loaded once on the browse view). The bookmark reflects this and updates the
  // set on toggle, so every card for the same job stays in sync.
  const saved = $derived(isSaved(job.public_slug));
  // Guards against a double-click racing two requests for the same job.
  let saving = $state(false);

  // The non-intrusive post-save reminder prompt: after a fresh save (not an
  // unsave), offer quick "remind me" choices anchored under the bookmark. Ignoring
  // it leaves the account default in effect; picking a delay schedules that reminder
  // for this job (an explicit choice works even when reminders are off by default).
  // Suppressed in the compact (assistant) card, where the corner has no room.
  let reminderPrompt = $state(false);
  let reminderBusy = $state(false);
  let reminderSet = $state(false);

  const REMINDER_CHOICES: { label: string; days: number }[] = [
    { label: 'Tomorrow', days: 1 },
    { label: 'In 3 days', days: 3 },
    { label: 'In a week', days: 7 },
  ];

  async function setReminder(days: number) {
    if (reminderBusy) return;
    reminderBusy = true;
    try {
      await api.saveJob(job.public_slug, { delay_days: days });
      reminderSet = true;
    } catch {
      // Best-effort: leave the prompt so the user can retry.
    } finally {
      reminderBusy = false;
    }
  }

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
    // Unsaving closes any open prompt; a fresh save opens it.
    reminderPrompt = false;
    reminderSet = false;
    try {
      if (wasSaved) await api.unsaveJob(job.public_slug);
      else await api.saveJob(job.public_slug);
      if (!wasSaved && !compact) reminderPrompt = true;
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
  class:opacity-60={isViewed}
>
  <!-- Company + timestamp rail: a quiet eyebrow that yields the stage to the title.
       The name truncates to a single line, so a long company (e.g. "Veterinary
       Emergency Group (VEG)") keeps the logo centred and the card rhythm even
       instead of wrapping into a ragged multi-line header. -->
  <!-- pr-9 reserves the top-right corner for the save button (an overlay outside
       this link), so the timestamp never slides under it. -->
  <div class="flex items-center justify-between gap-3 pr-9">
    <div class="flex min-w-0 items-center gap-2">
      <CompanyLogo name={job.company} size="size-7" />
      <span class="truncate text-sm font-medium text-muted-foreground">
        {job.company || 'Unknown company'}
      </span>
    </div>
    {#if posted}
      <span class="shrink-0 text-xs tabular-nums text-muted-foreground">{posted}</span>
    {/if}
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
  {#if job.reality || tags.length > 0}
    <div class="mt-2 flex flex-wrap items-center gap-1.5">
      <RealityBadge reality={job.reality} />
      {#each tags as tag (tag)}
        <Badge variant="outline">{tag}</Badge>
      {/each}
    </div>
  {/if}

  {#if blurb && !compact}
    <p class="mt-2 line-clamp-2 text-sm text-muted-foreground">{blurb}</p>
  {/if}

  <div class="mt-3 flex items-end justify-between gap-3">
    <div class="flex min-w-0 flex-wrap items-center gap-1.5">
      {#each shownSkills as skill (skill)}
        <Badge variant="brand">{skill}</Badge>
      {/each}
      {#if extraSkills > 0}
        <span class="text-xs text-muted-foreground">+{extraSkills} skills</span>
      {/if}
    </div>
    {#if salary}
      <span class="shrink-0 text-base font-bold tabular-nums tracking-tight">{salary}</span>
    {/if}
  </div>
</a>

{#if footer}
  <!-- Optional in-card actions row (e.g. the saved list's reminder chip), divided
       from the content and rendered outside the <a> so its controls stay clickable. -->
  <div class="border-t border-border px-4 py-2.5">
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

{#if reminderPrompt && saved}
  <!-- Post-save reminder prompt: a small popover under the bookmark. Non-modal —
       dismissing it (or ignoring the card) leaves the account default in effect. -->
  <div class="absolute right-2.5 top-12 z-10 w-56 rounded-lg border border-border bg-popover p-2.5 shadow-md">
    {#if reminderSet}
      <p class="flex items-center gap-1.5 px-1 py-0.5 text-xs text-muted-foreground">
        <Bell class="size-3.5 text-brand" aria-hidden="true" /> Reminder set.
        <button type="button" onclick={() => (reminderPrompt = false)} class="ml-auto font-medium text-foreground hover:opacity-80">Done</button>
      </p>
    {:else}
      <div class="flex items-center justify-between px-1 pb-1.5">
        <span class="text-xs font-semibold">Remind me to apply?</span>
        <button type="button" onclick={() => (reminderPrompt = false)} aria-label="Dismiss reminder prompt" class="text-muted-foreground hover:text-foreground">
          <X class="size-3.5" aria-hidden="true" />
        </button>
      </div>
      <div class="flex flex-wrap gap-1.5">
        {#each REMINDER_CHOICES as c (c.days)}
          <button
            type="button"
            onclick={() => setReminder(c.days)}
            disabled={reminderBusy}
            class="rounded-full border border-border bg-background px-2.5 py-1 text-xs font-medium text-muted-foreground transition-colors hover:border-brand hover:text-brand disabled:opacity-50"
          >
            {c.label}
          </button>
        {/each}
      </div>
    {/if}
  </div>
{/if}

<!-- Hide control: only the browse feed passes `onHide`, so this appears there and
     nowhere else. A quiet icon in the card's bottom-right corner, revealed on hover
     (and on keyboard focus). It's an overlay sibling of the card link, so hiding
     never navigates. An opaque background keeps it legible over the salary corner on
     the minority of cards that show one. -->
{#if onHide}
  <button
    type="button"
    onclick={hide}
    disabled={hiding}
    aria-label="Hide this job"
    title="Not interested — hide this job"
    class="absolute bottom-2.5 right-2.5 grid size-8 place-items-center rounded-lg bg-accent text-muted-foreground opacity-0 shadow-sm ring-1 ring-border transition hover:text-foreground focus-visible:opacity-100 group-hover:opacity-100 disabled:pointer-events-none disabled:opacity-50"
  >
    <EyeOff class="size-3.5" aria-hidden="true" />
  </button>
{/if}
</div>
