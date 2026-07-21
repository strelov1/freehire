<script lang="ts">
  import DiscussionIndex from '$lib/components/community/DiscussionIndex.svelte';
  let { data } = $props();
  const basePath = $derived(`/companies/${data.slug}/discussion`);
</script>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
  <a class="crumb" href={`/companies/${data.slug}`}>← {data.slug}</a>
  <DiscussionIndex
    subjectType="company"
    subjectSlug={data.slug}
    {basePath}
    initialThreads={data.threads}
    initialCursor={data.nextCursor}
  />
</div>

<style>
  .crumb {
    display: inline-block;
    margin-bottom: 1rem;
    color: var(--muted-foreground, #6b7280);
    text-decoration: none;
    font-size: 0.85rem;
  }
</style>
