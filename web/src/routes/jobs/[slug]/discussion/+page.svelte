<script lang="ts">
  import DiscussionIndex from '$lib/components/community/DiscussionIndex.svelte';
  import SubjectHeader from '$lib/components/community/SubjectHeader.svelte';
  let { data } = $props();
</script>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
  <SubjectHeader subject={data.subject} absence={data.absence} subjectType="job" subjectSlug={data.slug} class="mb-4" />
  <DiscussionIndex
    subjectType="job"
    subjectSlug={data.slug}
    initialThreads={data.threads}
    initialCursor={data.nextCursor}
  />
</div>
