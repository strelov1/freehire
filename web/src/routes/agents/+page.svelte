<script lang="ts">
  import { page } from '$app/state';
  import { AGENTS_FAQ } from '$lib/agentsFaq';
  import AgentsLandingView from '$lib/components/AgentsLandingView.svelte';
  import Seo from '$lib/components/Seo.svelte';
  import { breadcrumbJsonLd, faqPageJsonLd, jsonLdScript } from '$lib/seo';

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/agents`);
  const jsonLd = $derived(
    jsonLdScript([
      faqPageJsonLd(AGENTS_FAQ),
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'Agents', url: canonical },
      ]),
    ])
  );
</script>

<Seo
  title="freehire for AI agents — CLI, MCP and ChatGPT"
  description="Point your AI agent at the freehire job catalogue. A local harness driving the CLI reaches the whole surface; an MCP host or ChatGPT covers most of it. Search needs no API key."
  {canonical}
/>

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <AgentsLandingView />
</div>
