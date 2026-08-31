<script lang="ts">
  import { resolve } from '$app/paths';
  import { CLI_REPO, MCP_REPO } from '$lib/cliLinks';
  import { Button } from '$lib/ui';
  import { SectionLabel } from '$lib/ui';

  const SKILLS_URL = 'https://github.com/strelov1/freehire-cli/tree/main/skills';
  const INSTALL = 'curl -fsSL https://freehire.me/install.sh | sh';

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
  // truth). Discover the market and its jobs first, then track your interaction.
  const discover = [
    { cmd: 'facets', desc: "Every filter's live values + counts — the vocabulary to filter by." },
    {
      cmd: 'market-fit --skills go,react',
      desc: 'How much of the live market your skills cover, and the gaps.',
    },
    { cmd: 'search <query>', desc: 'List matching jobs (add --remote, --region, --company).' },
    { cmd: 'job <slug>', desc: "Show a job's full content." },
    { cmd: 'company <slug>', desc: 'Show a company and its open jobs.' },
  ];

  const track = [
    { cmd: 'apply <slug>', desc: 'Mark a job applied for your account.' },
    { cmd: 'save <slug>', desc: 'Bookmark a job (unsave to remove).' },
    { cmd: 'stage <slug> <stage>', desc: 'Set the application stage.' },
    { cmd: 'note <slug> <text>', desc: 'Attach a free-text note.' },
    { cmd: 'my --filter applied', desc: 'Your tracked jobs (all|viewed|saved|applied).' },
    { cmd: 'profile', desc: 'Your saved search profile — skills, seniority, regions.' },
  ];

  // Application mail. The whole inbox surface is here because freehire fetches
  // nothing on this tier: your own client does, and `inbox push` hands it over.
  const inbox = [
    { cmd: 'inbox push --file mail.json', desc: 'Upload a batch your own client fetched (keyed by Message-ID, so a re-sync updates instead of duplicating).' },
    { cmd: 'inbox list --unclassified --body', desc: "The agent's work queue: unjudged mail with bodies inline, and nothing gets marked read." },
    { cmd: 'inbox triage <id> <signal>', desc: 'Record what a message is; a confident verdict moves the application forward.' },
    { cmd: 'inbox list --link suggested', desc: 'Proposed links awaiting your word — inbox confirm / inbox reject.' },
    { cmd: 'inbox list --link unlinked', desc: 'Mail with no application to attach to — inbox application records one from the message.' },
  ];

  // Widening the catalogue, and the moderator queue behind it.
  const contribute = [
    { cmd: 'contribute <url>', desc: "Hand freehire a job link; a board we don't track yet gets added and crawled." },
    { cmd: 'contributions', desc: "The boards you've contributed and their state." },
    { cmd: 'submissions', desc: 'Jobs you submitted; the review queue with pending/approve/reject (moderator).' },
    { cmd: 'jobs add | edit <slug>', desc: 'Create or edit a job posting (moderator).' },
  ];

  // Tasteful micro-interaction: copy the install one-liner, flash a confirmation.
  let copied = $state(false);
  let copyTimer: ReturnType<typeof setTimeout> | undefined;
  async function copyInstall() {
    try {
      await navigator.clipboard.writeText(INSTALL);
      copied = true;
      clearTimeout(copyTimer);
      copyTimer = setTimeout(() => (copied = false), 1600);
    } catch {
      // Clipboard can be blocked (no permission / insecure context) — the command
      // is plainly visible to select by hand, so a failed copy needs no fallback.
    }
  }
</script>

