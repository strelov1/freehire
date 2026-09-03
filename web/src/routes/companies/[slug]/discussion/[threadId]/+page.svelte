<script lang="ts">
  import DiscussionThread from '$lib/components/community/DiscussionThread.svelte';
  import SubjectHeader from '$lib/components/community/SubjectHeader.svelte';
  let { data } = $props();
</script>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
  <SubjectHeader subject={data.subject} absence={data.absence} subjectType="company" subjectSlug={data.slug} class="mb-4" />
  <DiscussionThread
    thread={data.thread}
    initialReplies={data.replies}
    initialCursor={data.nextCursor}
    subjectType="company"
    subjectSlug={data.slug}
  />
</div>
