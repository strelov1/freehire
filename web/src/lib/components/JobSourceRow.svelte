<script lang="ts">
  import { Ban, RotateCcw } from '@lucide/svelte';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { sourceFamily, sourceLabel } from '#lib/facets.js';
  import { filterHref } from '#lib/enrichment.js';
  import { SOURCE_LOGO_DOMAINS, sourceLogoUrl } from '#lib/logo.js';
  import { profileStore } from '#lib/profile.svelte.js';
  import { syncProfileAlert } from '#lib/profileAlertSync.js';
  import { promptSignIn } from '#lib/signin.js';
  import { Badge, EntityLogo, Tooltip, badgeVariants, cn } from '#lib/ui/index.js';
  import AdzunaAttribution from './AdzunaAttribution.svelte';
  import DevGlobalJobsAttribution from './DevGlobalJobsAttribution.svelte';

  // The job page's provenance row: where this posting came from, and the control to stop
  // seeing that source at all. Lifted out of JobView — which is 1,100 lines — because it is
  // the one part of that sidebar with its own state and its own write, and because a
  // control that edits a profile deserves a test that does not have to mount a job page.
  let {
    source: posted,
    jobUrl,
    manuallyAdded = false,
  }: { source: string; jobUrl: string; manuallyAdded?: boolean } = $props();

  // A regional market (whatjobs-uk) is shown, linked and avoided as its brand.
  const source = $derived(sourceFamily(posted));

  const label = $derived(sourceLabel(source));

  // Compared lowercased: the server normalises excluded_sources to lowercase on save
  // (userprofile.normalizeExcludedSet), and nothing guarantees a caller hands us the same
  // casing the list was written in.
  const avoided = $derived(
    (profileStore.profile?.excluded_sources ?? []).some((s) => s.toLowerCase() === source.toLowerCase()),
  );

  let pending = $state(false);
  let failed = $state(false);

  // The write goes to the PROFILE, not to this job: a source is a crawl adapter serving
  // thousands of postings, so avoiding one means "stop showing me anything from
  // SmartRecruiters" — the same scope the profile's own "Sources to avoid" card has. The
  // shape mirrors JobMatch's skill avoid, down to re-syncing the saved-search alert, which
  // is the only way the exclusion reaches a digest.
  async function setAvoided(avoid: boolean) {
    if (pending) return;
    if (!isAuthenticated()) {
      promptSignIn();
      return;
    }
    pending = true;
    failed = false;
    try {
      await (avoid ? profileStore.avoidSource(source) : profileStore.unavoidSource(source));
      void syncProfileAlert();
    } catch {
      failed = true;
    } finally {
      pending = false;
    }
  }
</script>

<!-- No frame of its own: the job page sets it in the sidebar's footer line beside the votes. -->
<div class="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1.5 text-xs text-muted-foreground">
  <!-- One chip: the source link, and for a signed-in reader the avoid control at its right
       edge — the SkillChip shape. The tooltip wraps only the icon, so hovering the name
       still reads as a link and not as "Avoid this source". -->
  <span class={cn(badgeVariants({ variant: 'outline' }), 'gap-1.5 pl-1')}>
    <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- internal /jobs filter link from filterHref; query-only, no route to resolve -->
    <a href={filterHref('source', source)} class="inline-flex items-center gap-1.5 transition hover:text-foreground">
      <!-- The source's own brand mark: its DISPLAY NAME, plus the platform's own domain where
           one is curated — never a posting host, which serves the employer's logo under the
           platform's name. EntityLogo also catches a 404 that landed before hydration, which a
           hand-rolled onerror cannot. -->
      <EntityLogo
        name={label}
        src={sourceLogoUrl(label, SOURCE_LOGO_DOMAINS[source]) ?? undefined}
        shape="square"
        size="xs"
        class="shrink-0"
      />
      {label}
    </a>

    <!-- Hidden from signed-out readers, who have no profile to write to. -->
    {#if isAuthenticated()}
      <Tooltip side="top">
        <!-- size-6 is the 24px target WCAG 2.5.8 asks for; the negative margins keep the
             chip at badge height. -->
        <button
          type="button"
          disabled={pending}
          onclick={() => setAvoided(!avoided)}
          aria-label={avoided ? 'Stop avoiding this source' : 'Avoid this source'}
          aria-pressed={avoided}
          class={cn(
            '-my-1 -mr-1.5 inline-flex size-6 items-center justify-center rounded-full transition hover:text-foreground disabled:opacity-50',
            avoided && 'text-destructive',
          )}
        >
          {#if avoided}
            <RotateCcw class="size-3.5" aria-hidden="true" />
          {:else}
            <Ban class="size-3.5" aria-hidden="true" />
          {/if}
        </button>
        {#snippet content()}
          {avoided ? `Stop avoiding — show ${label} postings again` : 'Avoid this source'}
        {/snippet}
      </Tooltip>
    {/if}
  </span>

  {#if source === 'adzuna'}
    <!-- Required by Adzuna's API terms, not a courtesy credit — see the component. It sits
         in the provenance row beside the source chip, which is where a reader already looks
         to find out where a posting came from. -->
    <AdzunaAttribution {jobUrl} />
  {:else if source === 'devglobaljobs'}
    <!-- Required by Dev Global Jobs API terms: visible link to https://devglobaljobs.com
         wherever postings from their feed are shown. -->
    <DevGlobalJobsAttribution />
  {/if}

  {#if manuallyAdded}
    <Badge variant="secondary">Manually added</Badge>
  {/if}

  {#if failed}
    <!-- An alert, so the failure is announced: the button that caused it does not move
         focus, and a screen reader user would otherwise hear nothing at all. -->
    <span role="alert" class="text-destructive">Could not save that — try again.</span>
  {/if}
</div>
