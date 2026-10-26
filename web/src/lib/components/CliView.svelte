<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { CLI_INSTALL, CLI_REPO, MCP_REPO } from '#lib/cliLinks.js';
  import { announce } from '#lib/announce.svelte.js';
  import { Button, SectionLabel, TabStrip, tabStripId } from '#lib/ui/index.js';

  const SKILLS_URL = 'https://github.com/strelov1/freehire-cli/tree/main/skills';

  // The three steps from nothing to a first search. `auth login` opens the
  // browser (freehire-cli v0.24+), so a new user never handles an API key.
  const steps = [
    { n: 1, label: 'Install — no Go needed', cmd: CLI_INSTALL },
    { n: 2, label: 'Sign in — opens your browser, no key to paste', cmd: 'freehire auth login' },
    { n: 3, label: 'Search', cmd: 'freehire search "golang" --remote --region eu' },
  ];

  // Agent skills, one per task rather than one per tool, so a host loads what the
  // question needs instead of the whole surface. Mirrors freehire-cli's skills/
  // directory; the plugin ships each one's slash command alongside it.
  const skills = [
    {
      name: 'freehire-job-search',
      slash: '/job-search',
      desc: 'Read the saved profile, ground every filter in facets, search, and shortlist.',
    },
    {
      name: 'freehire-track-applications',
      slash: '/track-applications',
      desc: 'Where each application stands, stage moves, and reporting one that never answered.',
    },
    {
      name: 'freehire-market-fit',
      slash: '/market-fit',
      desc: 'Score a stack against live vacancy demand and put a number on every gap.',
    },
    {
      name: 'freehire-tailor-cv',
      slash: '/tailor-cv',
      desc: 'Reframe a CV toward one vacancy — every claim cited, nothing invented.',
    },
    {
      name: 'freehire-mail-triage',
      slash: '/triage-inbox',
      desc: 'Push a mail batch, judge each message, and drain the two link queues.',
    },
  ];

  // Command reference, mirroring the freehire-cli README/SKILL.md (the source of
  // truth), one tab per job to be done. Every group stays in the HTML — only the
  // unselected ones are `hidden` — because agents and crawlers read this page
  // without clicking a tab.
  type Command = { cmd: string; desc: string };
  type Group = { value: string; label: string; intro?: string; commands: Command[] };

  const groups: Group[] = [
    {
      value: 'discover',
      label: 'Discover',
      intro:
        "Start from facets — it lists every filter's live values, so search and market-fit use real values, not guesses.",
      commands: [
        { cmd: 'facets', desc: "Every filter's live values and counts." },
        { cmd: 'search <query>', desc: 'Matching jobs. Add --remote, --region, --company.' },
        { cmd: 'job <slug>', desc: "A job's full content." },
        { cmd: 'company <slug>', desc: 'A company and its open jobs.' },
        { cmd: 'market-fit --skills go,react', desc: 'How much of the live market your skills cover, and the gaps.' },
      ],
    },
    {
      value: 'track',
      label: 'Track',
      intro:
        'Everything you save, apply to and stage shows up on your Tracking board. You still apply on the employer’s site — the CLI records that you did.',
      commands: [
        { cmd: 'save <slug>', desc: 'Bookmark a job (unsave to remove).' },
        { cmd: 'apply <slug>', desc: 'Mark a job applied.' },
        {
          cmd: 'stage <slug> <stage>',
          desc: 'preparing → applied → screening → responded → interview → offer → accepted, or rejected / withdrawn.',
        },
        { cmd: 'note <slug> <text>', desc: 'Attach a free-text note.' },
        { cmd: 'my --filter applied', desc: 'Your tracked jobs: all, viewed, saved or applied.' },
        { cmd: 'profile', desc: 'Your saved search profile — skills, seniority, regions.' },
      ],
    },
    {
      value: 'cv',
      label: 'Tailor a CV',
      intro:
        "Reframe your CV toward one job, grounded in what you actually did, then export an ATS-ready PDF. cv context splits the job's requirements into the ones your history covers but buries (reframe those) and the ones it doesn't (ask, don't invent).",
      commands: [
        { cmd: 'cv context <id>', desc: 'The match analysis to reframe toward.' },
        { cmd: 'cv get <id>', desc: 'The CV document as JSON.' },
        { cmd: 'cv edit <id> --set …', desc: 'One edit by path (--ops for a batch).' },
        { cmd: 'cv render <id> --out cv.pdf', desc: 'Export the PDF.' },
      ],
    },
    {
      value: 'mail',
      label: 'Mail',
      intro:
        'freehire fetches no mail on this tier: your own client does, and inbox push hands it over. Then an agent judges each message and links it to an application.',
      commands: [
        { cmd: 'inbox push --file mail.json', desc: 'Upload a batch. Keyed by Message-ID, so a re-sync updates instead of duplicating.' },
        { cmd: 'inbox list --unclassified --body', desc: 'The work queue: unjudged mail with bodies inline.' },
        { cmd: 'inbox triage <id> <signal>', desc: 'Record what a message is; a confident verdict moves the application forward.' },
        { cmd: 'inbox list --link suggested', desc: 'Proposed links awaiting your word — inbox confirm / inbox reject.' },
        { cmd: 'inbox list --link unlinked', desc: 'Mail with no application — inbox application records one from it.' },
      ],
    },
    {
      value: 'contribute',
      label: 'Contribute',
      intro: 'Widen the catalogue. The last two need the moderator role.',
      commands: [
        { cmd: 'contribute <url>', desc: "Hand freehire a job link; a board we don't track yet gets added and crawled." },
        { cmd: 'contributions', desc: "The boards you've contributed and their state." },
        { cmd: 'submissions', desc: 'Jobs you submitted, and the review queue for moderators.' },
        { cmd: 'jobs add --url <url> --title … --company …', desc: 'Author a posting (moderator).' },
        { cmd: 'jobs edit <slug> --title …', desc: 'Edit a posting (moderator).' },
      ],
    },
  ];

  let tab = $state('discover');
  const commandTabs = groups.map(({ value, label }) => ({ id: value, label }));
  const commandsPanelId = 'cli-commands-panel';

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
      // Clipboard can be blocked (no permission / insecure context) — the command
      // is plainly visible to select by hand, so a failed copy needs no fallback.
    }
  }

  const mcpConfig = `{
  "mcpServers": {
    "freehire": {
      "command": "npx",
      "args": ["-y", "freehire-mcp"]
    }
  }
}`;

  // The two hosted MCP servers, for hosts that take a URL instead of a command
  // (Claude.ai and Claude Desktop connectors, ChatGPT). They are separate on purpose:
  // a host only offers sign-in when the server answers 401, so the anonymous one can
  // never lead to the account one.
  const remoteServers = $derived([
    {
      url: `${page.url.origin}/api/v1/mcp/account`,
      label: 'Your account',
      desc: 'Sign in with freehire when the host asks. Search jobs, read your profile, check your skills against the market, and save, mark applied or move the jobs on your board.',
    },
    {
      url: `${page.url.origin}/api/v1/mcp`,
      label: 'Anonymous, read-only',
      desc: 'No sign-in. Search jobs and employers, read postings, salary ranges and market breakdowns. Cannot save or change anything.',
    },
  ]);
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

