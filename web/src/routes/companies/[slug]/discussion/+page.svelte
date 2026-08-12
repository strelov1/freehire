<script lang="ts">
  import { resolve } from '$app/paths';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import CompanyFeedbackDialog from '$lib/components/CompanyFeedbackDialog.svelte';
  import DiscussionIndex from '$lib/components/community/DiscussionIndex.svelte';
  import { Button } from '$lib/ui';

  let { data } = $props();

  // Feedback is a rated review, distinct from the anonymous discussion below it —
  // it needs a signed-in caller up front rather than deferring to the dialog, the
  // same gate JobView uses before its own report dialog.
  let showFeedback = $state(false);
  function openFeedback() {
    if (!isAuthenticated()) {
      openAuthDialog('login');
      return;
    }
    showFeedback = true;
  }
</script>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
  <div class="top-row">
    <a class="crumb" href={resolve('/companies/[slug]', { slug: data.slug })}>← {data.slug}</a>
    <Button variant="outline" onclick={openFeedback}>Leave feedback</Button>
  </div>
  <DiscussionIndex
    subjectType="company"
    subjectSlug={data.slug}
    initialThreads={data.threads}
    initialCursor={data.nextCursor}
  />
</div>

{#if showFeedback}
  <CompanyFeedbackDialog slug={data.slug} onClose={() => (showFeedback = false)} />
{/if}

<style>
  .top-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    margin-bottom: 1rem;
  }
  .crumb {
    display: inline-block;
    color: var(--muted-foreground, #6b7280);
    text-decoration: none;
    font-size: 0.85rem;
  }
</style>
