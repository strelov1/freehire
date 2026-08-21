<script lang="ts">
  import { resolve } from '$app/paths';
  import { AGENTS_FAQ } from '$lib/agentsFaq';
  import { CLI_REPO, MCP_REPO } from '$lib/cliLinks';
  import { Button, SectionLabel } from '$lib/ui';

  const LLMS_TXT = 'https://freehire.me/llms.txt';
  const OPENAPI = 'https://freehire.me/openapi.yaml';
  const GPT_URL = 'https://chatgpt.com/g/g-6a5281b64948819193bf3a1021e075da-freehire';

  // The whole point of the page: one block you paste into whatever agent you
  // already have. It leans on two URLs that already exist and are already
  // maintained — the install script and llms.txt — rather than restating setup
  // that would then drift from them.
  const HANDOFF = `Install freehire and use it to run my job search.

  curl -fsSL https://freehire.me/install.sh | sh

Then read https://freehire.me/llms.txt for the conventions,
and \`freehire --help\` for the commands.`;

  // What an agent can actually do, each pointing at the feature that owns it.
  // Six cells, so no half-empty last row on any breakpoint — those hairline
  // grids render a missing cell as a slab of border colour, not as nothing.
  const capabilities = [
    {
      n: '01',
      title: 'Search grounded in facets',
      body: 'Ask for the live filter vocabulary first, then search with real values instead of guesses. A filter nothing reads is ignored, not refused — so a typo silently returns everything.',
      href: resolve('/features/advanced-search'),
      label: 'Advanced search',
    },
    {
      n: '02',
      title: 'Score a stack against demand',
      body: 'Market fit measures how much of the open market your skills already cover, and puts a number on every gap — which is a better question than "am I qualified for this one job".',
      href: resolve('/insights'),
      label: 'Market insights',
    },
    {
      n: '03',
      title: 'Track every application',
      body: 'Save, mark applied, move a stage, attach a note. Everything lands on the same board the website shows, because it is the same account.',
      href: resolve('/features/tracking'),
      label: 'Application tracking',
    },
    {
      n: '04',
      title: 'Tailor a CV to one vacancy',
      body: 'Reframe what you actually did toward one posting, cite every claim, invent nothing, then render an ATS-ready PDF. The agent is told which requirements your history covers and which it does not.',
      href: resolve('/features/tailor'),
      label: 'CV tailoring',
    },
    {
      n: '05',
      title: 'Triage the replies',
      body: 'Push a batch of application mail, judge each message, and drain the two link queues. A confident verdict moves the application forward on its own.',
      href: resolve('/features/inbox'),
      label: 'Inbox',
    },
    {
      n: '06',
      title: 'Skip the dead postings',
      body: 'Every posting carries a reality signal, so an agent can drop the ones that have gone stale instead of writing you a cover letter for a role nobody is filling.',
      href: resolve('/features/ghost-jobs'),
      label: 'Ghost jobs',
    },
  ];

  // The comparison is written off the clients themselves, not off intent: the
  // MCP server ships cv_* and experience_* tools but no inbox ones, and the
  // published GPT covers public reads plus tracking. Check the tool list before
  // changing a mark here.
  const matrix = {
    columns: ['Local CLI', 'MCP host', 'ChatGPT'],
    rows: [
      { label: 'Search, facets, market fit', marks: ['yes', 'yes', 'yes'] },
      { label: 'Save, apply, stages, notes', marks: ['yes', 'yes', 'key'] },
      { label: 'CV tailoring and PDF export', marks: ['yes', 'yes', 'no'] },
      { label: 'Application mail triage', marks: ['yes', 'no', 'no'] },
      { label: 'Reads and writes your own files', marks: ['yes', 'no', 'no'] },
      { label: 'Task-shaped agent skills', marks: ['yes', 'no', 'no'] },
    ],
  } as const;

  const MARK_GLYPH: Record<string, string> = { yes: '●', key: '◐', no: '–' };
  const MARK_TITLE: Record<string, string> = {
    yes: 'Supported',
    key: 'Supported once you paste an API key',
    no: 'Not available on this surface',
  };

  // Copy the handoff block, flash a confirmation. The block is plainly visible
  // to select by hand, so a blocked clipboard needs no fallback.
  let copied = $state(false);
  let copyTimer: ReturnType<typeof setTimeout> | undefined;
  async function copyHandoff() {
    try {
      await navigator.clipboard.writeText(HANDOFF);
      copied = true;
      clearTimeout(copyTimer);
      copyTimer = setTimeout(() => (copied = false), 1800);
    } catch {
      // Clipboard can be blocked (no permission, insecure context) — ignore.
    }
  }
