<script lang="ts">
  import { resolve } from '$app/paths';
  import { Button } from '#lib/ui/index.js';
  import { CHATGPT_APP_URL, OPENAI_MARK } from '#lib/appLinks.js';
  import { glyphColorFor } from '#lib/markColor.js';

  // The older custom GPT over the REST API. Still published, and still the one way to
  // track from ChatGPT with an API key instead of an OAuth connector.
  const GPT_URL = 'https://chatgpt.com/g/g-6a5281b64948819193bf3a1021e075da-freehire';

  // What the app answers, one line each. Every row is a tool the published app's
  // server (`/api/v1/mcp`, internal/api/mcpapp) actually has.
  const answers = [
    { title: 'Search with real filters', body: 'Region, work mode, stack, seniority, salary — ChatGPT searches the live freehire catalogue, not the open web.' },
    { title: 'Jobs as cards', body: 'Results and postings render as cards right in the chat, each linking to its freehire page and the employer.' },
    { title: 'Is it still open?', body: 'Paste a shortlist and ChatGPT checks which postings are still open and which have closed.' },
    { title: 'Similar roles', body: 'Ask for more like the one you picked — the same neighbours the job page shows.' },
    { title: 'Salary ranges', body: 'What a role pays by seniority, from postings that state a salary.' },
    { title: 'Market breakdowns', body: 'Which skills, countries and work modes a search turns up most.' },
  ];
</script>