<div class="cli">
  <!-- Hero. Left: the pitch, framed for agents. Right: the only terminal on the
       page — install, authenticate, search. -->
  <section class="grid-bg relative -mx-4 px-4 pb-16 pt-12 sm:pt-16">
    <div class="grid items-center gap-12 lg:grid-cols-[1.05fr_0.95fr]">
      <div>
        <SectionLabel text="freehire on the command line" class="reveal" style="--d:0ms" />

        <h1
          class="reveal mt-6 max-w-2xl text-balance text-4xl font-semibold leading-[0.98] tracking-tighter sm:text-6xl"
          style="--d:80ms"
        >
          Search and track<br />from the terminal.
        </h1>

        <p class="reveal mt-7 max-w-xl text-lg leading-relaxed text-muted-foreground" style="--d:160ms">
          <code class="font-mono text-foreground">freehire</code> is a small CLI — and an
          <a href="#mcp" class="text-foreground underline-offset-4 hover:underline">MCP server</a> — over the
          same job API the site runs on. One <span class="text-foreground">API key</span> lets an
          <span class="text-foreground">AI agent</span> or a script search and open jobs, track
          applications, work through your
          <a href={resolve('/features/inbox')} class="text-foreground underline-offset-4 hover:underline"
            >application mail</a
          >
          and tailor a CV to a vacancy — all without a browser. (You still apply on the employer's site;
          the CLI records that you did.)
        </p>

        <div class="reveal mt-9 flex flex-wrap items-center gap-3" style="--d:240ms">
          <Button href={resolve('/my/api-keys')} variant="primary" size="lg">Get an API key</Button>
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
        class="reveal overflow-hidden rounded-xl border border-border bg-secondary/60 font-mono text-sm shadow-sm"
        style="--d:320ms"
      >
        <figcaption
          class="flex items-center gap-2 border-b border-border px-4 py-2.5 text-xs text-muted-foreground"
        >
          <span class="size-2.5 rounded-full bg-muted-foreground/30"></span>
          terminal
          <button
            type="button"
            onclick={copyInstall}
            class="ml-auto rounded-md border border-border px-2 py-0.5 text-[11px] font-medium text-muted-foreground transition-colors hover:text-foreground"
          >
            {copied ? 'copied ✓' : 'copy'}
          </button>
        </figcaption>
        <pre class="overflow-x-auto p-4 leading-relaxed"><span class="text-muted-foreground"># install — no Go needed</span>
curl -fsSL <span class="text-foreground">https://freehire.me/install.sh</span> | sh

<span class="text-muted-foreground"># authenticate once (key from /my/api-keys)</span>
freehire auth login --token <span class="text-foreground">fhk_…</span>