</script>

<div class="agents">
  <!-- Hero. Left: why an agent belongs here at all. Right: the one thing to
       copy — styled as a hand-off card, with a beam crossing its top edge. -->
  <section class="grid-bg relative -mx-4 px-4 pb-16 pt-12 sm:pt-16">
    <div class="grid items-center gap-12 lg:grid-cols-[1.02fr_0.98fr]">
      <div>
        <SectionLabel text="built for agents" class="reveal" style="--d:0ms" />

        <h1
          class="reveal mt-6 max-w-2xl text-balance text-4xl font-semibold leading-[0.98] tracking-tighter sm:text-6xl"
          style="--d:80ms"
        >
          Hand freehire<br />to your agent.
        </h1>

        <p class="reveal mt-7 max-w-xl text-lg leading-relaxed text-muted-foreground" style="--d:160ms">
          Millions of open postings behind a public API with
          <span class="text-foreground">no login wall</span>, so an agent can run the whole hunt:
          search, shortlist, track, reframe a CV, read the replies. There is no sign-up to get
          through first — the fastest way to start is to paste the block on the right into whatever
          agent you already use.
        </p>

        <div class="reveal mt-9 flex flex-wrap items-center gap-3" style="--d:240ms">
          <Button href={resolve('/my/api-keys')} variant="primary" size="lg">Get an API key</Button>
          <Button href={LLMS_TXT} target="_blank" rel="noopener noreferrer" variant="outline" size="lg">
            llms.txt ↗
          </Button>
        </div>

        <p class="reveal mt-5 max-w-lg text-sm leading-relaxed text-muted-foreground" style="--d:300ms">
          Searching needs no key at all. One is only needed for the half that belongs to your
          account — saving, applying, stages, CV work and application mail.
        </p>
      </div>

      <div class="reveal handoff relative" style="--d:340ms">
        <figure
          class="overflow-hidden rounded-xl border border-border bg-secondary/60 font-mono text-sm shadow-sm"
        >
          <figcaption
            class="flex items-center gap-2 border-b border-dashed border-border px-4 py-2.5 text-xs text-muted-foreground"
          >
            <span class="beam-dot size-2.5 rounded-full bg-muted-foreground/30"></span>
            hand-off
            <button
              type="button"
              onclick={copyHandoff}
              class="ml-auto rounded-md border border-border px-2 py-0.5 text-[11px] font-medium text-muted-foreground transition-colors hover:text-foreground"
            >
              {copied ? 'copied ✓' : 'copy'}
            </button>
          </figcaption>
          <pre class="overflow-x-auto p-4 leading-relaxed">Install freehire and use it to run my job search.

  curl -fsSL <span class="text-foreground">https://freehire.me/install.sh</span> | sh

