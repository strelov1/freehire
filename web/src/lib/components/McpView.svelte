<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { siClaude, siCursor, type SimpleIcon } from 'simple-icons';
  import { sourceLogoUrl } from '#lib/logo.js';
  import { EntityLogo, SectionLabel } from '#lib/ui/index.js';

  // The two hosted MCP servers. They are separate on purpose: a host only offers
  // sign-in when the server answers 401, so the anonymous one can never lead to the
  // account one — the visitor picks which connector they are adding, here.
  type Mode = 'anonymous' | 'account';
  const servers = $derived({
    anonymous: {
      url: `${page.url.origin}/api/v1/mcp`,
      label: 'No sign-in',
      desc: 'Search jobs and employers, read postings, salary ranges and market breakdowns. Read-only.',
    },
    account: {
      url: `${page.url.origin}/api/v1/mcp/account`,
      label: 'With your account',
      desc: 'Everything above, plus your profile, your skills against the market, and saving, applying and moving jobs on your board.',
    },
  });
  let mode = $state<Mode>('anonymous');
  const url = $derived(servers[mode].url);

  // One tile per host. `domain` is what the logo proxy resolves the mark from — by
  // name alone "Cursor" or "Grok" is ambiguous. `directory` is a one-click install from the host's own directory, drawn in that
  // host's brand colour and mark (from simple-icons, like every other brand mark on the
  // site) so the button reads as the host's install, not ours.
  type Host = {
    id: string;
    label: string;
    domain: string;
    directory?: { url: string; brand: SimpleIcon; note: string };
    steps: string[];
    file?: string;
    snippet?: (url: string) => string;
  };
  const hosts: Host[] = [
    {
      id: 'claude',
      label: 'Claude',
      domain: 'claude.ai',
      // Our Claude listing is the account connector, so it signs the visitor in.
      directory: {
        url: 'https://claude.ai/directory/manage/freehire',
        brand: siClaude,
        note: 'One click, signs in with freehire',
      },
      steps: [
        'Open Settings, then Connectors.',
        'Choose Add custom connector.',
        'Name it freehire, paste the address, and choose Add.',
        'Start a new chat, turn the freehire connector on, and ask a job question.',
      ],
    },
    {
      id: 'chatgpt',
      label: 'ChatGPT',
      domain: 'chatgpt.com',
      steps: [
        'Open Settings, then Apps & Connectors.',
        'Under Advanced settings, turn on Developer mode.',
        'Choose Create, name it freehire, and paste the address.',
        'In a new chat, pick freehire from the + menu and ask a job question.',
      ],
    },
    {
      id: 'grok',
      label: 'Grok Bot',
      domain: 'grok.com',
      steps: [
        'Open Connect apps in the sidebar.',
        'Add a custom app, name it freehire, and paste the address.',
        'If Grok Bot asks you to authorize, finish the sign-in in your browser.',
        'In a chat, type @ and pick freehire, then ask a job question.',
      ],
    },
    {
      id: 'claude-code',
      label: 'Claude Code',
      domain: 'claude.ai',
      steps: ['Run this once in your terminal, then ask Claude Code a job question.'],
      snippet: (u) => `claude mcp add --transport http freehire ${u}`,
    },
    {
      id: 'cursor',
      label: 'Cursor',
      domain: 'cursor.com',
      directory: {
        url: 'https://cursor.directory/plugins/freehire-mcp',
        brand: siCursor,
        note: 'Listed on cursor.directory',
      },
      steps: ['Add this to your MCP config, then ask the agent a job question.'],
      file: '~/.cursor/mcp.json',
      snippet: (u) => `{
  "mcpServers": {
    "freehire": { "url": "${u}" }
  }
}`,
    },
  ];
  let host = $state('claude');

  // Things to ask, grouped by what the visitor is after. Each one is answerable by
  // the anonymous server's tools alone, except the last group, which needs the account.
  const prompts = [
    {
      title: 'Find jobs',
      items: [
        'Remote senior Go jobs in Europe posted this week',
        'Junior data analyst roles in Berlin with a stated salary',
      ],
    },
    {
      title: 'Look at an employer',
      items: ['What is Stripe hiring for right now?', 'Fintech companies hiring backend engineers in Poland'],
    },
    {
      title: 'Read the market',
      items: ['What does a senior frontend role pay in Germany?', 'Which skills do DevOps postings ask for most?'],
    },
    {
      title: 'With your account',
      items: ['Find jobs that fit my profile and save the best three', 'Which of my applications have gone quiet?'],
    },
  ];

  // The hero's sample conversation. Illustrative, so no real employer is named.
  const heroJobs = [
    { title: 'Senior Go Engineer — payments platform', meta: 'Remote · EU · €85–100k' },
    { title: 'Senior Backend Engineer (Go, Kafka)', meta: 'Remote · Germany, Netherlands' },
    { title: 'Staff Go Developer — observability', meta: 'Remote · Europe · posted 2 days ago' },
  ];

  // One copy confirmation at a time, keyed by the text that was copied.
  let copied = $state<string | null>(null);
  let copyTimer: ReturnType<typeof setTimeout> | undefined;
  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text);
      copied = text;
      clearTimeout(copyTimer);
      copyTimer = setTimeout(() => (copied = null), 1600);
    } catch {
      // Clipboard can be blocked (no permission / insecure context) — the text is
      // plainly visible to select by hand, so a failed copy needs no fallback.
    }
  }
