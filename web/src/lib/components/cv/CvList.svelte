<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { FileText, Download, Trash2, ArrowRight, Copy, Pencil } from '@lucide/svelte';
  import { api, ApiError } from '#lib/api.js';
  import { companyLogoUrl } from '#lib/logo.js';
  import { ConfirmDialog, EntityLogo } from '#lib/ui/index.js';
  import { openCvIntake } from '#lib/cvIntakeDialog.svelte.js';
  import type { CvBaseItem, CvTailoredItem, CvTemplate } from '#lib/cv.js';
  import { messages } from './CvList.messages';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { formatDate } from '#lib/utils.js';
  import { format, t } from '#lib/i18n/t.js';
  import { announce } from '#lib/announce.svelte.js';

  // The CV landing: the candidate's BASE CVs (the general CVs tailored copies start from) above one
  // company card per CV built for a vacancy, styled like the saved-jobs cards. Both kinds open in
  // the same workspace (a base in its base mode, a tailored copy in its vacancy mode, each
  // resuming its agent session); the PDF, duplicate and delete actions sit on the card without
  // triggering the open.

  let status = $state<'loading' | 'error' | 'ready'>('loading');
  // `listError` is the tailored list failing to load and stays while it is failed; `error` is the
  // outcome of an action (duplicate, delete) and is cleared by the next success.
  let listError = $state<string | null>(null);
  let error = $state<string | null>(null);
  let items = $state<CvTailoredItem[]>([]);
  let bases = $state<CvBaseItem[]>([]);
  // False until the base list has loaded (and after it failed): the empty state must not claim
  // "no base CV yet" about a list we could not read.
  let basesLoaded = $state(false);
  let baseError = $state<string | null>(null);
  let templates = $state<CvTemplate[]>([]);
  let busy = $state(false);

  const s = $derived(t(messages, locale()));

  onMount(load);

  async function load() {
    status = 'loading';
    // Independent reads, each shown as soon as it settles: a slow or failed base list must not
    // hold back or hide the tailored CVs, nor the reverse.
    await Promise.all([loadTailored(), loadBases(), loadTemplates()]);
  }

  async function loadTailored() {
    try {
      items = await api.listCvs();
      status = 'ready';
    } catch (e) {
      listError = e instanceof ApiError ? e.message : s.loadFailed;
      announce(listError, 'assertive');
      status = 'error';
    }
  }

  async function loadBases() {
    try {
      bases = await api.listBaseCvs();
      basesLoaded = true;
    } catch (e) {
      basesLoaded = false;
      baseError = e instanceof ApiError ? e.message : s.base.loadFailed;
    }
  }

  // Labels are a nicety: a failed read leaves the template id, not a broken list.
  async function loadTemplates() {
    templates = await api.listCvTemplates().catch(() => []);
  }

  const templateLabel = (id: string) => templates.find((tpl) => tpl.id === id)?.label ?? id;

  async function duplicate(base: CvBaseItem) {
    if (busy) return;
    busy = true;
    try {
      const copy = await api.duplicateCv(base.id);
      await goto(resolve('/tailor/base/[id]', { id: copy.id }));
    } catch (e) {
      error = e instanceof ApiError ? e.message : s.base.duplicateFailed;
      announce(error, 'assertive');
    } finally {
      busy = false;
    }
  }

  // The reseed endpoint creates the first base from the candidate's CV when there is none.
  async function createFromCv() {
    if (busy) return;
    busy = true;
    try {
      const created = await api.reseedBaseCv();
      await goto(resolve('/tailor/base/[id]', { id: created.id }));
    } catch (e) {
      error = e instanceof ApiError ? e.message : s.base.empty.failed;
      announce(error, 'assertive');
    } finally {
      busy = false;
    }
  }

  // One confirm for both kinds; `base` picks the wording (a base's tailored copies keep working).
  let removeTarget = $state<{ id: string; title: string; base: boolean } | null>(null);
  let confirmRemoveOpen = $state(false);

  function requestRemove(target: { id: string; title: string; base: boolean }) {
    removeTarget = target;
    confirmRemoveOpen = true;
  }

  async function remove() {
    const cv = removeTarget;
    if (!cv) return;
    try {
      await api.deleteCv(cv.id);
      if (cv.base) {
        // The default may have moved (it falls back to the oldest base); re-read rather than guess.
        bases = await api.listBaseCvs();
      } else {
        items = items.filter((i) => i.id !== cv.id);
      }
      error = null;
      announce(s.deleted);
    } catch (e) {
      error = e instanceof ApiError ? e.message : s.deleteFailed;
      announce(error, 'assertive');
    }
  }

  // The site's one date format ("Oct 27, 2026"), not the browser's numeric default.
  const fmt = (iso: string) => formatDate(iso, locale());
  const iconButton =
    'flex size-9 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground';