<div class="gpt">
  <!-- Hero. Left: the pitch and the one-click install. Right: a chat mockup. -->
  <section class="grid-bg relative -mx-4 px-4 pb-16 pt-12 sm:pt-16">
    <div class="grid items-center gap-12 lg:grid-cols-[1.05fr_0.95fr]">
      <div>
        <p class="reveal font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground" style="--d:0ms">
          // freehire app for ChatGPT
        </p>

        <h1
          class="reveal mt-6 max-w-2xl text-balance text-4xl font-semibold leading-[0.98] tracking-tighter sm:text-6xl"
          style="--d:80ms"
        >
          Your job search,<br />inside ChatGPT.
        </h1>

        <p class="reveal mt-7 max-w-xl text-lg leading-relaxed text-muted-foreground" style="--d:160ms">
          freehire is in ChatGPT's app directory. Add it in one click and ask for jobs in plain language —
          ChatGPT searches the live <code class="font-mono text-foreground">freehire</code> catalogue and shows
          the results as cards. Free, and no freehire account needed.
        </p>

        <div class="reveal mt-9 flex flex-wrap items-center gap-3" style="--d:240ms">
          <!-- eslint-disable svelte/no-navigation-without-resolve -- external app directory URL, not a SvelteKit route -->
          <a
            href={CHATGPT_APP_URL}
            target="_blank"
            rel="noopener noreferrer"
            class="inline-flex h-11 items-center gap-2 rounded-lg px-5 text-base font-medium transition-opacity hover:opacity-90"
            style:background-color="#{OPENAI_MARK.hex}"
            style:color={glyphColorFor(`#${OPENAI_MARK.hex}`)}
          >
            <svg viewBox={OPENAI_MARK.viewBox} class="size-5 fill-current" aria-hidden="true"
              ><path d={OPENAI_MARK.path} /></svg
            >
            Add to ChatGPT ↗
          </a>
          <!-- eslint-enable svelte/no-navigation-without-resolve -->
          <Button href={resolve('/mcp')} variant="outline" size="lg">Other assistants</Button>
        </div>
      </div>

      <figure
        class="reveal overflow-hidden rounded-xl border border-border bg-secondary/60 text-sm shadow-sm"
        style="--d:320ms"
      >
        <figcaption
          class="flex items-center gap-2 border-b border-border px-4 py-2.5 text-xs text-muted-foreground"
        >
          <span class="size-2.5 rounded-full bg-muted-foreground/30"></span>
          ChatGPT · freehire
        </figcaption>
        <div class="flex flex-col gap-4 p-4 leading-relaxed">
          <div class="self-end max-w-[85%] rounded-2xl rounded-br-sm bg-foreground px-3.5 py-2 text-background">
            @freehire remote senior backend Go jobs in Europe
          </div>
          <div class="max-w-[92%] rounded-2xl rounded-bl-sm border border-border bg-background px-3.5 py-2.5">
            <p class="text-muted-foreground">Here are open roles from freehire:</p>
            <p class="mt-2 font-medium text-foreground">Senior Go Engineer — payments platform</p>
            <p class="text-xs text-muted-foreground">Remote · EU · €85–100k</p>
            <p class="mt-2 font-medium text-foreground">Senior Backend Engineer (Go, Kafka)</p>
            <p class="text-xs text-muted-foreground">Remote · Germany, Netherlands</p>
          </div>
          <div class="self-end max-w-[85%] rounded-2xl rounded-br-sm bg-foreground px-3.5 py-2 text-background">
            Which of these are still open, and what do they pay?
          </div>
        </div>
      </figure>
    </div>
  </section>

  <!-- What it answers. -->
  <section class="border-t border-border py-14 sm:py-16">
    <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// what it answers</p>
    <dl class="mt-8 grid gap-x-12 gap-y-6 sm:grid-cols-2 lg:grid-cols-3">
      {#each answers as row (row.title)}
        <div>
          <dt class="text-sm font-medium text-foreground">{row.title}</dt>
          <dd class="text-sm leading-relaxed text-muted-foreground">{row.body}</dd>
        </div>
      {/each}
    </dl>
  </section>

  <!-- Setup. -->
  <section class="border-t border-border py-14 sm:py-16">
    <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// setup</p>
    <ol class="mt-6 max-w-2xl space-y-3 text-sm leading-relaxed text-muted-foreground">
      <li>
        <span class="font-medium text-foreground">1.</span>
        <!-- eslint-disable svelte/no-navigation-without-resolve -- external app directory URL, not a SvelteKit route -->
        <a
          href={CHATGPT_APP_URL}
          target="_blank"
          rel="noopener noreferrer"
          class="font-medium text-foreground underline-offset-4 hover:underline">Open freehire in ChatGPT</a
        ><!-- eslint-enable svelte/no-navigation-without-resolve -->
        and choose Add.
      </li>
      <li>
        <span class="font-medium text-foreground">2.</span>
        In a new chat, pick freehire from the + menu (or type @freehire) and ask a job question.
      </li>
    </ol>

    <!-- The app is read-only, so this is where saving and applying live. -->
    <h2 class="mt-10 text-sm font-semibold uppercase tracking-wide text-muted-foreground">Save and track</h2>
    <p class="mt-4 max-w-2xl text-sm leading-relaxed text-muted-foreground">
      The app reads the catalogue and changes nothing of yours. To have ChatGPT save jobs, mark them applied
      and move stages on your
      <a href={resolve('/my/tracking')} class="font-medium text-foreground underline-offset-4 hover:underline"
        >Tracking</a
      >
      board, add the account connector instead — it signs in with freehire the first time it is used. The
      <a href={resolve('/mcp')} class="font-medium text-foreground underline-offset-4 hover:underline"
        >setup guide</a
      > walks through it.
    </p>
    <p class="mt-4 max-w-2xl text-sm leading-relaxed text-muted-foreground">
      Prefer an API key? The older
      <!-- eslint-disable svelte/no-navigation-without-resolve -- external GPT URL, not a SvelteKit route -->
      <a
        href={GPT_URL}
        target="_blank"
        rel="noopener noreferrer"
        class="font-medium text-foreground underline-offset-4 hover:underline">freehire GPT ↗</a
      ><!-- eslint-enable svelte/no-navigation-without-resolve -->
      still works: paste an
      <a
        href={resolve('/my/integrations/api-keys')}
        class="font-medium text-foreground underline-offset-4 hover:underline">API key</a
      >
      into its authentication field. It calls the endpoints in the
      <a href={resolve('/docs/api')} class="font-medium text-foreground underline-offset-4 hover:underline"
        >API reference</a
      >.
    </p>
  </section>

  <!-- Free / open-source — the project's promise, with the source. -->
  <section class="border-t border-border py-10">
    <p class="text-sm leading-relaxed text-muted-foreground">
      Free and open source — no tracking, no lock-in. Read every line on
      <a
        href="https://github.com/strelov1/freehire"
        target="_blank"
        rel="noopener noreferrer"
        class="font-medium text-foreground underline-offset-4 hover:underline">GitHub ↗</a
      >.
    </p>
  </section>
</div>

<style>
  /* Dotted hero background, faded toward the edges with a radial mask — the same
     device the homepage and CLI hero use, so the page reads as the same product.
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
