<script lang="ts">
  // The base CV editor: the same pieces the tailoring workspace edits with — the section form, the
  // live preview, the template gallery and the typography and margin settings — without a vacancy,
  // a chat or a match analysis, and with no model call at all.
  //
  // This address serves two kinds of CV. A BASE opens the editor. A TAILORED copy keeps the legacy
  // behaviour this page always had: redirect into its workspace, /tailor/<slug>?cv=<id>.
  import { onDestroy, onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { ArrowLeft, Download, RefreshCw } from '@lucide/svelte';
  import { api, ApiError } from '#lib/api.js';
  import { announce } from '#lib/announce.svelte.js';
  import { Button, ConfirmDialog } from '#lib/ui/index.js';
  import CvHtmlPreview from '#lib/tailor/CvHtmlPreview.svelte';
  import CvSectionForm from '#lib/components/cv/CvSectionForm.svelte';
  import MarginSettings from '#lib/components/cv/MarginSettings.svelte';
  import StyleSettings from '#lib/components/cv/StyleSettings.svelte';
  import TemplateGallery from '#lib/tailor/TemplateGallery.svelte';
  import { fitToWidthZoom } from '#lib/tailor/geometry.js';
  import { emptyDocument, toEditable, type CvFont, type CvRecord } from '#lib/cv.js';
  import type { Document } from '#lib/generated/contracts.js';
  import { messages } from './messages';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, plural, t } from '#lib/i18n/t.js';

  const id = $derived(page.params.id ?? '');
  const s = $derived(t(messages, locale()));

  let status = $state<'loading' | 'editor'>('loading');

  // The page owns the document: the form and the preview share one object, so typing re-renders
  // the preview at once and autosave persists it in the background.
  let doc = $state<Document>(emptyDocument());
  let title = $state('');
  let templateId = $state('classic-ats');
  let fonts = $state<CvFont[]>([]);
  let photoSrc = $state<string | null>(null);
  let loaded = $state(false);
  let tab = $state<'content' | 'design'>('content');

  function hydrate(rec: CvRecord) {
    title = rec.title;
    templateId = rec.template_id;
    doc = toEditable(rec.document);
    lastSnapshot = snapshot();
    loaded = true;
  }

  onMount(async () => {
    const cvId = id;
    try {
      const bases = await api.listBaseCvs();
      if (bases.some((b) => b.id === cvId)) {
        hydrate(await api.getCv(cvId));
        status = 'editor';
        void api.listCvFonts().then((f) => (fonts = f)).catch(() => {});
        void api
          .getPhoto()
          .then((meta) => {
            if (meta.present) photoSrc = `/api/v1/me/photo/image?v=${encodeURIComponent(meta.uploaded_at ?? '')}`;
          })
          .catch(() => {});
        return;
      }
      // Not a base of this account: a tailored copy goes to its workspace, anything else to the list.
      const match = (await api.listCvs()).find((cv) => cv.id === cvId);
      // eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() applied to both paths; the rule can't see through the appended ?cv= query
      await goto(match ? `${resolve('/tailor/[slug]', { slug: match.job_slug })}?cv=${cvId}` : resolve('/my/cvs'), { replaceState: true });
    } catch {
      await goto(resolve('/my/cvs'), { replaceState: true });
    }
  });

  // ---- Autosave: the same debounce the tailoring workspace uses, through the same endpoint. ----
  let lastSnapshot = '';
  const snapshot = () => JSON.stringify({ title, templateId, doc });
  let saveState = $state<'idle' | 'saving' | 'saved' | 'error'>('idle');
  let pdfVersion = $state(0);
  let saveTimer: ReturnType<typeof setTimeout> | null = null;

  async function persist() {
    const snap = snapshot(); // edits during the round-trip re-trigger the effect
    saveState = 'saving';
    try {
      await api.updateCv(id, { title, template_id: templateId, document: doc });
      lastSnapshot = snap;
      saveState = 'saved';
      pdfVersion += 1;
    } catch {
      saveState = 'error';
    }
  }

  $effect(() => {
    if (!loaded) return;
    if (snapshot() === lastSnapshot) return; // subscribes to title/templateId/doc
    if (saveTimer) clearTimeout(saveTimer);
    saveTimer = setTimeout(() => {
      saveTimer = null;
      void persist();
    }, 800);
  });

  onDestroy(() => {
    if (saveTimer) clearTimeout(saveTimer);
    if (loaded && snapshot() !== lastSnapshot) {
      void api.updateCv(id, { title, template_id: templateId, document: doc }).catch(() => {});
    }
  });

  // The gallery persists the switch itself; mirror the id so the next autosave does not undo it.
  function onTemplateSelected(next: string) {
    templateId = next;
    lastSnapshot = snapshot();
    pdfVersion += 1;
  }

  // ---- Preview ----
  const SCROLLER_PADDING = 48;
  let previewWidth = $state(0);
  const zoom = $derived(fitToWidthZoom(previewWidth - SCROLLER_PADDING));
  let pageCount = $state<number | null>(null);
  const pdfUrl = $derived(`${api.cvPdfUrl(id)}?v=${pdfVersion}`);

  // ---- Update from my CV: the explicit reseed of THIS base. ----
  let confirmRefreshOpen = $state(false);
  let refreshError = $state<string | null>(null);

  async function refreshFromCv() {
    refreshError = null;
    // Anything typed a moment ago must land first, or the debounce overwrites the reseed.
    if (saveTimer) {
      clearTimeout(saveTimer);
      saveTimer = null;
      if (snapshot() !== lastSnapshot) await persist();
    }
    try {
      hydrate(await api.reseedCv(id));
      pdfVersion += 1;
    } catch (e) {
      refreshError = e instanceof ApiError ? e.message : s.refreshFailed;
      announce(refreshError, 'assertive');
    }
  }