</script>

<div class="space-y-8">
  {#if error}<p class="text-sm text-destructive">{error}</p>{/if}

  {#if status === 'loading'}
    <p class="text-muted-foreground">{s.loading}</p>
  {/if}

  <section class="space-y-3" aria-labelledby="base-cvs-heading">
    <div>
      <h2 id="base-cvs-heading" class="text-base font-semibold">{s.base.heading}</h2>
      <p class="text-sm text-muted-foreground">{s.base.description}</p>
    </div>
    {#if baseError}
      <p class="text-sm text-destructive">{baseError}</p>
    {:else if basesLoaded && bases.length === 0}
      <div class="rounded-lg border border-dashed border-border p-6 text-center">
        <FileText class="mx-auto h-8 w-8 text-muted-foreground" />
        <p class="mt-3 font-medium">{s.base.empty.title}</p>
        <p class="mx-auto mt-1 max-w-md text-sm text-muted-foreground">{s.base.empty.body}</p>
        <button
          type="button"
          onclick={createFromCv}
          disabled={busy}
          class="mt-4 inline-flex items-center gap-1.5 rounded-lg bg-foreground px-4 py-2 text-sm font-medium text-background transition-opacity hover:opacity-90 disabled:opacity-50"
        >
          {s.base.empty.cta}
          <ArrowRight class="size-4" />
        </button>
      </div>
    {:else if basesLoaded}
      <ul class="flex flex-col gap-3">
        {#each bases as base (base.id)}
          <li
            class="group relative flex flex-wrap items-center gap-x-4 gap-y-3 rounded-xl border border-border bg-card p-4 transition-colors hover:border-border/80 hover:bg-muted/30"
          >
            <!-- The whole card opens the workspace in base mode; the actions below sit above it. -->
            <a
              href={resolve('/tailor/base/[id]', { id: base.id })}
              class="absolute inset-0"
              aria-label={format(s.openAria, { title: base.title })}
            ></a>
            <img
              src={`/cv-previews/${base.template_id}.svg`}
              alt=""
              loading="lazy"
              class="h-16 w-12 shrink-0 rounded border border-border bg-white object-cover object-top"
            />
            <div class="min-w-0 flex-1">
              <p class="flex flex-wrap items-center gap-x-2 gap-y-1">
                <span class="truncate font-medium">{base.title}</span>
                {#if base.is_default}
                  <span class="shrink-0 rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground"
                    >{s.base.defaultBadge}</span
                  >
                {/if}
              </p>
              <p class="truncate text-sm text-muted-foreground">
                {format(s.base.templateLine, { template: templateLabel(base.template_id) })}
              </p>
              <p class="mt-0.5 text-xs text-muted-foreground/80">{format(s.updated, { date: fmt(base.updated_at) })}</p>
            </div>
            <div
              class="relative z-10 flex w-full flex-wrap items-center justify-between gap-2 border-t border-border pt-3"
            >
              <div class="flex flex-wrap items-center gap-1">
                <a
                  href={resolve('/tailor/base/[id]', { id: base.id })}
                  class="inline-flex h-9 items-center gap-1.5 rounded-lg px-3 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                >
                  <Pencil class="size-4" aria-hidden="true" />{s.base.edit}
                </a>
                <button
                  type="button"
                  onclick={() => openCvIntake(base.id)}
                  class="inline-flex min-h-9 items-center justify-center gap-1.5 rounded-lg px-3 py-2 text-center text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                >
                  {s.base.tailor}
                </button>
              </div>
              <div class="ml-auto flex shrink-0 items-center gap-1">
                <!-- eslint-disable svelte/no-navigation-without-resolve -- external CV PDF API URL, not an internal route -->
                <a
                  href={api.cvPdfUrl(base.id)}
                  target="_blank"
                  rel="noopener"
                  aria-label={s.openPdf}
                  title={s.openPdf}
                  class={iconButton}
                >
                  <!-- eslint-enable svelte/no-navigation-without-resolve -->
                  <Download class="h-4 w-4" />
                </a>
                <button
                  type="button"
                  aria-label={s.base.duplicate}
                  title={s.base.duplicate}
                  disabled={busy}
                  onclick={() => duplicate(base)}
                  class={iconButton}
                >
                  <Copy class="h-4 w-4" />
                </button>
                <button
                  type="button"
                  aria-label={s.deleteTitle}
                  title={s.deleteTitle}
                  onclick={() => requestRemove({ id: base.id, title: base.title, base: true })}
                  class={iconButton}
                >
                  <Trash2 class="h-4 w-4" />
                </button>
              </div>
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  </section>

  <h2 class="text-base font-semibold">{s.heading}</h2>

  {#if listError}<p class="text-sm text-destructive">{listError}</p>{/if}

  {#if status === 'ready' && items.length === 0}
    <div class="rounded-lg border border-dashed border-border p-8 sm:p-10">
      <div class="mx-auto max-w-md">
        <FileText class="mx-auto h-8 w-8 text-muted-foreground" />
        <p class="mt-3 text-center font-medium">{s.empty.title}</p>
        <p class="mt-1 text-center text-sm text-muted-foreground">
          {s.empty.body}
        </p>
        <ol class="mx-auto mt-5 flex max-w-sm flex-col gap-3 text-left text-sm">
          <li class="flex items-start gap-3">
            <span class="flex size-6 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-semibold"
              >1</span
            >
            <span>{s.empty.step1}</span>
          </li>
          <li class="flex items-start gap-3">
            <span class="flex size-6 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-semibold"
              >2</span
            >
            <span
              >{s.empty.step2Lead} <strong class="font-medium text-foreground">{s.empty.tailorMyCv}</strong>
              {s.empty.step2Tail}</span
            >
          </li>
          <li class="flex items-start gap-3">
            <span class="flex size-6 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-semibold"
              >3</span
            >
            <span
              >{s.empty.step3Lead} <strong class="font-medium text-foreground">{s.empty.tailorMyCv}</strong>
              {s.empty.step3Tail}</span
            >
          </li>
        </ol>
        <div class="mt-6 text-center">
          <button
            type="button"
            onclick={() => openCvIntake()}
            class="inline-flex items-center gap-1.5 rounded-lg bg-foreground px-4 py-2 text-sm font-medium text-background transition-opacity hover:opacity-90"
          >
            {s.empty.cta}
            <ArrowRight class="size-4" />
          </button>
        </div>
      </div>
    </div>
  {:else}
    <ul class="flex flex-col gap-3">
      {#each items as cv (cv.id)}
        <li
          class="group relative flex items-center gap-4 rounded-xl border border-border bg-card p-4 transition-colors hover:border-border/80 hover:bg-muted/30"
        >
          <!-- The whole card opens the workspace; the action buttons stop propagation below. -->
          <!-- eslint-disable svelte/no-navigation-without-resolve -- resolve() applied to the path; the rule can't see through the appended ?cv= query -->
          <a
            href={`${resolve('/tailor/[slug]', { slug: cv.job_slug })}?cv=${cv.id}`}
            class="absolute inset-0"
            aria-label={format(s.openAria, { title: cv.job_title })}
          ></a><!-- eslint-enable svelte/no-navigation-without-resolve -->
          <EntityLogo
            name={cv.job_company}
            src={companyLogoUrl(cv.job_company) ?? undefined}
            shape="square"
            size="lg"
          />
          <div class="min-w-0 flex-1">
            <p class="truncate font-medium">{cv.job_title}</p>
            <p class="truncate text-sm text-muted-foreground">{cv.job_company}</p>
            <p class="mt-0.5 text-xs text-muted-foreground/80">{format(s.updated, { date: fmt(cv.updated_at) })}</p>
          </div>
          <div class="relative z-10 flex items-center gap-1">
            <!-- eslint-disable svelte/no-navigation-without-resolve -- external CV PDF API URL, not an internal route -->
            <a
              href={api.cvPdfUrl(cv.id)}
              target="_blank"
              rel="noopener"
              aria-label={s.openPdf}
              title={s.openPdf}
              class="flex size-9 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            >
              <!-- eslint-enable svelte/no-navigation-without-resolve -->
              <Download class="h-4 w-4" />
            </a>
            <button
              type="button"
              aria-label={s.deleteTitle}
              title={s.deleteTitle}
              onclick={() => requestRemove({ id: cv.id, title: cv.job_title, base: false })}
              class="flex size-9 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            >
              <Trash2 class="h-4 w-4" />
            </button>
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<ConfirmDialog
  bind:open={confirmRemoveOpen}
  title={format(removeTarget?.base ? s.base.deleteDialog.title : s.deleteDialog.title, {
    title: removeTarget?.title ?? '',
  })}
  description={removeTarget?.base ? s.base.deleteDialog.description : s.deleteDialog.description}
  confirmLabel={s.deleteDialog.confirm}
  variant="destructive"
  onConfirm={remove}
/>
