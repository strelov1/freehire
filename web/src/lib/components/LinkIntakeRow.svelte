<script lang="ts">
  import { resolve } from '$app/paths';
  import { Link2 } from '@lucide/svelte';
  import type { LinkIntakeStep } from '#lib/linkIntake.js';
  import IntakeOutcome from './IntakeOutcome.svelte';

  // What HeaderSearch's panel says about a pasted vacancy link (`url`): the offer to open it,
  // the lookup in flight, and where the last run stopped (`step`). `onActivate` runs the
  // lookup, and hands the link in when we don't have it.
  let {
    url,
    busy,
    step,
    onActivate,
  }: { url: string; busy: boolean; step: LinkIntakeStep | null; onActivate: () => void } = $props();

  /** Where a signed-out visitor goes to hand this link in. The link rides along in the
   *  return path so it survives the round trip — otherwise signing in costs them the
   *  paste, and the whole point was that they had it in hand. */
  const signinHref = $derived.by(() => {
    const back = `/my/contributions?url=${encodeURIComponent(url)}`;
    return `${resolve('/signin')}?returnTo=${encodeURIComponent(back)}`;
  });
</script>

{#if busy}
  <span class="flex items-center gap-2 text-muted-foreground">
    <Link2 class="size-4 shrink-0" />
    Checking whether we have this one…
  </span>
{:else if step?.kind === 'signin'}
  <span class="flex flex-col gap-1">
    <span>We don't have this one yet.</span>
    <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- query string appended to a resolved path -->
    <a href={signinHref} class="font-medium underline underline-offset-4"> Sign in to send it to us → </a>
  </span>
{:else if step?.kind === 'outcome'}
  <IntakeOutcome resolved={step.resolved} />
{:else if step?.kind === 'error'}
  <span class="flex flex-col gap-1">
    <span>That didn't go through.</span>
    <button type="button" onclick={onActivate} class="self-start font-medium underline underline-offset-4">
      Try again
    </button>
  </span>
{:else}
  <button type="button" onclick={onActivate} class="flex w-full items-center gap-2 text-left">
    <Link2 class="size-4 shrink-0 text-muted-foreground" />
    <span class="min-w-0 flex-1 truncate">Open this job link</span>
    <kbd class="hidden shrink-0 rounded border border-border px-1.5 text-xs text-muted-foreground sm:inline"> ↵ </kbd>
  </button>
{/if}
