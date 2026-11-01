<script lang="ts">
  import { page } from '$app/state';
  import { AGENTS_FAQ } from '#lib/agentsFaq.js';
  import AgentsLandingView from '#lib/components/AgentsLandingView.svelte';
  import Seo from '#lib/components/Seo.svelte';
  import { breadcrumbJsonLd, faqPageJsonLd, jsonLdScript } from '#lib/seo.js';

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
  title="freehire AI — Assistant, MCP and CLI"
  description="Use the freehire Assistant with Pro, connect your own AI assistant through MCP, or search from the CLI. Choose your setup and start your job search."
  {canonical}
/>

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <AgentsLandingView />
</div>