Then read <span class="text-foreground">https://freehire.me/llms.txt</span> for the conventions,
and <span class="text-foreground">`freehire --help`</span> for the commands.</pre>
        </figure>
        <p class="mt-3 pl-1 font-mono text-xs text-muted-foreground">
          ↑ paste into your agent — it takes it from there
        </p>
      </div>
    </div>
  </section>

  <!-- The three tracks, deliberately unequal in weight. The wide card is the
       recommendation; the ranking is carried by the layout, not by a claim. -->
  <section id="tracks" class="scroll-mt-20 border-t border-border py-14 sm:py-16">
    <SectionLabel text="pick your track" />
    <h2 class="mt-6 max-w-2xl text-2xl font-semibold tracking-tight sm:text-3xl">
      Three ways in, and they are not equal.
    </h2>
    <p class="mt-4 max-w-2xl leading-relaxed text-muted-foreground">
      All three run on one <code class="font-mono text-foreground">fhk_</code> key and one account.
      What separates them is how much of the surface the host can actually reach.
    </p>

    <!-- 01 — the recommendation. Wide, with the terminal inside it. -->
    <div class="mt-10 overflow-hidden rounded-xl border border-border bg-secondary/40">
      <div class="grid gap-x-10 gap-y-8 p-6 sm:p-8 lg:grid-cols-[0.95fr_1.05fr]">
        <div>
          <div class="flex flex-wrap items-center gap-3">
            <span class="font-mono text-sm text-muted-foreground">01</span>
            <span
              class="rounded-full border border-foreground/25 px-2.5 py-0.5 font-mono text-[11px] uppercase tracking-wider text-foreground"
              >best fit</span
            >
          </div>
          <h3 class="mt-4 text-xl font-semibold tracking-tight">A local harness</h3>
          <p class="mt-3 leading-relaxed text-muted-foreground">
            Claude Code, Codex, Cursor, Cline, Aider — anything that runs commands on your machine.
            Give it the hand-off block and it installs the CLI itself. This is the only track that
            reaches the whole surface, because it is the only one that can also touch your files:
            reading the CV you keep locally, writing the tailored PDF back next to it.
          </p>
          <p class="mt-4 text-sm leading-relaxed text-muted-foreground">
            The CLI ships five agent skills named for the task rather than the tool, so a host loads
            what the question needs instead of the whole surface — plus a Claude Code plugin that
            brings their slash commands with them.
          </p>
          <div class="mt-6 flex flex-wrap gap-3">
            <Button href={resolve('/cli')} variant="primary" size="md">CLI reference</Button>
            <Button href={CLI_REPO} target="_blank" rel="noopener noreferrer" variant="outline" size="md">
              Source ↗
            </Button>
          </div>
        </div>

        <figure
          class="overflow-hidden self-start rounded-lg border border-border bg-background/70 font-mono text-sm shadow-sm"
        >
          <figcaption
            class="flex items-center gap-2 border-b border-border px-4 py-2.5 text-xs text-muted-foreground"
          >
            <span class="size-2.5 rounded-full bg-muted-foreground/30"></span>
            terminal
          </figcaption>
          <pre class="overflow-x-auto p-4 leading-relaxed"><span class="text-muted-foreground"># no Go toolchain needed</span>
curl -fsSL <span class="text-foreground">https://freehire.me/install.sh</span> | sh

<span class="text-muted-foreground"># only for the account half</span>
freehire auth login --token <span class="text-foreground">fhk_…</span>

