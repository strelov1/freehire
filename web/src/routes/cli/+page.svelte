<script lang="ts">
  import { page } from '$app/state';
  import { CLI_REPO, MCP_REPO } from '$lib/cliLinks';
  import CliView from '$lib/components/CliView.svelte';
  import Seo from '$lib/components/Seo.svelte';
  import { breadcrumbJsonLd, cliApplicationJsonLd, jsonLdScript } from '$lib/seo';

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/cli`);
  const jsonLd = $derived(
    jsonLdScript([
      cliApplicationJsonLd(origin, [CLI_REPO, MCP_REPO]),
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'CLI', url: canonical },
      ]),
    ])
  );
</script>

<Seo
  title="freehire CLI & MCP — search and track jobs from the terminal"
  description="A small Go CLI and an MCP server over the freehire job API, built so an AI agent or a script can search, open and track jobs — no browser. One API key drives both CLI and MCP."
  {canonical}
/>

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <CliView />
</div>
