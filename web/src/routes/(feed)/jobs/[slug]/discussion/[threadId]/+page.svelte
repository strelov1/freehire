<script lang="ts">
  import DiscussionThread from '$lib/components/community/DiscussionThread.svelte';
  let { data } = $props();
</script>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
  <DiscussionThread
    thread={data.thread}
    initialReplies={data.replies}
    initialCursor={data.nextCursor}
    subjectType="job"
    subjectSlug={data.slug}
  />
</div>
