<script lang="ts">
  import { currentUser } from '$lib/auth.svelte';
  import ReferralReviewView from '$lib/components/ReferralReviewView.svelte';

  // The nav link is moderator-only, but a direct visit needs its own guard. This is a
  // UI affordance only — the endpoint re-checks the moderator role on every request.
  const isModerator = $derived(['moderator', 'admin'].includes(currentUser()?.role ?? ''));
</script>

<svelte:head>
  <title>Referral review — freehire</title>
</svelte:head>

<div class="max-w-3xl">
  <div class="mb-4 flex items-center gap-3">
    <a href="/my/referrals" class="text-sm text-muted-foreground hover:text-foreground">← Referrals</a>
    <h1 class="text-lg font-semibold tracking-tight">Referral offers · review</h1>
  </div>
  {#if isModerator}
    <ReferralReviewView />
  {:else}
    <p class="py-12 text-center text-sm text-muted-foreground">Moderators only.</p>
  {/if}
</div>