<div class="cli">
  <!-- Hero. Left: the pitch. Right: three numbered steps from nothing to a search. -->
  <section class="grid-bg relative -mx-4 px-4 pb-16 pt-12 sm:pt-16">
    <div class="grid items-center gap-12 lg:grid-cols-2">
      <div>
        <SectionLabel text="freehire on the command line" class="reveal" style="--d:0ms" />

        <h1
          class="reveal mt-6 max-w-2xl text-balance text-4xl font-semibold leading-[0.98] tracking-tighter sm:text-6xl"
          style="--d:80ms"
        >
          Search and track<br />from the terminal.
        </h1>

        <p class="reveal mt-7 max-w-lg text-lg leading-relaxed text-muted-foreground" style="--d:160ms">
          A small CLI and an <a href="#mcp" class="text-foreground underline-offset-4 hover:underline"
            >MCP server</a
          > over the same API the site runs on — so you, a script or an AI agent can search jobs, track applications
          and tailor a CV without opening a browser.
        </p>

        <div class="reveal mt-9 flex flex-wrap items-center gap-3" style="--d:240ms">
          <Button variant="primary" size="lg" onclick={() => copy(CLI_INSTALL)}>
            {copied === CLI_INSTALL ? 'Copied ✓' : 'Copy install command'}
          </Button>
          <Button href={CLI_REPO} target="_blank" rel="noopener noreferrer" variant="outline" size="lg">
            Source ↗
          </Button>
        </div>

        <p class="reveal mt-5 text-sm leading-relaxed text-muted-foreground" style="--d:300ms">
          Not on the terminal? <a
            href={resolve('/agents')}
            class="font-medium text-foreground underline-offset-4 hover:underline"
            >Every way to connect an agent</a
          > — MCP hosts, ChatGPT, and your own client.
        </p>
      </div>

      <figure
        class="reveal overflow-hidden rounded-xl border border-border bg-secondary/60 shadow-sm"
        style="--d:320ms"
      >
        <figcaption
          class="flex items-center gap-2 border-b border-border px-4 py-2.5 text-xs text-muted-foreground"
        >
          <span class="size-2.5 rounded-full bg-muted-foreground/30"></span>
          Get started in three steps
        </figcaption>
        <ol class="divide-y divide-border">
          {#each steps as step (step.n)}
            <li class="flex gap-4 px-4 py-4">
              <span
                class="flex size-6 shrink-0 items-center justify-center rounded-full bg-foreground text-xs font-semibold text-background"
                >{step.n}</span
              >
              <div class="min-w-0 flex-1">
                <p class="text-sm text-muted-foreground">{step.label}</p>
                <div class="mt-1.5 flex items-center gap-3">
                  <code class="min-w-0 flex-1 overflow-x-auto whitespace-nowrap font-mono text-sm text-foreground"
                    >{step.cmd}</code
                  >
                  {@render copyButton(step.cmd, step.label)}
                </div>
              </div>
            </li>
          {/each}
        </ol>
      </figure>
    </div>
  </section>

  <!-- Demo — a real Claude Code session driving the CLI against the live API,
       recorded with asciinema and rendered by agg (sped up 1.5×). -->
  <section class="border-t border-border py-14 sm:py-16">
    <SectionLabel text="in claude code" />
    <div class="mt-6 grid gap-x-12 gap-y-8 lg:grid-cols-5 lg:items-center">
      <div class="lg:col-span-2">
        <h2 class="text-2xl font-semibold tracking-tight">Ask in plain words. The agent runs the CLI.</h2>
        <p class="mt-4 max-w-md leading-relaxed text-muted-foreground">
          One request — <span class="text-foreground"
            >“find senior remote Golang backend jobs in Europe and pick the 3 best”</span
          >. Claude Code searches with <code class="font-mono text-foreground">freehire search</code>, reads the
          postings with <code class="font-mono text-foreground">freehire job</code>, and explains its picks.
        </p>
        <p class="mt-4 max-w-md text-sm leading-relaxed text-muted-foreground">
          A real, unedited session against the live catalogue, sped up 1.5×.
        </p>
      </div>
      <figure class="overflow-hidden rounded-xl border border-border shadow-sm lg:col-span-3">
        <img
          src="/cli-demo-claude-code.gif"
          alt="Claude Code running freehire search and freehire job, then listing three senior remote Go backend roles in Europe with reasons for each."
          width="918"
          height="735"
          loading="lazy"
          decoding="async"
          class="block h-auto w-full"
        />
      </figure>
    </div>
  </section>

  <!-- Commands — one tab per job to be done. -->
  <section class="border-t border-border py-14 sm:py-16">
    <SectionLabel text="commands" />
    <div class="mt-6 flex flex-wrap items-end justify-between gap-4">
      <h2 class="text-2xl font-semibold tracking-tight">What it can do</h2>
      <p class="text-sm text-muted-foreground">
        Every command takes <code class="font-mono text-foreground">--json</code> for the raw API payload.
      </p>
    </div>

    <TabStrip
      tabs={commandTabs}
      active={tab}
      onSelect={(id) => (tab = id)}
      label="Command groups"
      panelId={commandsPanelId}
      class="mt-6"
    />
    <div id={commandsPanelId} role="tabpanel" aria-labelledby={tabStripId(commandsPanelId, tab)} tabindex="-1">
      {#each groups as group (group.value)}
        <div hidden={group.value !== tab} class="mt-5">
          {#if group.intro}
            <p class="max-w-2xl text-sm leading-relaxed text-muted-foreground">{group.intro}</p>
          {/if}
          <dl class="mt-5 divide-y divide-border overflow-hidden rounded-xl border border-border">
            {#each group.commands as row (row.cmd)}
              <div class="grid gap-x-6 gap-y-1 px-4 py-3 sm:grid-cols-5 sm:items-center">
                <dt class="font-mono text-sm sm:col-span-2">
                  <span class="text-muted-foreground">freehire</span> {row.cmd}
                </dt>
                <dd class="text-sm leading-relaxed text-muted-foreground sm:col-span-2">{row.desc}</dd>
                <dd class="hidden justify-self-end sm:block">{@render copyButton(`freehire ${row.cmd}`, row.cmd)}</dd>
              </div>
            {/each}
          </dl>
        </div>
      {/each}
    </div>
  </section>

  <!-- MCP — the second surface over the same API and the same sign-in. -->
  <section id="mcp" class="scroll-mt-20 border-t border-border py-14 sm:py-16">
    <SectionLabel text="mcp" />
    <div class="mt-6 grid gap-x-12 gap-y-8 lg:grid-cols-[0.95fr_1.05fr]">
      <div>
        <h2 class="text-2xl font-semibold tracking-tight">Same sign-in, any AI host</h2>
        <p class="mt-4 max-w-md leading-relaxed text-muted-foreground">
          <code class="font-mono text-foreground">freehire-mcp</code> exposes the same search, market-fit and
          tracking tools over the
          <a
            href="https://modelcontextprotocol.io"
            target="_blank"
            rel="noopener noreferrer"
            class="text-foreground underline-offset-4 hover:underline">Model Context Protocol</a
          >, so Claude Desktop, Claude Code or any MCP host can drive freehire. It runs via
          <code class="font-mono text-foreground">npx</code> — no global install.
        </p>
        <p class="mt-4 max-w-md text-sm leading-relaxed text-muted-foreground">
          It reads the CLI's sign-in from <code class="font-mono text-foreground">~/.freehire/creds.json</code>.
          On a machine without it, add
          <code class="whitespace-nowrap font-mono text-foreground">"env": &lbrace; "FREEHIRE_TOKEN": "fhk_…" &rbrace;</code> with a key
          from <a href={resolve('/my/api-keys')} class="font-medium text-foreground underline-offset-4 hover:underline"
            >API keys</a
          >.
        </p>
        <div class="mt-6">
          <Button href={MCP_REPO} target="_blank" rel="noopener noreferrer" variant="outline" size="md">
            MCP source ↗
          </Button>
        </div>
      </div>
      <figure class="overflow-hidden rounded-xl border border-border bg-secondary/60 shadow-sm">
        <figcaption
          class="flex items-center gap-2 border-b border-border px-4 py-2.5 text-xs text-muted-foreground"
        >
          <span class="size-2.5 rounded-full bg-muted-foreground/30"></span>
          <span class="font-mono">~/.claude.json</span>
          <span class="ml-auto">{@render copyButton(mcpConfig, 'MCP config')}</span>
        </figcaption>
        <pre class="overflow-x-auto p-4 font-mono text-sm leading-relaxed">{mcpConfig}</pre>
      </figure>
    </div>

    <h3 class="mt-12 text-lg font-semibold tracking-tight">Or connect by URL — nothing to install</h3>
    <p class="mt-2 max-w-2xl text-sm leading-relaxed text-muted-foreground">
      In Claude, add a custom connector; in ChatGPT, add an app. Paste one of these addresses.
    </p>
    <dl class="mt-6 grid gap-4 lg:grid-cols-2">
      {#each remoteServers as server (server.url)}
        <div class="rounded-xl border border-border bg-secondary/40 p-4">
          <dt class="text-sm font-semibold text-foreground">{server.label}</dt>
          <dd class="mt-3 flex items-center gap-3 rounded-lg border border-border bg-secondary/60 px-3 py-2">
            <code class="min-w-0 flex-1 overflow-x-auto font-mono text-sm">{server.url}</code>
            {@render copyButton(server.url, `${server.label} MCP URL`)}
          </dd>
          <dd class="mt-3 text-sm leading-relaxed text-muted-foreground">{server.desc}</dd>
        </div>
      {/each}
    </dl>
  </section>

  <!-- For AI agents — the task-shaped skills and their slash commands. -->
  <section class="border-t border-border py-14 sm:py-16">
    <SectionLabel text="for ai agents" />
    <div class="mt-6 flex flex-wrap items-end justify-between gap-6">
      <div class="max-w-xl">
        <h2 class="text-2xl font-semibold tracking-tight">Five skills, one per job</h2>
        <p class="mt-4 leading-relaxed text-muted-foreground">
          Each
          <!-- eslint-disable svelte/no-navigation-without-resolve -- absolute GitHub URL, not a SvelteKit route -->
          <a
            href={SKILLS_URL}
            target="_blank"
            rel="noopener noreferrer"
            class="font-medium text-foreground underline-offset-4 hover:underline">agent skill</a
          ><!-- eslint-enable svelte/no-navigation-without-resolve -->
          is named for the task, so a host loads only what the question needs. Install the Claude Code plugin to
          get them with their slash commands.
        </p>
      </div>
      <div class="flex items-start gap-3 rounded-lg border border-border bg-secondary/60 p-3">
        <pre class="overflow-x-auto font-mono text-sm leading-relaxed">/plugin marketplace add strelov1/freehire-cli
/plugin install freehire@freehire-cli</pre>
        {@render copyButton(
          '/plugin marketplace add strelov1/freehire-cli\n/plugin install freehire@freehire-cli',
          'plugin install'
        )}
      </div>
    </div>

    <dl class="mt-8 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {#each skills as skill (skill.name)}
        <div class="rounded-xl border border-border bg-secondary/40 p-4">
          <dt class="font-mono text-sm font-semibold text-foreground">{skill.slash}</dt>
          <dd class="mt-2 text-sm leading-relaxed text-muted-foreground">{skill.desc}</dd>
        </div>
      {/each}
    </dl>
  </section>

  <!-- Free / open-source / transparent — the project's promise, with the source. -->
  <section class="border-t border-border py-10">
    <p class="text-sm leading-relaxed text-muted-foreground">
      Free and open source — no tracking, no lock-in. Read every line of the
      <!-- eslint-disable svelte/no-navigation-without-resolve -- absolute GitHub URLs from #lib/cliLinks (shared with the JSON-LD's codeRepository), not SvelteKit routes -->
      <a
        href={CLI_REPO}
        target="_blank"
        rel="noopener noreferrer"
        class="font-medium text-foreground underline-offset-4 hover:underline">CLI ↗</a
      >
      and the
      <a
        href={MCP_REPO}
        target="_blank"
        rel="noopener noreferrer"
        class="font-medium text-foreground underline-offset-4 hover:underline">MCP server ↗</a
      ><!-- eslint-enable svelte/no-navigation-without-resolve --> on GitHub. The same endpoints are in the
      <a href={resolve('/docs/api')} class="font-medium text-foreground underline-offset-4 hover:underline"
        >API reference</a
      >.
    </p>
  </section>
</div>

<style>
  /* Dotted hero background, faded toward the edges with a radial mask — the same
     device the homepage hero uses, so the CLI page reads as the same product.
     Component styles are scoped, so this is duplicated rather than shared. */
  .grid-bg::before {
    content: '';
    position: absolute;
    inset: 0;
    z-index: -1;
    background-image: radial-gradient(var(--muted-foreground) 1px, transparent 1.2px);
    background-size: 22px 22px;
    opacity: 0.16;
    -webkit-mask-image: radial-gradient(ellipse 90% 75% at 25% 0%, #000 18%, transparent 80%);
    mask-image: radial-gradient(ellipse 90% 75% at 25% 0%, #000 18%, transparent 80%);
  }

  /* One orchestrated page-load: each .reveal rises in, staggered by its --d. */
  .reveal {
    opacity: 0;
    animation: rise 0.7s cubic-bezier(0.2, 0.7, 0.2, 1) forwards;
    animation-delay: var(--d, 0ms);
  }
  @keyframes rise {
    from {
      opacity: 0;
      transform: translateY(10px);
    }
    to {
      opacity: 1;
      transform: none;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .reveal {
      animation: none;
      opacity: 1;
    }
  }
</style>