</script>

{#snippet copyButton(text: string, label: string)}
  <button
    type="button"
    onclick={() => copy(text)}
    aria-label={`Copy ${label}`}
    class="shrink-0 rounded-md border border-border px-2 py-0.5 font-sans text-xs font-medium text-muted-foreground transition-colors hover:text-foreground"
  >
    {copied === text ? 'copied ✓' : 'copy'}
  </button>
{/snippet}

<!-- Hero. Left: the pitch. Right: what a conversation with the connector looks like. -->
<section class="grid items-center gap-10 pb-10 pt-12 sm:pt-16 lg:grid-cols-2">
  <div>
    <SectionLabel text="mcp" />
    <h1 class="mt-6 max-w-3xl text-balance text-4xl font-semibold leading-none tracking-tighter sm:text-6xl">
      Search jobs right inside your AI assistant.
    </h1>
    <p class="mt-7 max-w-xl text-lg leading-relaxed text-muted-foreground">
      Free. No freehire account needed to search — sign in only if you want the assistant to save and track jobs for
      you.
    </p>
  </div>

  <figure class="overflow-hidden rounded-xl border border-border bg-secondary/60 text-sm shadow-sm">
    <figcaption class="flex items-center gap-2 border-b border-border px-4 py-2.5 text-xs text-muted-foreground">
      <span class="size-2.5 rounded-full bg-muted-foreground/30"></span>
      Your assistant · freehire connector
    </figcaption>
    <div class="flex flex-col gap-3 p-4 leading-relaxed">
      <div class="max-w-xs self-end rounded-2xl rounded-br-sm bg-foreground px-3.5 py-2 text-background">
        Remote senior Go jobs in Europe, posted this week
      </div>
      <p class="text-xs text-muted-foreground">Used freehire · Search jobs</p>
      <div class="max-w-md rounded-2xl rounded-bl-sm border border-border bg-background px-3.5 py-2.5">
        <p class="text-muted-foreground">Here are the strongest matches:</p>
        {#each heroJobs as job (job.title)}
          <div class="mt-2.5">
            <p class="font-medium text-foreground">{job.title}</p>
            <p class="text-xs text-muted-foreground">{job.meta}</p>
          </div>
        {/each}
      </div>
      <div class="max-w-xs self-end rounded-2xl rounded-br-sm bg-foreground px-3.5 py-2 text-background">
        Save the first one to my board
      </div>
    </div>
  </figure>
</section>

<!-- Setup: which assistant, then which connector. Every host's panel stays in the
     HTML, `hidden` when unselected, so crawlers and agents read all of them. -->
<section class="rounded-2xl border border-border bg-secondary/20 p-5 sm:p-8">
  <h2 class="text-sm text-muted-foreground">Pick your assistant</h2>
  <div class="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5" role="group" aria-label="Assistant">
    {#each hosts as h (h.id)}
      <button
        type="button"
        aria-pressed={host === h.id}
        onclick={() => (host = h.id)}
        class={[
          'flex flex-col items-center gap-3 rounded-xl border-2 px-3 py-5 transition-colors',
          host === h.id ? 'border-foreground bg-secondary/60' : 'border-border bg-background hover:bg-secondary/40',
        ]}
      >
        <EntityLogo name={h.label} src={sourceLogoUrl(h.label, h.domain) ?? undefined} size="lg" shape="square" />
        <span class="text-base font-medium text-foreground">{h.label}</span>
      </button>
    {/each}
  </div>

  {#each hosts as h (h.id)}
    <div hidden={h.id !== host} class="mt-8">
      <div class="flex flex-wrap items-start justify-between gap-4">
        <h3 class="text-xl font-semibold tracking-tight">Add it to {h.label}</h3>
        {#if h.directory}
          <div class="flex flex-col items-end gap-1.5">
            <!-- eslint-disable svelte/no-navigation-without-resolve -- external directory URL, not a SvelteKit route -->
            <a
              href={h.directory.url}
              target="_blank"
              rel="noopener noreferrer"
              class="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90"
              style:background-color="#{h.directory.brand.hex}"
            >
              <svg viewBox="0 0 24 24" class="size-4 fill-current" aria-hidden="true"
                ><path d={h.directory.brand.path} /></svg
              >
              Add to {h.label} ↗
            </a>
            <!-- eslint-enable svelte/no-navigation-without-resolve -->
            <span class="text-xs text-muted-foreground">{h.directory.note}</span>
          </div>
        {/if}
      </div>

      <div class="max-w-2xl">
      <div class="mt-4 flex flex-wrap gap-2" role="group" aria-label="Connector">
        {#each Object.entries(servers) as [id, server] (id)}
          <button
            type="button"
            aria-pressed={mode === id}
            onclick={() => (mode = id as Mode)}
            class={[
              'rounded-full border px-3 py-1 text-sm font-medium transition-colors',
              mode === id
                ? 'border-foreground bg-foreground text-background'
                : 'border-border text-muted-foreground hover:text-foreground',
            ]}
          >
            {server.label}
          </button>
        {/each}
      </div>
      <p class="mt-2 text-sm leading-relaxed text-muted-foreground">{servers[mode].desc}</p>

      <div class="mt-4 flex items-center gap-3 rounded-lg border border-border bg-background px-3 py-2">
        <code class="min-w-0 flex-1 overflow-x-auto whitespace-nowrap font-mono text-sm">{url}</code>
        {@render copyButton(url, 'connector address')}
      </div>

      <div class="mt-5">
        <ol class="space-y-3">
          {#each h.steps as step, i (i)}
            <li class="flex gap-4">
              <span
                class="flex size-6 shrink-0 items-center justify-center rounded-full bg-foreground text-xs font-semibold text-background"
                >{i + 1}</span
              >
              <span class="leading-relaxed text-muted-foreground">{step}</span>
            </li>
          {/each}
        </ol>
        {#if h.snippet}
          {@const text = h.snippet(url)}
          <figure class="mt-5 overflow-hidden rounded-xl border border-border bg-secondary/60 shadow-sm">
            <figcaption
              class="flex items-center gap-2 border-b border-border px-4 py-2.5 text-xs text-muted-foreground"
            >
              <span class="font-mono">{h.file ?? 'terminal'}</span>
              <span class="ml-auto">{@render copyButton(text, `${h.label} setup`)}</span>
            </figcaption>
            <pre class="overflow-x-auto p-4 font-mono text-sm leading-relaxed">{text}</pre>
          </figure>
        {/if}
        {#if mode === 'account'}
          <p class="mt-5 text-sm leading-relaxed text-muted-foreground">
            The first time it is used, your assistant opens a freehire sign-in page and asks you to approve the
            connection.
          </p>
        {/if}
      </div>
      </div>
    </div>
  {/each}
</section>

<!-- Things to ask. -->
<section class="border-t border-border py-14 sm:py-16">
  <h2 class="text-2xl font-semibold tracking-tight">Things to ask</h2>
  <div class="mt-6 grid gap-4 sm:grid-cols-2">
    {#each prompts as group (group.title)}
      <div class="rounded-xl border border-border bg-secondary/40 p-4">
        <h3 class="text-sm font-semibold text-foreground">{group.title}</h3>
        <ul class="mt-3 space-y-2">
          {#each group.items as item (item)}
            <li class="text-sm leading-relaxed text-muted-foreground">“{item}”</li>
          {/each}
        </ul>
      </div>
    {/each}
  </div>
</section>

<!-- What it can reach. -->
<section class="border-t border-border py-14 sm:py-16">
  <h2 class="text-2xl font-semibold tracking-tight">What the connector can reach</h2>
  <ul class="mt-6 max-w-2xl list-disc space-y-2 pl-5 leading-relaxed text-muted-foreground">
    <li>Public job postings, employer profiles and market figures — the same catalogue the site shows.</li>
    <li>Without sign-in it cannot see or change anything of yours.</li>
    <li>
      With your account it reads your profile and tracked jobs, and saves, marks applied or moves jobs on your board —
      never another person's.
    </li>
  </ul>
  <p class="mt-8 text-sm leading-relaxed text-muted-foreground">
    Prefer a local server or the terminal? The <a
      href={resolve('/cli')}
      class="font-medium text-foreground underline-offset-4 hover:underline">CLI and npx MCP server</a
    > use the same API.
  </p>
</section>
