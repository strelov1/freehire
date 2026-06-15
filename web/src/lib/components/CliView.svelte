<script lang="ts">
  import { Button } from '$lib/ui';

  const CLI_REPO = 'https://github.com/strelov1/freehire-cli';
  const INSTALL = 'curl -fsSL https://freehire.dev/install.sh | sh';

  // Command reference, mirroring the freehire-cli README verbatim. Two groups:
  // first you find jobs, then you track your interaction with them.
  const discover = [
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
        <p class="reveal font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground" style="--d:0ms">
          // freehire on the command line
        </p>

        <h1
          class="reveal mt-6 max-w-2xl text-balance text-4xl font-semibold leading-[0.98] tracking-tighter sm:text-6xl"
          style="--d:80ms"
        >
          Search and track<br />from the terminal.
        </h1>

        <p class="reveal mt-7 max-w-xl text-lg leading-relaxed text-muted-foreground" style="--d:160ms">
          <code class="font-mono text-foreground">freehire</code> is a small CLI over the same job API the
          site runs on — so an <span class="text-foreground">AI agent</span> or a script can search and open
          jobs, then track applications and notes without a browser. (You still apply on the employer's
          site; the CLI records that you did.)
        </p>

        <div class="reveal mt-9 flex flex-wrap items-center gap-3" style="--d:240ms">
          <Button href="/my/api-keys" variant="primary" size="lg">Get an API key</Button>
          <Button href={CLI_REPO} target="_blank" rel="noopener noreferrer" variant="outline" size="lg">
            Source ↗
          </Button>
        </div>
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
curl -fsSL <span class="text-foreground">https://freehire.dev/install.sh</span> | sh

<span class="text-muted-foreground"># authenticate once (key from /my/api-keys)</span>
freehire auth login --token <span class="text-foreground">fhk_…</span>

<span class="text-muted-foreground"># search</span>
freehire search <span class="text-foreground">"golang"</span> --remote --region eu</pre>
      </figure>
    </div>
  </section>

  <!-- Commands — the whole reference, compact. -->
  <section class="border-t border-border py-14 sm:py-16">
    <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// commands</p>
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
    </div>
    <p class="mt-8 max-w-2xl text-sm leading-relaxed text-muted-foreground">
      Everything you save, apply to and stage shows up on your
      <a href="/my/jobs" class="font-medium text-foreground underline-offset-4 hover:underline">My jobs</a>
      board.
      <code class="font-mono text-foreground">stage</code> takes a controlled value:
      <span class="font-mono text-foreground"
        >applied → screening → responded → interview → offer → accepted</span
      >, plus <span class="font-mono text-foreground">rejected</span> /
      <span class="font-mono text-foreground">withdrawn</span>. For scripts and agents, add
      <code class="font-mono text-foreground">--json</code> for the raw API payload; results go to stdout,
      errors to stderr, and a non-zero exit code signals failure.
    </p>
  </section>

  <!-- Moderators — gated authoring, kept to one line + two commands. -->
  <section class="border-t border-border py-14 sm:py-16">
    <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// moderators</p>
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
      Free and open source — no tracking, no lock-in. Read every line on
      <a
        href={CLI_REPO}
        target="_blank"
        rel="noopener noreferrer"
        class="font-medium text-foreground underline-offset-4 hover:underline">GitHub ↗</a
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
