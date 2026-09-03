<script lang="ts">
  import { page } from '$app/state';
  import DiscussionFeed from '$lib/components/community/DiscussionFeed.svelte';
  import Seo from '$lib/components/Seo.svelte';
  import { breadcrumbJsonLd, jsonLdScript } from '$lib/seo';

  let { data } = $props();

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/discussions`);
  const jsonLd = $derived(
    jsonLdScript([
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'Discussions', url: canonical },
      ]),
    ]),
  );
</script>

<Seo
  title="Discussions — freehire"
  description="What job seekers are asking about companies and vacancies on freehire: is this posting real, does this employer reply, what is the process actually like."
  {canonical}
/>

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
  <header class="mb-5">
    <h1 class="text-2xl font-semibold tracking-tight">Discussions</h1>
    <p class="mt-1 text-sm text-muted-foreground">
      Every open topic across companies and vacancies, newest first. Anonymous — people
      post under a pseudonym, not their name. To start one, open the company or vacancy
      it is about.
    </p>
  </header>

  <DiscussionFeed
    initialThreads={data.threads}
    initialCursor={data.nextCursor}
    failed={data.failed}
  />
</div>
