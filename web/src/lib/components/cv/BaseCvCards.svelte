<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { Copy, Download, FileText, Pencil, Trash2, ArrowRight } from '@lucide/svelte';
  import { api, ApiError } from '#lib/api.js';
  import { ConfirmDialog } from '#lib/ui/index.js';
  import { openCvIntake } from '#lib/cvIntakeDialog.svelte.js';
  import type { CvBaseItem, CvTemplate } from '#lib/cv.js';
  import { messages } from './BaseCvCards.messages';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, t } from '#lib/i18n/t.js';
  import { announce } from '#lib/announce.svelte.js';

  // The "Base CVs" block above the tailored list: one card per CV the candidate can edit by hand,
  // duplicate and tailor from. A tailored copy is made from one of these; this is where they
  // become visible. The card itself opens the workspace in base mode; the actions sit on it without opening it.

  let status = $state<'loading' | 'error' | 'ready'>('loading');
  let error = $state<string | null>(null);
  let bases = $state<CvBaseItem[]>([]);
  let templates = $state<CvTemplate[]>([]);
  let busy = $state(false);

  const s = $derived(t(messages, locale()));
  const fmt = (iso: string) => new Date(iso).toLocaleDateString();
  const templateLabel = (id: string) => templates.find((tpl) => tpl.id === id)?.label ?? id;

  onMount(async () => {
    try {
      bases = await api.listBaseCvs();
      status = 'ready';
    } catch (e) {
      error = e instanceof ApiError ? e.message : s.loadFailed;
      announce(error, 'assertive');
      status = 'error';
      return;
    }
    // Labels are a nicety: a failed read leaves the template id, not a broken block.
    templates = await api.listCvTemplates().catch(() => []);
  });

  async function duplicate(base: CvBaseItem) {
    if (busy) return;
    busy = true;
    try {
      const copy = await api.duplicateCv(base.id);
      await goto(resolve('/tailor/base/[id]', { id: copy.id }));
    } catch (e) {
      error = e instanceof ApiError ? e.message : s.duplicateFailed;
      announce(error, 'assertive');
    } finally {
      busy = false;
    }
  }

  async function createFromCv() {
    if (busy) return;
    busy = true;
    try {
      const created = await api.reseedBaseCv();
      await goto(resolve('/tailor/base/[id]', { id: created.id }));
    } catch (e) {
      error = e instanceof ApiError ? e.message : s.empty.failed;
      announce(error, 'assertive');
    } finally {
      busy = false;
    }
  }

  let removeTarget = $state<CvBaseItem | null>(null);
  let confirmRemoveOpen = $state(false);

  function requestRemove(base: CvBaseItem) {
    removeTarget = base;
    confirmRemoveOpen = true;
  }

  async function remove() {
    const base = removeTarget;
    if (!base) return;
    try {
      await api.deleteCv(base.id);
      // The default may have moved (it falls back to the oldest base); re-read rather than guess.
      bases = await api.listBaseCvs();
      error = null;
      announce(s.deleted);
    } catch (e) {
      error = e instanceof ApiError ? e.message : s.deleteFailed;
      announce(error, 'assertive');
    }
  }

  const iconButton =
    'flex size-9 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground';
</script>

<section class="space-y-3" aria-labelledby="base-cvs-heading">
  <div>
    <h2 id="base-cvs-heading" class="text-base font-semibold">{s.heading}</h2>
    <p class="text-sm text-muted-foreground">{s.description}</p>
  </div>

  {#if error}<p class="text-sm text-destructive">{error}</p>{/if}

  {#if status === 'loading'}
    <p class="text-muted-foreground">{s.loading}</p>
  {:else if status === 'ready' && bases.length === 0}
    <div class="rounded-lg border border-dashed border-border p-6 text-center">
      <FileText class="mx-auto h-8 w-8 text-muted-foreground" />
      <p class="mt-3 font-medium">{s.empty.title}</p>
      <p class="mx-auto mt-1 max-w-md text-sm text-muted-foreground">{s.empty.body}</p>
      <button
        type="button"
        onclick={createFromCv}
        disabled={busy}
        class="mt-4 inline-flex items-center gap-1.5 rounded-lg bg-foreground px-4 py-2 text-sm font-medium text-background transition-opacity hover:opacity-90 disabled:opacity-50"
      >
        {s.empty.cta} <ArrowRight class="size-4" />
      </button>
    </div>
  {:else if status === 'ready'}
    <ul class="flex flex-col gap-3">
      {#each bases as base (base.id)}
        <li
          class="group relative flex items-center gap-4 rounded-xl border border-border bg-card p-4 transition-colors hover:border-border/80 hover:bg-muted/30"
        >
          <!-- The whole card opens the editor; the actions below sit above it. -->
          <a href={resolve('/tailor/base/[id]', { id: base.id })} class="absolute inset-0" aria-label={format(s.openAria, { title: base.title })}></a>
          <img
            src={`/cv-previews/${base.template_id}.svg`}
            alt=""
            loading="lazy"
            class="h-16 w-12 shrink-0 rounded border border-border bg-white object-cover object-top"
          />
          <div class="min-w-0 flex-1">
            <p class="flex items-center gap-2">
              <span class="truncate font-medium">{base.title}</span>
              {#if base.is_default}
                <span class="shrink-0 rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">{s.defaultBadge}</span>
              {/if}
            </p>
            <p class="truncate text-sm text-muted-foreground">{format(s.templateLine, { template: templateLabel(base.template_id) })}</p>
            <p class="mt-0.5 text-xs text-muted-foreground/80">{format(s.updated, { date: fmt(base.updated_at) })}</p>
          </div>
          <div class="relative z-10 flex flex-wrap items-center justify-end gap-1">
            <a href={resolve('/tailor/base/[id]', { id: base.id })} class="inline-flex h-9 items-center gap-1.5 rounded-lg px-3 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground">
              <Pencil class="size-4" aria-hidden="true" />{s.edit}
            </a>
            <button
              type="button"
              onclick={() => openCvIntake(base.id)}
              class="inline-flex h-9 items-center gap-1.5 rounded-lg px-3 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            >
              {s.tailor}
            </button>
            <!-- eslint-disable svelte/no-navigation-without-resolve -- external CV PDF API URL, not an internal route -->
            <a href={api.cvPdfUrl(base.id)} target="_blank" rel="noopener" aria-label={s.openPdf} title={s.openPdf} class={iconButton}>
              <!-- eslint-enable svelte/no-navigation-without-resolve -->
              <Download class="h-4 w-4" />
            </a>
            <button type="button" aria-label={s.duplicate} title={s.duplicate} disabled={busy} onclick={() => duplicate(base)} class={iconButton}>
              <Copy class="h-4 w-4" />
            </button>
            <button type="button" aria-label={s.deleteTitle} title={s.deleteTitle} onclick={() => requestRemove(base)} class={iconButton}>
              <Trash2 class="h-4 w-4" />
            </button>
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</section>

<ConfirmDialog
  bind:open={confirmRemoveOpen}
  title={format(s.deleteDialog.title, { title: removeTarget?.title ?? '' })}
  description={s.deleteDialog.description}
  confirmLabel={s.deleteDialog.confirm}
  variant="destructive"
  onConfirm={remove}
/>