<span class="text-muted-foreground"># vocabulary first, then search</span>
freehire facets
freehire search <span class="text-foreground">"golang"</span> --remote --region eu</pre>
        </figure>
      </div>
    </div>

    <!-- 02 and 03 — the hosted assistants, side by side and visibly smaller. -->
    <div class="mt-6 grid gap-6 lg:grid-cols-2">
      <div class="flex flex-col rounded-xl border border-border p-6 sm:p-7">
        <span class="font-mono text-sm text-muted-foreground">02</span>
        <h3 class="mt-3 text-lg font-semibold tracking-tight">Claude Desktop, or any MCP host</h3>
        <p class="mt-3 text-sm leading-relaxed text-muted-foreground">
          <code class="font-mono text-foreground">freehire-mcp</code> exposes search, market fit,
          tracking and CV tailoring as
          <!-- eslint-disable svelte/no-navigation-without-resolve -- absolute URL to the protocol spec, not a SvelteKit route -->
          <a
            href="https://modelcontextprotocol.io"
            target="_blank"
            rel="noopener noreferrer"
            class="text-foreground underline-offset-4 hover:underline">Model Context Protocol</a
          ><!-- eslint-enable svelte/no-navigation-without-resolve -->
          tools. It runs via <code class="font-mono text-foreground">npx</code> — no global install —
          and reuses the CLI credentials if you have already signed in there.
        </p>
        <pre
          class="mt-4 overflow-x-auto rounded-lg border border-border bg-secondary/50 p-3 font-mono text-xs leading-relaxed">{`{
  "mcpServers": {
    "freehire": {
      "command": "npx",
      "args": ["-y", "freehire-mcp"]
    }
  }
}`}</pre>
        <div class="mt-auto flex flex-wrap gap-3 pt-6">
          <Button href={resolve('/cli') + '#mcp'} variant="outline" size="md">MCP setup</Button>
          <Button href={MCP_REPO} target="_blank" rel="noopener noreferrer" variant="ghost" size="md">
            Source ↗
          </Button>
        </div>
      </div>

      <div class="flex flex-col rounded-xl border border-border p-6 sm:p-7">
        <span class="font-mono text-sm text-muted-foreground">03</span>
        <h3 class="mt-3 text-lg font-semibold tracking-tight">ChatGPT</h3>
        <p class="mt-3 text-sm leading-relaxed text-muted-foreground">
          A published custom GPT is already wired to the same API, and searching in it needs no
          setup at all. Paste your own key into its authentication field and it can save, apply and
          move stages too. Prefer to build your own? The
          <!-- eslint-disable svelte/no-navigation-without-resolve -- absolute URL to the served schema file, not a SvelteKit route -->
          <a
            href={OPENAPI}
            target="_blank"
            rel="noopener noreferrer"
            class="text-foreground underline-offset-4 hover:underline">OpenAPI schema</a
          ><!-- eslint-enable svelte/no-navigation-without-resolve -->
          imports straight into an Action.
        </p>
        <p class="mt-4 text-sm leading-relaxed text-muted-foreground">
          What it cannot do is the work that needs your files: there is no CV rendering and no mail
          triage on this track.
        </p>
        <div class="mt-auto flex flex-wrap gap-3 pt-6">
          <Button href={resolve('/chatgpt')} variant="outline" size="md">ChatGPT guide</Button>
          <Button href={GPT_URL} target="_blank" rel="noopener noreferrer" variant="ghost" size="md">
            Open the GPT ↗
          </Button>
        </div>
      </div>
    </div>

    <!-- The quiet fourth option, for anyone writing their own client. -->
    <p class="mt-8 max-w-3xl text-sm leading-relaxed text-muted-foreground">
      <span class="font-medium text-foreground">Rolling your own?</span> Every page on this site is
      one unauthenticated JSON call. Start from the
      <a
        href={resolve('/docs/api')}
        class="font-medium text-foreground underline-offset-4 hover:underline">API reference</a
      >
      or the
      <!-- eslint-disable svelte/no-navigation-without-resolve -- absolute URLs to served static files, not SvelteKit routes -->
      <a
        href={OPENAPI}
        target="_blank"
        rel="noopener noreferrer"
        class="font-medium text-foreground underline-offset-4 hover:underline">OpenAPI schema ↗</a
      >, and hand your agent
      <a
        href={LLMS_TXT}
        target="_blank"
        rel="noopener noreferrer"
        class="font-medium text-foreground underline-offset-4 hover:underline">llms.txt ↗</a
      ><!-- eslint-enable svelte/no-navigation-without-resolve -->, which states the conventions in
      the form it reads best. Please send a
      <code class="font-mono text-foreground">User-Agent</code> that names you.
    </p>
  </section>

  <!-- What the agent does once it is connected — each cell owned by a feature
       page, so this section is a map rather than a second copy of them. -->
  <section class="border-t border-border py-14 sm:py-16">
    <SectionLabel text="what your agent can do" />
    <div
      class="mt-8 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-2 lg:grid-cols-3"
    >
      {#each capabilities as item (item.n)}
        <div class="group flex flex-col bg-background p-6 transition-colors hover:bg-secondary/40">
          <span
            class="font-mono text-sm text-muted-foreground transition-colors group-hover:text-foreground"
            >{item.n}</span
          >
          <h3 class="mt-4 text-lg font-semibold tracking-tight">{item.title}</h3>
          <p class="mt-2 text-sm leading-relaxed text-muted-foreground">{item.body}</p>
          <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- internal route already passed through resolve() when building `capabilities`; the linter can't trace it via the variable -->
          <a href={item.href}
            class="mt-4 inline-flex w-fit items-center gap-1 text-sm font-medium text-foreground underline-offset-4 hover:underline"
          >
            {item.label} <span aria-hidden="true">→</span>
          </a>
        </div>
      {/each}
    </div>

    <p class="mt-8 max-w-3xl text-sm leading-relaxed text-muted-foreground">
      An agent never submits an application for you — you still apply on the employer's own site and
      freehire records that you did. The one exception works the other way round: the
      <a
        href={resolve('/features/extension')}
        class="font-medium text-foreground underline-offset-4 hover:underline">browser extension</a
      >
      fills an ATS form for you to check before you press send.
    </p>
  </section>

  <!-- The honest comparison. Marks are read off the clients' tool lists. -->
  <section class="border-t border-border py-14 sm:py-16">
    <SectionLabel text="why local wins" />
    <div class="mt-8 grid gap-x-12 gap-y-8 lg:grid-cols-[0.85fr_1.15fr]">
      <div>
        <h2 class="text-2xl font-semibold tracking-tight">The surface is the difference</h2>
        <p class="mt-4 leading-relaxed text-muted-foreground">
          A hosted assistant runs freehire's tools inside someone else's conversation. A local
          harness runs them next to your files, which is where a job hunt actually lives: the CV you
          keep, the notes you take, the mailbox you export. That is the whole of the gap — not
          model quality.
        </p>
        <p class="mt-4 text-sm leading-relaxed text-muted-foreground">
          Every mark below comes from the client's own tool list. If a surface gains a tool, this
          table changes with it.
        </p>
      </div>

      <!-- `table-fixed` with explicit column widths, so the three narrow mark
           columns all fit at 390px rather than pushing the last one off the
           edge; below ~340px the wrapper still scrolls. -->
      <div class="overflow-x-auto">
        <table class="w-full min-w-80 table-fixed border-collapse text-sm">
          <thead>
            <tr class="border-b border-border">
              <th class="w-2/5 py-3 pr-3 text-left text-xs font-medium text-muted-foreground sm:text-sm"
                >Capability</th
              >
              {#each matrix.columns as column, i (column)}
                <th
                  class="w-1/5 px-1 py-3 text-center text-xs font-medium sm:px-3 sm:text-sm {i === 0
                    ? 'text-foreground'
                    : 'text-muted-foreground'}"
                >
                  {column}
                </th>
              {/each}
            </tr>
          </thead>
          <tbody>
            {#each matrix.rows as row (row.label)}
              <tr class="border-b border-border/60">
                <td class="py-3 pr-3 leading-relaxed">{row.label}</td>
                {#each row.marks as mark, i (i)}
                  <td class="px-1 py-3 text-center sm:px-3">
                    <span
                      class="font-mono {mark === 'no'
                        ? 'text-muted-foreground/50'
                        : 'text-foreground'}"
                      title={MARK_TITLE[mark]}
                    >
                      {MARK_GLYPH[mark]}
                    </span>
                    <span class="sr-only">{MARK_TITLE[mark]}</span>
                  </td>
                {/each}
              </tr>
            {/each}
          </tbody>
        </table>
        <p class="mt-3 font-mono text-xs text-muted-foreground">
          ● supported · ◐ needs your API key · – not on this surface
        </p>
      </div>
    </div>
  </section>

  <!-- FAQ. Same array the FAQPage JSON-LD is built from. -->
  <section class="border-t border-border py-14 sm:py-16">
    <SectionLabel text="faq" />
    <dl class="mt-8 grid gap-x-12 gap-y-8 sm:grid-cols-2">
      {#each AGENTS_FAQ as item (item.question)}
        <div>
          <dt class="font-medium">{item.question}</dt>
          <dd class="mt-2 text-sm leading-relaxed text-muted-foreground">{item.answer}</dd>
        </div>
      {/each}
    </dl>
  </section>

  <!-- Closing. The project's promise, plus the two repos this page sends you to. -->
  <section class="border-t border-border py-10">
    <p class="text-sm leading-relaxed text-muted-foreground">
      Free and open source — no tracking, no lock-in, and nothing to sign up for before your agent
      can search. Read every line of the
      <!-- eslint-disable svelte/no-navigation-without-resolve -- absolute GitHub URLs from $lib/cliLinks, not SvelteKit routes -->
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
      ><!-- eslint-enable svelte/no-navigation-without-resolve -->.
    </p>
  </section>
</div>

<style>
  /* Dotted hero background, faded toward the edges with a radial mask — the same
     device the homepage, /cli and /chatgpt heroes use, so this page reads as the
     same product. Component styles are scoped, so this is duplicated, not shared. */
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

  /* The hand-off card is the page's one action, so it gets the page's one extra
     flourish: a slow beam crossing its top edge, reading as a live wire rather
     than a static code block. Purely decorative and behind the content. */
  .handoff::before {
    content: '';
    position: absolute;
    inset-inline: 0.75rem;
    top: -1px;
    height: 1px;
    background: linear-gradient(
      90deg,
      transparent 0%,
      var(--foreground) 45%,
      var(--foreground) 55%,
      transparent 100%
    );
    background-size: 45% 100%;
    background-repeat: no-repeat;
    opacity: 0.5;
    animation: beam 5.5s cubic-bezier(0.6, 0, 0.4, 1) infinite;
  }
  @keyframes beam {
    0%,
    12% {
      background-position: -60% 0;
    }
    88%,
    100% {
      background-position: 160% 0;
    }
  }

  /* The caption dot pulses in time with the beam — one signal, two places. */
  .beam-dot {
    animation: pulse 5.5s ease-in-out infinite;
  }
  @keyframes pulse {
    0%,
    70%,
    100% {
      opacity: 1;
    }
    35% {
      opacity: 0.35;
    }
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
    .handoff::before,
    .beam-dot {
      animation: none;
    }
  }
</style>
