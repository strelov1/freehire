<script lang="ts">
  import { api } from '#lib/api.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { signinUrl } from '#lib/signin.js';
  import type { CommunityThread } from '#lib/types.js';
  import { Button, EmptyState, Skeleton } from '#lib/ui/index.js';
  import DiscussionIndex from './community/DiscussionIndex.svelte';

  // The job's discussion, as the job page's tab (it used to be a page of its own at
  // /jobs/<slug>/discussion, which now redirects here). Members-only, like the page was:
  // a signed-out reader gets the way in, not the threads. Read on first activation, in the
  // browser — the discussion is kept out of the index (robots.ts), so nothing is lost by
  // not server-rendering it. JobView keys this on the job, so a new job remounts it.
  let { slug, active }: { slug: string; active: boolean } = $props();

  let threads = $state.raw<CommunityThread[] | null>(null);
  let cursor = $state<string | undefined>();
  let failed = $state(false);
  // Plain, not $state: the effect writes it, and a reactive read would make the effect
  // its own dependency. Retry clears it together with `failed`, which re-runs the effect.
  let requested = false;

  // Back to this tab after sign-in, not to the job's Description.
  function signInHere() {
    const returnTo = `${resolve('/jobs/[slug]', { slug })}?tab=discussion`;
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- signinUrl() wraps resolve('/signin'); the rule can't see through the appended query
    void goto(signinUrl({ returnTo, mode: 'login' }));
  }

  function retry() {
    requested = false;
    failed = false;
  }

  $effect(() => {
    // `failed` is read before `requested` so it stays tracked after a failure: with
    // `requested` first the guard short-circuits and Retry would never re-run this.
    if (!active || failed || requested || !isAuthenticated()) return;
    requested = true;
    api
      .listThreads('job', slug)
      .then((r) => {
        threads = r.threads;
        cursor = r.nextCursor;
      })
      .catch(() => (failed = true));
  });
</script>

{#snippet signIn()}
  <Button onclick={signInHere}>Sign in</Button>
{/snippet}

{#snippet retryButton()}
  <Button variant="secondary" onclick={retry}>Retry</Button>
{/snippet}

{#if !isAuthenticated()}
  <EmptyState title="Sign in to read and join the discussion." variant="muted" action={signIn} />
{:else if failed}
  <EmptyState title="Couldn't load the discussion." variant="destructive" action={retryButton} />
{:else if threads}
  <DiscussionIndex subjectType="job" subjectSlug={slug} initialThreads={threads} initialCursor={cursor} />
{:else if active}
  <div class="flex flex-col gap-3">
    <Skeleton class="h-10 w-full" />
    <Skeleton class="h-16 w-full" />
  </div>
{/if}
