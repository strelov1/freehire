<script lang="ts">
  import { Badge } from '$lib/ui';
  import DocsCodeBlock from './DocsCodeBlock.svelte';
  import { BASE_URL, AUTH_LABELS, type Auth, type Endpoint, type Param } from '$lib/docs/api-spec';
  import { METHOD_CHIP, inlineCode } from '$lib/docs/format';

  // One endpoint, rendered as a focused page: signature, prose, parameters, then
  // full-width request/response code. `curlHtml`/`responseHtml` are Shiki output
  // precomputed in `load`; absent, DocsCodeBlock falls back to plain text.
  let {
    endpoint: ep,
    groupTitle,
    curlHtml = undefined,
    responseHtml = undefined,
    responseLabel = 'json — response',
  }: {
    endpoint: Endpoint;
    groupTitle: string;
    curlHtml?: string;
    responseHtml?: string;
    responseLabel?: string;
  } = $props();

  const authVariant = (a: Auth) => (a === 'none' ? 'secondary' : 'outline');

  // Path split into segments, so the slashes can be dimmed and the leaf emphasised.
  const segs = $derived(ep.path.replace(/^\//, '').split('/'));

  let copied = $state(false);
  let timer: ReturnType<typeof setTimeout> | undefined;
  async function copyPath() {
    try {
      await navigator.clipboard.writeText(`${BASE_URL}${ep.path}`);
      copied = true;
      clearTimeout(timer);
      timer = setTimeout(() => (copied = false), 1600);
    } catch {
      // Clipboard can be blocked (insecure context / no permission); the path is
      // visible to select by hand, so a failed copy needs no fallback.
    }
  }
</script>

<article class="min-w-0 max-w-3xl">
  <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">{groupTitle}</p>

  <!-- Signature pill. -->
  <div class="mt-4 flex flex-wrap items-center gap-2 rounded-xl border border-border bg-secondary/30 p-2 pl-2.5">
    <span
      class={`inline-flex items-center rounded-md px-2 py-1 font-mono text-[11px] font-bold uppercase tracking-wide ring-1 ring-inset ${METHOD_CHIP[ep.method]}`}
    >
      {ep.method}
    </span>
    <span class="font-mono text-sm">
      {#each segs as seg, i (i)}<span class="text-muted-foreground/50">/</span><span
          class={i === segs.length - 1 ? 'text-foreground' : 'text-muted-foreground'}>{seg}</span
        >{/each}
    </span>
    <button
      type="button"
      onclick={copyPath}
      class="ml-auto rounded-md border border-border px-2 py-1 text-[11px] font-medium text-muted-foreground transition-colors hover:text-foreground"
      aria-label="Copy endpoint URL"
    >
      {copied ? 'copied ✓' : 'copy'}
    </button>
    <Badge variant={authVariant(ep.auth)}>{AUTH_LABELS[ep.auth]}</Badge>
  </div>

  <p class="mt-5 text-lg leading-relaxed text-foreground">{ep.summary}</p>
  {#if ep.description}
    <p class="mt-3 leading-relaxed text-muted-foreground">{@html inlineCode(ep.description)}</p>
  {/if}

  {#if ep.pathParams?.length}
    {@render paramFields('Path parameters', ep.pathParams)}
  {/if}
  {#if ep.query?.length}
    {@render paramFields('Query parameters', ep.query)}
  {/if}
  {#if ep.filterable}
    <p class="mt-4 text-sm text-muted-foreground">
      Plus every filter in
      <a href="/docs/api#filtering-jobs" class="font-medium text-brand-strong underline-offset-4 hover:underline"
        >Filtering jobs</a
      >.
    </p>
  {/if}
  {#if ep.body?.length}
    {@render paramFields('Body', ep.body)}
  {/if}

  <!-- Full-width request / response code. -->
  <div class="mt-8 space-y-3">
    <DocsCodeBlock code={ep.curl} label="curl" html={curlHtml} />
    {#if ep.responseExample}
      <DocsCodeBlock code={ep.responseExample} label={responseLabel} html={responseHtml} />
    {/if}
  </div>
</article>

{#snippet paramFields(title: string, params: Param[])}
  <div class="mt-6">
    <p class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">{title}</p>
    <dl class="mt-2 divide-y divide-border/60 border-t border-border/60">
      {#each params as p (p.name)}
        <div class="py-3">
          <dt class="flex flex-wrap items-baseline gap-x-2 gap-y-1">
            <code class="font-mono text-sm font-medium text-foreground">{p.name}</code>
            <span class="rounded bg-secondary px-1.5 py-0.5 font-mono text-[11px] text-muted-foreground">{p.type}</span>
            {#if p.required}
              <span class="rounded bg-brand-muted px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-brand-strong"
                >required</span
              >
            {/if}
          </dt>
          <dd class="mt-1.5 text-sm leading-relaxed text-muted-foreground">
            {@html inlineCode(p.description)}{#if p.example}<span class="ml-1">Example
                <code class="rounded bg-secondary px-1 py-0.5 font-mono text-[12px] text-foreground/80">{p.example}</code
                ></span
              >{/if}
          </dd>
        </div>
      {/each}
    </dl>
  </div>
{/snippet}
