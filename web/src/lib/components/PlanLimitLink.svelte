<script lang="ts">
  import { resolve } from '$app/paths';
  import { currentUser } from '$lib/auth.svelte';
  import { Button } from '$lib/ui';

  // The one link every spent-daily-allowance message points to. `/my/plan` shows the
  // candidate exactly what they've used today and where to upgrade — see PlanView.svelte —
  // so every refusal banner sends them there instead of stating the limit and nothing else.
  //
  // A free reader is the one this refusal can actually convert, so they get the real CTA:
  // a button naming the price, not a line of underlined text easy to skim past. A paying
  // reader lands here too on rare occasion — Pro's own fair-use guard firing — and "Upgrade
  // to Pro" would be a wrong sentence to show somebody who already bought it, so they keep
  // the plain link instead.
  const isFree = $derived((currentUser()?.tier ?? 'free') === 'free');
</script>

{#if isFree}
  <Button href={resolve('/my/plan')} variant="primary" size="sm">Upgrade to Pro — $5/mo</Button>
{:else}
  <a href={resolve('/my/plan')} class="text-xs font-medium underline underline-offset-4">
    See your plan
  </a>
{/if}
