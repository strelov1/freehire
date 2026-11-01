<script lang="ts">
  import { onDestroy } from 'svelte';
  import { resolve } from '$app/paths';
  import { MessageSquare, Plug, Terminal } from '@lucide/svelte';
  import { announce } from '#lib/announce.svelte.js';
  import { AGENTS_FAQ } from '#lib/agentsFaq.js';
  import { CLI_INSTALL, CLI_REPO, MCP_REPO } from '#lib/cliLinks.js';
  import { Button, TabStrip, tabStripId } from '#lib/ui/index.js';
  import McpSetup from './McpSetup.svelte';

  const tabs = [
    { id: 'assistant', label: 'freehire Assistant · Pro', icon: MessageSquare },
    { id: 'mcp', label: 'MCP', icon: Plug },
    { id: 'cli', label: 'CLI', icon: Terminal },
  ] as const;
  let active = $state<(typeof tabs)[number]['id']>('assistant');
  const uid = $props.id();
  const panelId = `${uid}-setup`;
  const commands = [
    { title: 'Install the CLI', note: 'Run this once in your terminal.', text: CLI_INSTALL },
    { title: 'Try your first search', note: 'Search is public. No account or API key needed.', text: 'freehire facets\nfreehire search "golang" --remote --region eu' },
    { title: 'Connect your account', note: 'Optional. Opens your browser so you can sign in to save and track jobs.', text: 'freehire auth login' },
  ];
  const prompts = [
    { title: 'Find your next role', text: 'Find remote senior Go jobs in Europe posted this week.' },
    { title: 'Explore an employer', text: 'What is Stripe hiring for right now?' },
    { title: 'Build a shortlist', text: 'Find jobs that fit my profile and save the best three.' },
  ];
  let copied = $state<string | null>(null);
  let copyTimer: ReturnType<typeof setTimeout> | undefined;
  onDestroy(() => clearTimeout(copyTimer));

  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text);
      copied = text;
      announce('Copied to the clipboard');
      clearTimeout(copyTimer);
      copyTimer = setTimeout(() => (copied = null), 1800);
    } catch {
      announce('Could not copy. Select the text and copy it manually.');
    }
  }
</script>

