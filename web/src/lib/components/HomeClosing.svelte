<script lang="ts">
  import { resolve } from '$app/paths';
  import { HOME_FAQ } from '#lib/homeFaq.js';
  import { Button, SectionLabel } from '#lib/ui/index.js';
  import { CLI, GITHUB, ISSUES } from './homeLinks';
</script>

<!-- The landing page's closing sections, split out of HomeView: the CLI for agents, the
     open-source invitation and the FAQ. -->
<!-- CLI / agents — the same API from the terminal. Mirrors the open-source
     section's two-column copy + terminal figure. -->
<section class="border-t border-border py-16 sm:py-20">
  <SectionLabel text="For agents" />
  <div class="mt-6 grid gap-10 lg:grid-cols-2 lg:items-center">
    <div>
      <h2 class="max-w-md text-3xl font-semibold tracking-tight sm:text-4xl">
        Use it from the terminal. Built for agents.
      </h2>
      <p class="mt-5 max-w-md leading-relaxed text-muted-foreground">
        freehire is also a CLI, so an AI agent or a script can search, open and track jobs over the same API — no
        browser. Create an API key and it lives in
        <code class="font-mono text-foreground">~/.freehire/creds.json</code>; add
        <code class="font-mono text-foreground">--json</code> for machine-readable output.
      </p>
      <div class="mt-8 flex flex-wrap gap-3">
        <Button href={resolve('/cli')} variant="primary" size="lg">Explore the CLI</Button>
        <Button href={CLI} target="_blank" rel="noopener noreferrer" variant="ghost" size="lg">View on GitHub ↗</Button>
      </div>
    </div>

    <figure class="overflow-hidden rounded-xl border border-border bg-secondary/60 font-mono text-sm shadow-sm">
      <figcaption class="flex items-center gap-2 border-b border-border px-4 py-2.5 text-xs text-muted-foreground">
        <span class="size-2.5 rounded-full bg-muted-foreground/30"></span>
        terminal
      </figcaption>
      <pre class="overflow-x-auto p-4 leading-relaxed"><span class="text-muted-foreground"
          ># install — no Go needed</span
        >
curl -fsSL <span class="text-foreground">https://freehire.me/install.sh</span> | sh

<span class="text-muted-foreground"># authenticate once, then search &amp; track</span>
freehire auth login --token <span class="text-foreground">fhk_…</span>
freehire search <span class="text-foreground">"golang"</span> --remote --region eu
freehire save <span class="text-foreground">&lt;slug&gt;</span></pre>
    </figure>
  </div>
</section>

<!-- Open source / contribute. -->
<section class="border-t border-border py-16 sm:py-20">
  <SectionLabel text="Open source" />
  <div class="mt-6">
    <h2 class="max-w-md text-3xl font-semibold tracking-tight sm:text-4xl">Built in the open. Add your own source.</h2>
    <p class="mt-5 max-w-md leading-relaxed text-muted-foreground">
      Missing a company or source? Open an issue and we'll add it. A new ATS platform is one adapter. The backend is Go,
      the frontend is Svelte, the license is MIT — issues and pull requests welcome.
    </p>
    <div class="mt-8 flex flex-wrap gap-3">
      <Button href={ISSUES} target="_blank" rel="noopener noreferrer" variant="primary" size="lg">
        Add a source ↗
      </Button>
      <Button href={GITHUB} target="_blank" rel="noopener noreferrer" variant="ghost" size="lg">
        Star on GitHub ↗
      </Button>
    </div>
  </div>
</section>

<!-- FAQ. Visible answers mirror the FAQPage JSON-LD (see homeFaq.ts) so the
     structured data always matches what's on the page. -->
<section class="border-t border-border py-16 sm:py-20">
  <SectionLabel text="FAQ" />
  <h2 class="mt-6 max-w-md text-3xl font-semibold tracking-tight sm:text-4xl">Frequently asked questions.</h2>
  <dl class="mt-10 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-2">
    {#each HOME_FAQ as item (item.question)}
      <div class="bg-background p-6 sm:p-7">
        <dt class="text-lg font-semibold tracking-tight">{item.question}</dt>
        <dd class="mt-2 text-sm leading-relaxed text-muted-foreground">{item.answer}</dd>
      </div>
    {/each}
  </dl>
</section>
