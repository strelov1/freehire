<script lang="ts">
  import { page } from '$app/state';
  import DiscussionNewTopic from '$lib/components/community/DiscussionNewTopic.svelte';
  const slug = $derived(page.params.slug ?? "");
</script>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
  <DiscussionNewTopic subjectType="company" subjectSlug={slug} listPath={`/companies/${slug}/discussion`} />
</div>
