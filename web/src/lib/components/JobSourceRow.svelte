<script lang="ts">
  import { Ban, RotateCcw } from '@lucide/svelte';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { sourceLabel } from '$lib/facets';
  import { filterHref } from '$lib/enrichment';
  import { SOURCE_LOGO_DOMAINS, sourceLogoUrl } from '$lib/logo';
  import { profileStore } from '$lib/profile.svelte';
  import { syncProfileAlert } from '$lib/profileAlertSync';
  import { promptSignIn } from '$lib/signin';
  import { Badge, EntityLogo } from '$lib/ui';
  import AdzunaAttribution from './AdzunaAttribution.svelte';

  // The job page's provenance row: where this posting came from, and the control to stop
  // seeing that source at all. Lifted out of JobView — which is 1,100 lines — because it is
  // the one part of that sidebar with its own state and its own write, and because a
  // control that edits a profile deserves a test that does not have to mount a job page.
  let {
    source,
    jobUrl,
    manuallyAdded = false,
  }: { source: string; jobUrl: string; manuallyAdded?: boolean } = $props();

  const label = $derived(sourceLabel(source));

  // Compared lowercased: the server normalises excluded_sources to lowercase on save
  // (userprofile.normalizeExcludedSet), and nothing guarantees a caller hands us the same
  // casing the list was written in.
  const avoided = $derived(
    (profileStore.profile?.excluded_sources ?? []).some(
      (s) => s.toLowerCase() === source.toLowerCase()
    )
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

<div
  class="flex flex-wrap items-center justify-center gap-x-3 gap-y-1.5 border-t border-border pt-4 text-xs text-muted-foreground first:border-t-0 first:pt-0"
>
  <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- internal /jobs filter link from filterHref; query-only, no route to resolve -->
  <a href={filterHref('source', source)} class="inline-flex items-center gap-1.5">
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
    <Badge variant="outline" class="transition-colors hover:bg-accent hover:text-foreground">
      {label}
    </Badge>
  </a>

  {#if source === 'adzuna'}
    <!-- Required by Adzuna's API terms, not a courtesy credit — see the component. It sits
         in the provenance row beside the source chip, which is where a reader already looks
         to find out where a posting came from. -->
    <AdzunaAttribution {jobUrl} />
  {/if}

  {#if manuallyAdded}
    <Badge variant="secondary">Manually added</Badge>
  {/if}

  <!-- Hidden from signed-out readers, who have no profile to write to. -->
  {#if isAuthenticated()}
    <button
      type="button"
      disabled={pending}
      onclick={() => setAvoided(!avoided)}
      title={avoided
        ? `Show ${label} postings again`
        : `Hide every ${label} posting from your search and alerts`}
      class="inline-flex items-center gap-1.5 rounded-full border border-border bg-background px-2.5 py-1 text-xs font-medium text-muted-foreground transition-colors hover:border-muted-foreground/40 disabled:opacity-50"
    >
      {#if avoided}
        <RotateCcw class="size-3.5" aria-hidden="true" /> Stop avoiding
      {:else}
        <Ban class="size-3.5" aria-hidden="true" /> Avoid this source
      {/if}
    </button>
  {/if}

  {#if failed}
    <span class="text-destructive">Could not save that — try again.</span>
  {/if}
</div>