</script>

<svelte:head><title>{status === 'editor' ? s.editorTitle : s.headTitle}</title></svelte:head>

{#if status === 'loading'}
  <p class="text-muted-foreground">{s.opening}</p>
{:else}
  <div class="flex flex-col gap-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <a
        href={resolve('/my/cvs')}
        class="inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
      >
        <ArrowLeft class="size-4" aria-hidden="true" />{s.back}
      </a>
      <div class="flex items-center gap-3">
        <span class="text-xs text-muted-foreground" role="status" aria-live="polite">
          {#if saveState === 'saving'}{s.save.saving}{:else if saveState === 'saved'}{s.save.saved}{:else if saveState === 'error'}<span class="text-destructive">{s.save.error}</span>{/if}
        </span>
        <Button variant="outline" onclick={() => (confirmRefreshOpen = true)}>
          <RefreshCw class="size-4" aria-hidden="true" />{s.refresh}
        </Button>
        <!-- eslint-disable svelte/no-navigation-without-resolve -- external CV PDF API URL, not an internal route -->
        <a
          href={pdfUrl}
          target="_blank"
          rel="noopener"
          class="inline-flex items-center gap-1.5 rounded-md border border-border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-muted"
        >
          <!-- eslint-enable svelte/no-navigation-without-resolve -->
          <Download class="size-4" aria-hidden="true" />{s.pdf}
        </a>
      </div>
    </div>
    {#if refreshError}<p class="text-sm text-destructive">{refreshError}</p>{/if}

    <div class="grid gap-6 lg:grid-cols-2">
      <div class="flex min-w-0 flex-col gap-4">
        <div class="flex gap-4 border-b border-border text-sm" role="tablist">
          {#each [{ id: 'content', label: s.tabs.content }, { id: 'design', label: s.tabs.design }] as option (option.id)}
            <button
              type="button"
              role="tab"
              aria-selected={tab === option.id}
              onclick={() => (tab = option.id as typeof tab)}
              class="-mb-px border-b-2 px-1 py-2 transition-colors {tab === option.id
                ? 'border-brand font-medium text-foreground'
                : 'border-transparent text-muted-foreground hover:text-foreground'}"
            >
              {option.label}
            </button>
          {/each}
        </div>

        {#if tab === 'content'}
          <CvSectionForm bind:doc bind:title />
        {:else}
          <div class="space-y-8">
            <section class="space-y-3">
              <h2 class="text-sm font-semibold text-foreground">{s.template}</h2>
              <TemplateGallery cvId={id} onSelected={onTemplateSelected} />
            </section>
            <section class="space-y-3">
              <h2 class="text-sm font-semibold text-foreground">{s.typography}</h2>
              <StyleSettings bind:style={doc.style} {fonts} />
            </section>
            <section class="space-y-3">
              <h2 class="text-sm font-semibold text-foreground">
                {s.margins} <span class="font-normal text-muted-foreground">{s.marginsUnit}</span>
              </h2>
              <MarginSettings bind:margins={doc.margins} />
            </section>
          </div>
        {/if}
      </div>

      <div class="min-w-0">
        {#if pageCount !== null}
          <p class="mb-2 text-xs tabular-nums text-muted-foreground">
            {format(plural(locale(), pageCount, s.pages), { count: String(pageCount) })}
          </p>
        {/if}
        <div bind:clientWidth={previewWidth} class="max-h-screen overflow-auto rounded-lg border border-border bg-muted/30 p-6">
          <CvHtmlPreview {doc} {templateId} {zoom} {fonts} {photoSrc} onPageCount={(n) => (pageCount = n)} />
        </div>
      </div>
    </div>
  </div>

  <ConfirmDialog
    bind:open={confirmRefreshOpen}
    title={s.refreshDialog.title}
    description={s.refreshDialog.description}
    confirmLabel={s.refreshDialog.confirm}
    variant="destructive"
    onConfirm={refreshFromCv}
  />
{/if}
