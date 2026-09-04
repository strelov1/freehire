<script lang="ts">
  import { isAuthenticated } from '$lib/auth.svelte';
  import { promptSignIn } from '$lib/signin';
  import CompanyFeedbackDialog from '$lib/components/CompanyFeedbackDialog.svelte';
  import DiscussionIndex from '$lib/components/community/DiscussionIndex.svelte';
  import SubjectHeader from '$lib/components/community/SubjectHeader.svelte';
  import { Button } from '$lib/ui';

  let { data } = $props();

  // Feedback is a rated review, distinct from the anonymous discussion below it —
  // it needs a signed-in caller up front rather than deferring to the report dialog,
  // the same gate JobView uses before its own.
  let showFeedback = $state(false);
  function openFeedback() {
    if (!isAuthenticated()) {
      promptSignIn();
      return;
    }
    showFeedback = true;
  }
</script>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
  <!-- The header is a link and the feedback button is an action, so they are
       siblings: nesting a button inside the link would swallow its click. -->
  <div class="mb-4 flex items-center gap-2">
    <SubjectHeader
      subject={data.subject}
      absence={data.absence}
      subjectType="company"
      subjectSlug={data.slug}
      class="min-w-0 flex-1"
    />
    <Button variant="outline" class="shrink-0" onclick={openFeedback}>Leave feedback</Button>
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