{#snippet copyButton(text: string, label: string)}
  <button type="button" onclick={() => copy(text)} aria-label={copied === text ? `Copied ${label}` : `Copy ${label}`}
    class="shrink-0 rounded-md border border-border px-3 py-1.5 text-xs font-medium text-foreground transition-colors hover:bg-secondary"
  >{copied === text ? 'Copied' : 'Copy'}</button>
{/snippet}

<section class="pb-9 pt-10 sm:pt-14">
  <h1 class="max-w-3xl text-balance text-4xl font-semibold leading-tight tracking-tight sm:text-6xl">Your job search, with AI.</h1>
  <p class="mt-5 max-w-2xl text-lg leading-relaxed text-muted-foreground">
    Use the freehire assistant or connect the tools you already work with.
    Find jobs, compare employers and keep your applications moving.
  </p>
</section>

<TabStrip tabs={tabs} {active} onSelect={(id) => (active = id)} label="Choose how to use freehire" {panelId} />
<div id={panelId} role="tabpanel" aria-labelledby={tabStripId(panelId, active)} tabindex="-1" class="min-w-0 py-8 sm:py-10">
  <div hidden={active !== 'assistant'}>
    <div class="grid items-center gap-10 lg:grid-cols-2 lg:gap-16">
      <div>
        <div class="flex flex-wrap items-center gap-3">
          <h2 class="text-2xl font-semibold tracking-tight sm:text-3xl">Start here. Stay in freehire.</h2>
          <span class="rounded-full bg-foreground px-2.5 py-1 text-xs font-semibold text-background">Pro</span>
        </div>
        <p class="mt-5 max-w-xl leading-relaxed text-muted-foreground">
          Your job-search assistant is already here. It uses your saved profile to find relevant roles,
          helps you compare them, and saves your shortlist to the same board you use on the site.
        </p>
        <ul class="mt-6 space-y-3 text-sm leading-relaxed text-muted-foreground">
          <li>Search with your skills and location preferences.</li>
          <li>Compare your skills with what employers are asking for.</li>
          <li>Save jobs and update application stages when you ask.</li>
        </ul>
        <div class="mt-7 flex flex-wrap gap-3">
          <Button href={resolve('/my/assistant/[[id]]', { id: undefined })} variant="primary" size="lg">Open assistant</Button>
          <Button href={resolve('/pricing')} variant="outline" size="lg">Explore Pro</Button>
        </div>
        <p class="mt-3 text-sm text-muted-foreground">Included in Pro. Sign in to use your profile and tracked jobs.</p>
      </div>
      <figure class="overflow-hidden rounded-xl border border-border bg-secondary/20">
        <figcaption class="flex items-center justify-between gap-3 border-b border-border px-5 py-4 text-sm">
          <span class="font-medium">freehire Assistant</span>
          <span class="text-xs text-muted-foreground">Example conversation</span>
        </figcaption>
        <div class="space-y-5 p-5 sm:p-6">
          <p class="ml-auto max-w-xs rounded-2xl rounded-br-sm bg-foreground px-4 py-3 text-sm leading-relaxed text-background">Find remote Go roles in Europe that fit my profile.</p>
          <div class="max-w-sm space-y-3 text-sm leading-relaxed">
            <p class="text-muted-foreground">I’ll use your saved skills and location preferences to narrow the search.</p>
            <div class="border-y border-border py-4">
              <p class="font-semibold">Senior Go Engineer</p>
              <p class="mt-1 text-muted-foreground">Remote · Europe · Backend</p>
              <p class="mt-3">Go and PostgreSQL overlap with your profile. Check the on-call requirements before applying.</p>
            </div>
          </div>
          <p class="ml-auto max-w-xs rounded-2xl rounded-br-sm bg-foreground px-4 py-3 text-sm text-background">Save this one to my board.</p>
          <p class="text-sm text-muted-foreground">Saved. You can find it on your tracking board.</p>
        </div>
      </figure>
    </div>
  </div>

  <div hidden={active !== 'mcp'}>
    <h2 class="text-2xl font-semibold tracking-tight sm:text-3xl">Bring freehire to your assistant.</h2>
    <p class="mb-7 mt-4 max-w-2xl leading-relaxed text-muted-foreground">
      Choose the assistant you already use and follow its connection steps.
      Search is free and needs no freehire account. Connect your account to save and track jobs.
    </p>
    <McpSetup />
  </div>

  <div hidden={active !== 'cli'}>
    <div class="grid gap-8 lg:grid-cols-[0.8fr_1.2fr] lg:gap-16">
      <div>
        <h2 class="text-2xl font-semibold tracking-tight sm:text-3xl">Work from your terminal.</h2>
        <p class="mt-4 max-w-xl leading-relaxed text-muted-foreground">
          Search directly with the CLI, or let Codex, Claude Code or another local agent run it for you.
          A local agent can work with the CV and notes on your machine.
        </p>
        <p class="mt-4 text-sm leading-relaxed text-muted-foreground">Start with a public search. Sign in when you want to save jobs or work with your account.</p>
        <div class="mt-6"><Button href={resolve('/cli')} variant="outline" size="md">Full CLI guide</Button></div>
      </div>
      <ol class="min-w-0 space-y-6">
        {#each commands as command, i (command.title)}
          <li class="min-w-0">
            <h3 class="font-semibold">{i + 1}. {command.title}</h3>
            <p class="mt-1 text-sm leading-relaxed text-muted-foreground">{command.note}</p>
            <figure class="mt-3 overflow-hidden rounded-xl border border-border bg-secondary/30">
              <figcaption class="flex items-center justify-between gap-3 border-b border-border px-4 py-2 text-xs text-muted-foreground">
                <span>Terminal</span>{@render copyButton(command.text, command.title.toLowerCase())}
              </figcaption>
              <pre class="overflow-x-auto p-4 font-mono text-sm leading-relaxed">{command.text}</pre>
            </figure>
          </li>
        {/each}
      </ol>
    </div>
  </div>
</div>

<section class="border-t border-border py-10 sm:py-12">
  <h2 class="text-2xl font-semibold tracking-tight">Try asking</h2>
  <p class="mt-3 text-sm text-muted-foreground">Copy a prompt into your assistant. Saving a shortlist needs a connected account.</p>
  <div class="mt-6 divide-y divide-border">
    {#each prompts as prompt (prompt.title)}
      <div class="flex items-start justify-between gap-4 py-5">
        <div><h3 class="text-sm font-semibold">{prompt.title}</h3><p class="mt-2 max-w-2xl leading-relaxed text-muted-foreground">{prompt.text}</p></div>
        {@render copyButton(prompt.text, prompt.title.toLowerCase())}
      </div>
    {/each}
  </div>
</section>

<section class="border-t border-border py-10 sm:py-12">
  <h2 class="text-2xl font-semibold tracking-tight">A few things to know</h2>
  <div class="mt-5 divide-y divide-border">
    {#each AGENTS_FAQ as item (item.question)}
      <details class="py-4">
        <summary class="cursor-pointer font-medium leading-relaxed">{item.question}</summary>
        <p class="mt-3 max-w-3xl text-sm leading-relaxed text-muted-foreground">{item.answer}</p>
      </details>
    {/each}
  </div>
</section>

<details class="border-t border-border py-6">
  <summary class="cursor-pointer font-medium">Developer details</summary>
  <p class="mt-4 max-w-3xl text-sm leading-relaxed text-muted-foreground">
    Building your own integration? Use the API reference and machine-readable documentation.
    The CLI guide also covers the local MCP server and agent skills.
  </p>
  <div class="mt-5 flex flex-wrap gap-3">
    <Button href={resolve('/docs/api')} variant="outline" size="sm">API reference</Button>
    <Button href="https://freehire.me/llms.txt" target="_blank" rel="noopener noreferrer" variant="outline" size="sm">llms.txt</Button>
    <Button href="https://freehire.me/openapi.yaml" target="_blank" rel="noopener noreferrer" variant="outline" size="sm">OpenAPI schema</Button>
    <Button href={resolve('/cli') + '#mcp'} variant="outline" size="sm">Local MCP setup</Button>
    <Button href={CLI_REPO} target="_blank" rel="noopener noreferrer" variant="ghost" size="sm">CLI source</Button>
    <Button href={MCP_REPO} target="_blank" rel="noopener noreferrer" variant="ghost" size="sm">MCP source</Button>
  </div>
</details>
