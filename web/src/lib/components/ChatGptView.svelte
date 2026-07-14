<script lang="ts">
  import { resolve } from '$app/paths';
  import { Button } from '$lib/ui';

  // The published custom GPT. Public, so it opens for anyone with a ChatGPT
  // account; tracking actions additionally need the visitor's own API key.
  const GPT_URL = 'https://chatgpt.com/g/g-6a5281b64948819193bf3a1021e075da-freehire';

  // What the GPT can do, framed as the two halves of the job hunt: find, then
  // track. Mirrors the CLI page so the two agent surfaces read as one product.
  const discover = [
    { title: 'Search with real filters', body: 'Region, work mode, stack, seniority, salary — the GPT calls the live freehire search, not the open web.' },
    { title: 'Open a job or company', body: 'Full posting details, similar roles, and company context, straight from the catalogue.' },
    { title: 'Every result is real', body: 'Each job links back to its freehire page and the original apply URL — nothing invented.' },
  ];

  const track = [
    { title: 'Save & apply', body: 'Bookmark roles and mark the ones you applied to, by asking in plain language.' },
    { title: 'Move stages', body: 'Set applied → screening → interview → offer and add notes as you go.' },
    { title: 'Review your pipeline', body: 'Ask for your saved or applied jobs and the GPT reads them back from your account.' },
  ];
</script>

<div class="gpt">
  <!-- Hero. Left: the pitch. Right: a chat mockup — the ChatGPT analogue of the
       CLI page's terminal. -->
  <section class="grid-bg relative -mx-4 px-4 pb-16 pt-12 sm:pt-16">
    <div class="grid items-center gap-12 lg:grid-cols-[1.05fr_0.95fr]">
      <div>
        <p class="reveal font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground" style="--d:0ms">
          // freehire inside ChatGPT
        </p>

        <h1
          class="reveal mt-6 max-w-2xl text-balance text-4xl font-semibold leading-[0.98] tracking-tighter sm:text-6xl"
          style="--d:80ms"
        >
          Your job search,<br />inside ChatGPT.
        </h1>

        <p class="reveal mt-7 max-w-xl text-lg leading-relaxed text-muted-foreground" style="--d:160ms">
          <span class="text-foreground">freehire GPT</span> is a custom GPT wired to the same job API the
          site runs on. Ask for jobs in plain language and it searches the live
          <code class="font-mono text-foreground">freehire</code> catalogue — then saves, applies and tracks
          them on your account. (You still apply on the employer's site; the GPT records that you did.)
        </p>

        <div class="reveal mt-9 flex flex-wrap items-center gap-3" style="--d:240ms">
          <Button href={GPT_URL} target="_blank" rel="noopener noreferrer" variant="primary" size="lg">
            Open freehire GPT ↗
          </Button>
          <Button href={resolve('/my/api-keys')} variant="outline" size="lg">Get an API key</Button>
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
            Find remote senior backend Go jobs in Europe
          </div>
          <div class="max-w-[92%] rounded-2xl rounded-bl-sm border border-border bg-background px-3.5 py-2.5">
            <p class="text-muted-foreground">Here are open roles from freehire:</p>
            <p class="mt-2 font-medium text-foreground">Senior Go Backend Engineer — Wolt</p>
            <p class="text-xs text-muted-foreground">Remote · Helsinki / Stockholm · EU</p>
            <p class="mt-1 font-mono text-xs text-muted-foreground">freehire.dev/jobs/…-wolt</p>
          </div>
          <div class="self-end max-w-[85%] rounded-2xl rounded-br-sm bg-foreground px-3.5 py-2 text-background">
            Save it and mark me as applied
          </div>
        </div>
      </figure>
    </div>
  </section>

  <!-- What it does — the two halves, compact. -->
  <section class="border-t border-border py-14 sm:py-16">
    <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// what it does</p>
    <div class="mt-8 grid gap-x-12 gap-y-8 sm:grid-cols-2">
      <div>
        <h2 class="text-sm font-semibold uppercase tracking-wide text-muted-foreground">Find jobs</h2>
        <dl class="mt-4 space-y-4">
          {#each discover as row (row.title)}
            <div>
              <dt class="text-sm font-medium text-foreground">{row.title}</dt>
              <dd class="text-sm leading-relaxed text-muted-foreground">{row.body}</dd>
            </div>
          {/each}
        </dl>
      </div>
      <div>
        <h2 class="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
          Track applications
        </h2>
        <dl class="mt-4 space-y-4">
          {#each track as row (row.title)}
            <div>
              <dt class="text-sm font-medium text-foreground">{row.title}</dt>
              <dd class="text-sm leading-relaxed text-muted-foreground">{row.body}</dd>
            </div>
          {/each}
        </dl>
      </div>
    </div>
    <p class="mt-8 max-w-2xl text-sm leading-relaxed text-muted-foreground">
      Everything you save, apply to and stage shows up on your
      <a href={resolve('/my/tracking')} class="font-medium text-foreground underline-offset-4 hover:underline">Tracking</a>
      board — the same account the site and
      <a href={resolve('/cli')} class="font-medium text-foreground underline-offset-4 hover:underline">CLI</a>
      use.
    </p>
  </section>

  <!-- Setup — one step to unlock tracking. Search works without a key. -->
  <section class="border-t border-border py-14 sm:py-16">
    <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// setup</p>
    <ol class="mt-6 max-w-2xl space-y-3 text-sm leading-relaxed text-muted-foreground">
      <li>
        <span class="font-medium text-foreground">1.</span>
        <a href={GPT_URL} target="_blank" rel="noopener noreferrer" class="font-medium text-foreground underline-offset-4 hover:underline">Open freehire GPT</a>
        and start asking for jobs — search needs no setup.
      </li>
      <li>
        <span class="font-medium text-foreground">2.</span>
        To save, apply and track, create an
        <a href={resolve('/my/api-keys')} class="font-medium text-foreground underline-offset-4 hover:underline">API key</a>
        and paste it into the GPT's authentication field.
      </li>
    </ol>
    <p class="mt-6 max-w-2xl text-sm leading-relaxed text-muted-foreground">
      The GPT calls the same public endpoints documented in the
      <a href={resolve('/docs/api')} class="font-medium text-foreground underline-offset-4 hover:underline">API reference</a>.
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
