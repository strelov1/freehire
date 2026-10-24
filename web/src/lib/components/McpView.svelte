<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { sourceLogoUrl } from '#lib/logo.js';
  import { Button, EntityLogo, SectionLabel } from '#lib/ui/index.js';

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
  // name alone "Cursor" or "Grok" is ambiguous.
  // `directory` is a one-click install from the host's own connector directory. Our
  // Claude listing is the account connector, so it signs the visitor in.
  type Host = {
    id: string;
    label: string;
    steps: string[];
    snippet?: (url: string) => string;
    file?: string;
    directory?: string;
    domain: string;
  };
  const hosts: Host[] = [
    {
      id: 'claude',
      domain: 'claude.ai',
      label: 'Claude',
      directory: 'https://claude.ai/directory/manage/freehire',
      steps: [
        'Open Settings, then Connectors.',
        'Choose Add custom connector.',
        'Name it freehire, paste the address, and choose Add.',
        'Start a new chat, turn the freehire connector on, and ask a job question.',
      ],
    },
    {
      id: 'chatgpt',
      domain: 'chatgpt.com',
      label: 'ChatGPT',
      steps: [
        'Open Settings, then Apps & Connectors.',
        'Under Advanced settings, turn on Developer mode.',
        'Choose Create, name it freehire, and paste the address.',
        'In a new chat, pick freehire from the + menu and ask a job question.',
      ],
    },
    {
      id: 'grok',
      domain: 'grok.com',
      label: 'Grok Bot',
      steps: [
        'Open Connect apps in the sidebar.',
        'Add a custom app, name it freehire, and paste the address.',
        'If Grok Bot asks you to authorize, finish the sign-in in your browser.',
        'In a chat, type @ and pick freehire, then ask a job question.',
      ],
    },
    {
      id: 'claude-code',
      domain: 'claude.ai',
      label: 'Claude Code',
      steps: ['Run this once in your terminal, then ask Claude Code a job question.'],
      snippet: (u) => `claude mcp add --transport http freehire ${u}`,
    },
    {
      id: 'cursor',
      domain: 'cursor.com',
      label: 'Cursor',
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

<!-- Hero. -->
<section class="pb-8 pt-12 sm:pt-16">
  <SectionLabel text="mcp" />
  <h1 class="mt-6 max-w-3xl text-balance text-4xl font-semibold leading-none tracking-tighter sm:text-6xl">
    Search jobs right inside your AI assistant.
  </h1>
  <p class="mt-7 max-w-xl text-lg leading-relaxed text-muted-foreground">
    Free. No freehire account needed to search — sign in only if you want the assistant to save and track jobs for you.
  </p>
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
    <div hidden={h.id !== host} class="mt-8 max-w-2xl">
      <h3 class="text-xl font-semibold tracking-tight">Add it to {h.label}</h3>
      {#if h.directory}
        <div class="mt-4">
          <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external directory URL, not a SvelteKit route -->
          <Button href={h.directory} target="_blank" rel="noopener noreferrer" variant="primary" size="md">
            Add from the {h.label} directory ↗
          </Button>
          <p class="mt-3 text-sm leading-relaxed text-muted-foreground">
            One click, signs in with your freehire account. Or add it by hand:
          </p>
        </div>
      {/if}

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
