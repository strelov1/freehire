<script lang="ts">
  import { page } from '$app/state';
  import { siClaude, siCursor, type SimpleIcon } from 'simple-icons';
  import { sourceLogoUrl } from '#lib/logo.js';
  import { announce } from '#lib/announce.svelte.js';
  import { glyphColorFor } from '#lib/markColor.js';
  import { EntityLogo } from '#lib/ui/index.js';

  // Pick an assistant, then follow its steps — the setup half of /mcp, shared with the
  // onboarding tour. `compact` draws smaller host tiles for a narrow column;
  // `initialMode` lets a signed-in surface open on the account connector.
  let { compact = false, initialMode = 'anonymous' }: { compact?: boolean; initialMode?: 'anonymous' | 'account' } =
    $props();

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
      desc: 'Search jobs and employers, plus your profile, your skills against the market, and saving, applying and moving jobs on your board.',
    },
  });
  // svelte-ignore state_referenced_locally
  let mode = $state<Mode>(initialMode);
  const url = $derived(servers[mode].url);

  // One tile per host. `domain` is what the logo proxy resolves the mark from — by
  // name alone "Cursor" or "Grok" is ambiguous. `directory` is a one-click install from the host's own directory, drawn in that
  // host's brand colour and mark (from simple-icons, like every other brand mark on the
  // site) so the button reads as the host's install, not ours. The label's ink is picked
  // per fill: white on Claude's terracotta measured 3.1:1, the near-black it gets instead 5.7:1.
  type Host = {
    id: string;
    label: string;
    domain: string;
    // Our own copy of the mark, where the logo proxy resolves the wrong one: for Grok it
    // returns xAI's wordmark rather than Grok Bot's round mark.
    logo?: string;
    // A black-on-transparent mark, inverted on the dark theme where it would vanish.
    mono?: boolean;
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
      mono: true,
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
      logo: '/brands/grok.png',
      mono: true,
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
      // Gemini's own web app has no way to add an MCP server; its CLI does.
      id: 'gemini-cli',
      label: 'Gemini CLI',
      domain: 'gemini.google.com',
      steps: ['Run this once in your terminal, then ask Gemini CLI a job question.'],
      snippet: (u) => `gemini mcp add --transport http freehire ${u}`,
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
  const selected = $derived(hosts.find((h) => h.id === host));

  // One copy confirmation at a time, keyed by the text that was copied.
  let copied = $state<string | null>(null);
  let copyTimer: ReturnType<typeof setTimeout> | undefined;
  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text);
      copied = text;
      announce('Copied to the clipboard');
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
    aria-label={copied === text ? `Copied ${label}` : `Copy ${label}`}
    class="shrink-0 rounded-md border border-border px-2 py-0.5 font-sans text-xs font-medium text-muted-foreground transition-colors hover:text-foreground"
  >
    {copied === text ? 'copied ✓' : 'copy'}
  </button>
{/snippet}

<!-- Setup: which assistant, then which connector. Every host's panel stays in the
     HTML, `hidden` when unselected, so crawlers and agents read all of them. -->
<section class={compact ? '' : 'rounded-2xl border border-border bg-secondary/20 p-5 sm:p-8'}>
  <div class={['flex flex-wrap items-end justify-between gap-3', compact && 'min-h-14']}>
    <h2 class="text-sm text-muted-foreground">Pick your assistant</h2>
    {#if compact && selected?.directory}
      <!-- The selected host's one-click install, lifted above the picker: it needs no
           address or steps at all, so it leads whenever the host has one. -->
      {@const d = selected.directory}
      <div class="flex flex-col items-end gap-1.5">
        <!-- eslint-disable svelte/no-navigation-without-resolve -- external directory URL, not a SvelteKit route -->
        <a
          href={d.url}
          target="_blank"
          rel="noopener noreferrer"
          class="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium transition-opacity hover:opacity-90"
          style:background-color="#{d.brand.hex}"
          style:color={glyphColorFor(`#${d.brand.hex}`)}
        >
          <svg viewBox="0 0 24 24" class="size-4 fill-current" aria-hidden="true"><path d={d.brand.path} /></svg>
          Add to {selected?.label} ↗
        </a>
        <!-- eslint-enable svelte/no-navigation-without-resolve -->
        <span class="text-xs text-muted-foreground">{d.note}</span>
      </div>
    {/if}
  </div>
  <div class={['mt-4 grid gap-3', compact ? 'grid-cols-3 gap-2 sm:grid-cols-6' : 'grid-cols-2 sm:grid-cols-3 lg:grid-cols-5']} role="group" aria-label="Assistant">
    {#each hosts as h (h.id)}
      <button
        type="button"
        aria-pressed={host === h.id}
        onclick={() => (host = h.id)}
        class={[
          'flex flex-col items-center rounded-xl border-2 transition-colors',
          compact ? 'gap-1.5 px-1 py-2.5' : 'gap-3 px-3 py-5',
          host === h.id ? 'border-foreground bg-secondary/60' : 'border-border bg-background hover:bg-secondary/40',
        ]}
      >
        <EntityLogo
          name={h.label}
          src={h.logo ?? sourceLogoUrl(h.label, h.domain) ?? undefined}
          size={compact ? 'md' : 'lg'}
          shape="square"
          class={h.mono ? 'dark:invert' : undefined}
        />
        <span class={['font-medium text-foreground', compact ? 'whitespace-nowrap text-xs' : 'text-base']}>{h.label}</span>
      </button>
    {/each}
  </div>

  {#each hosts as h (h.id)}
    <div hidden={h.id !== host} class={compact ? 'mt-6' : 'mt-8'}>
      <div class="flex flex-wrap items-start justify-between gap-4">
        <h3 class={['font-semibold tracking-tight', compact ? 'text-base' : 'text-xl']}>Add it to {h.label}</h3>
        {#if h.directory && !compact}
          <div class="flex flex-col items-end gap-1.5">
            <!-- eslint-disable svelte/no-navigation-without-resolve -- external directory URL, not a SvelteKit route -->
            <a
              href={h.directory.url}
              target="_blank"
              rel="noopener noreferrer"
              class="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium transition-opacity hover:opacity-90"
              style:background-color="#{h.directory.brand.hex}"
              style:color={glyphColorFor(`#${h.directory.brand.hex}`)}
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
              <span class={['leading-relaxed text-muted-foreground', compact && 'text-sm']}>{step}</span>
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