<span class="text-muted-foreground"># discover the market, then search</span>
freehire facets
freehire search <span class="text-foreground">"golang"</span> --remote --region eu</pre>
      </figure>
    </div>
  </section>

  <!-- Commands — the whole reference, compact. -->
  <section class="border-t border-border py-14 sm:py-16">
    <SectionLabel text="commands" />
    <div class="mt-8 grid gap-x-12 gap-y-8 sm:grid-cols-2">
      <div>
        <h2 class="text-sm font-semibold uppercase tracking-wide text-muted-foreground">Discover</h2>
        <dl class="mt-4 space-y-3">
          {#each discover as row (row.cmd)}
            <div>
              <dt class="font-mono text-sm">
                <span class="text-muted-foreground">freehire</span> {row.cmd}
              </dt>
              <dd class="text-sm leading-relaxed text-muted-foreground">{row.desc}</dd>
            </div>
          {/each}
        </dl>
      </div>
      <div>
        <h2 class="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
          Track applications &amp; notes
        </h2>
        <dl class="mt-4 space-y-3">
          {#each track as row (row.cmd)}
            <div>
              <dt class="font-mono text-sm">
                <span class="text-muted-foreground">freehire</span> {row.cmd}
              </dt>
              <dd class="text-sm leading-relaxed text-muted-foreground">{row.desc}</dd>
            </div>
          {/each}
        </dl>
      </div>
      <div>
        <h2 class="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
          Application mail
        </h2>
        <dl class="mt-4 space-y-3">
          {#each inbox as row (row.cmd)}
            <div>
              <dt class="font-mono text-sm">
                <span class="text-muted-foreground">freehire</span> {row.cmd}
              </dt>
              <dd class="text-sm leading-relaxed text-muted-foreground">{row.desc}</dd>
            </div>
          {/each}
        </dl>
      </div>
      <div>
        <h2 class="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
          Contribute &amp; moderate
        </h2>
        <dl class="mt-4 space-y-3">
          {#each contribute as row (row.cmd)}
            <div>
              <dt class="font-mono text-sm">
                <span class="text-muted-foreground">freehire</span> {row.cmd}
              </dt>
              <dd class="text-sm leading-relaxed text-muted-foreground">{row.desc}</dd>
            </div>
          {/each}
        </dl>
      </div>
    </div>

    <p class="mt-8 max-w-2xl text-sm leading-relaxed text-muted-foreground">
      Start from <code class="font-mono text-foreground">freehire facets</code> — it lists every filter's live
      values so <code class="font-mono text-foreground">search</code> and
      <code class="font-mono text-foreground">market-fit</code> use real values, not guesses. Everything you
      save, apply to and stage shows up on your
      <a href={resolve('/my/tracking')} class="font-medium text-foreground underline-offset-4 hover:underline"
        >Tracking</a
      >
      board. <code class="font-mono text-foreground">stage</code> takes a controlled value:
      <span class="font-mono text-foreground"
        >preparing → applied → screening → responded → interview → offer → accepted</span
      >, plus <span class="font-mono text-foreground">rejected</span> /
      <span class="font-mono text-foreground">withdrawn</span>.
    </p>

    <!-- CV tailoring. On both surfaces: the CLI edits by path, the MCP server
         exposes cv_context/cv_get/cv_edit/cv_render. -->
    <div class="mt-8 max-w-2xl rounded-lg border border-border bg-secondary/40 p-4">
      <h3 class="text-sm font-semibold">Tailor a CV to a vacancy</h3>
      <p class="mt-2 text-sm leading-relaxed text-muted-foreground">
        After a match analysis, <a href={resolve('/features/tailor')} class="font-medium text-foreground underline-offset-4 hover:underline">reframe your CV</a> toward one job — grounded in what you actually did, never
        fabricated — then export an ATS-ready PDF. <code class="font-mono text-foreground">cv context</code>
        is the honest part: it splits the job's requirements into the ones your history already covers but
        buries (reframe those) and the ones it doesn't (ask, don't invent), so an agent editing your CV
        knows which is which.
      </p>
      <pre
        class="mt-3 overflow-x-auto rounded-md border border-border bg-background/60 p-3 font-mono text-sm leading-relaxed"><span class="text-muted-foreground">freehire</span> cv context &lt;id&gt;        <span class="text-muted-foreground"># the match analysis to reframe toward</span>
<span class="text-muted-foreground">freehire</span> cv get &lt;id&gt;            <span class="text-muted-foreground"># the CV document as JSON</span>
<span class="text-muted-foreground">freehire</span> cv edit &lt;id&gt; --set …   <span class="text-muted-foreground"># one edit by path (--ops for a batch)</span>
<span class="text-muted-foreground">freehire</span> cv render &lt;id&gt; --out cv.pdf</pre>
    </div>
  </section>

  <!-- MCP — the second surface over the same API and the same key. -->
  <section id="mcp" class="scroll-mt-20 border-t border-border py-14 sm:py-16">
    <SectionLabel text="mcp" />
    <div class="mt-8 grid gap-x-12 gap-y-8 lg:grid-cols-[0.95fr_1.05fr]">
      <div>
        <h2 class="text-2xl font-semibold tracking-tight">Same key, any AI host</h2>
        <p class="mt-4 max-w-md leading-relaxed text-muted-foreground">
          <code class="font-mono text-foreground">freehire-mcp</code> exposes the same search, market-fit and
          tracking tools over the
          <a
            href="https://modelcontextprotocol.io"
            target="_blank"
            rel="noopener noreferrer"
            class="text-foreground underline-offset-4 hover:underline">Model Context Protocol</a
          > — so Claude Desktop, Claude Code or any MCP host can drive freehire directly. It runs via
          <code class="font-mono text-foreground">npx</code>; no global install.
        </p>
        <p class="mt-4 max-w-md text-sm leading-relaxed text-muted-foreground">
          It shares the CLI's credentials: if you've run
          <code class="font-mono text-foreground">freehire auth login</code>, omit
          <code class="font-mono text-foreground">env</code> and it reads
          <code class="font-mono text-foreground">~/.freehire/creds.json</code>.
        </p>
        <div class="mt-6">
          <Button href={MCP_REPO} target="_blank" rel="noopener noreferrer" variant="outline" size="md">
            MCP source ↗
          </Button>
        </div>
      </div>
      <figure
        class="overflow-hidden rounded-xl border border-border bg-secondary/60 font-mono text-sm shadow-sm"
      >
        <figcaption
          class="flex items-center gap-2 border-b border-border px-4 py-2.5 text-xs text-muted-foreground"
        >
          <span class="size-2.5 rounded-full bg-muted-foreground/30"></span>
          ~/.claude.json
        </figcaption>
        <pre class="overflow-x-auto p-4 leading-relaxed">{`{
  "mcpServers": {
    "freehire": {
      "command": "npx",
      "args": ["-y", "freehire-mcp"],
      "env": { "FREEHIRE_TOKEN": "fhk_…" }
    }
  }
}`}</pre>
      </figure>
    </div>
  </section>

  <!-- For AI agents — the task-shaped skills, their slash commands, and the
       machine-readable conventions underneath them. -->
  <section class="border-t border-border py-14 sm:py-16">
    <SectionLabel text="for ai agents" />

    <div class="mt-6 grid gap-x-12 gap-y-6 lg:grid-cols-[0.95fr_1.05fr]">
      <div>
        <h2 class="text-2xl font-semibold tracking-tight">Five skills, one per job</h2>
        <p class="mt-4 max-w-md leading-relaxed text-muted-foreground">
          Each
          <!-- eslint-disable svelte/no-navigation-without-resolve -- absolute GitHub URL, not a SvelteKit route -->
          <a
            href={SKILLS_URL}
            target="_blank"
            rel="noopener noreferrer"
            class="font-medium text-foreground underline-offset-4 hover:underline">agent skill</a
          ><!-- eslint-enable svelte/no-navigation-without-resolve -->
          is named for the task, not the tool, so a host loads what the question needs instead of the
          whole surface. Drop them into a Claude Code (or compatible) skills directory — or install the
          plugin and get the slash commands too.
        </p>
        <pre
          class="mt-5 max-w-md overflow-x-auto rounded-lg border border-border bg-secondary/60 p-3 font-mono text-sm leading-relaxed">/plugin marketplace add strelov1/freehire-cli
/plugin install freehire@freehire-cli</pre>
        <p class="mt-4 max-w-md text-sm leading-relaxed text-muted-foreground">
          Every command takes <code class="font-mono text-foreground">--json</code> for the raw API
          payload — results to <span class="font-mono text-foreground">stdout</span>, errors to
          <span class="font-mono text-foreground">stderr</span>, non-zero exit on failure. The same
          endpoints are in the
          <a
            href={resolve('/docs/api')}
            class="font-medium text-foreground underline-offset-4 hover:underline">API reference</a
          >.
        </p>
      </div>

      <dl class="space-y-4">
        {#each skills as skill (skill.name)}
          <div class="rounded-lg border border-border bg-secondary/40 p-4">
            <dt class="flex flex-wrap items-baseline gap-x-3 gap-y-1">
              <span class="font-mono text-sm font-semibold text-foreground">{skill.slash}</span>
              <span class="font-mono text-xs text-muted-foreground">{skill.name}</span>
            </dt>
            <dd class="mt-2 text-sm leading-relaxed text-muted-foreground">{skill.desc}</dd>
          </div>
        {/each}
      </dl>
    </div>
  </section>

  <!-- Moderators — gated authoring, kept to one line + two commands. -->
  <section class="border-t border-border py-14 sm:py-16">
    <SectionLabel text="moderators" />
    <p class="mt-6 max-w-2xl leading-relaxed text-muted-foreground">
      With the <code class="font-mono text-foreground">moderator</code> role you can author postings:
    </p>
    <pre
      class="mt-4 max-w-2xl overflow-x-auto rounded-lg border border-border bg-secondary/60 p-3 font-mono text-sm leading-relaxed">freehire jobs add --url &lt;url&gt; --title "Senior Go Developer" --company Acme
freehire jobs edit &lt;slug&gt; --title "Staff Go Developer"</pre>
  </section>

  <!-- Free / open-source / transparent — the project's promise, with the source. -->
  <section class="border-t border-border py-10">
    <p class="text-sm leading-relaxed text-muted-foreground">
      Free and open source — no tracking, no lock-in. Read every line of the
      <!-- eslint-disable svelte/no-navigation-without-resolve -- absolute GitHub URLs from $lib/cliLinks (shared with the JSON-LD's codeRepository), not SvelteKit routes -->
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
      ><!-- eslint-enable svelte/no-navigation-without-resolve --> on GitHub.
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
