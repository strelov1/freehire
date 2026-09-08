<script lang="ts">
  import MentorsView from '$lib/components/MentorsView.svelte';
  import Seo from '$lib/components/Seo.svelte';
  import { page } from '$app/state';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const canonical = $derived(`${page.url.origin}/mentors`);
  const description =
    'Book half an hour with someone who actually works there, before you apply.';
</script>

<Seo title="Mentors · freehire" {description} {canonical} />

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <h1 class="text-2xl font-semibold">Talk to someone who works there</h1>
  <p class="text-muted-foreground mt-1 mb-6">{description}</p>

  <MentorsView mentors={data.mentors} options={data.options} filters={data.filters} />
</div>
