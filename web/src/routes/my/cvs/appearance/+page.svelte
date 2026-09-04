<script lang="ts">
  // Personal defaults for a NEW CV's template/typography/margins. Reuses the same controls
  // the tailoring workspace uses on an individual CV, but nothing here writes to any CV: it
  // only seeds the template/style/margins a base CV starts with the next time one is
  // created — an existing CV's own appearance is untouched by saving here.
  import { onMount } from 'svelte';
  import { api, ApiError } from '$lib/api';
  import type { Margins, Style } from '$lib/generated/contracts';
  import type { CvFont } from '$lib/cv';
  import { Button } from '$lib/ui';
  import TemplateGallery from '$lib/tailor/TemplateGallery.svelte';
  import StyleSettings from '$lib/components/cv/StyleSettings.svelte';
  import MarginSettings from '$lib/components/cv/MarginSettings.svelte';

  let status = $state<'loading' | 'ready' | 'error'>('loading');
  let loadError = $state<string | null>(null);
  let fonts = $state<CvFont[]>([]);

  let templateId = $state('');
  let style = $state<Style>({});
  let margins = $state<Margins>({ top: 0.5, right: 0.5, bottom: 0.5, left: 0.5 });

  let saving = $state(false);
  let saveError = $state<string | null>(null);
  let saved = $state(false);
  let savedTimer: ReturnType<typeof setTimeout> | undefined;

  onMount(load);

  async function load() {
    status = 'loading';
    try {
      const [defaults, fontList] = await Promise.all([api.getCvAppearanceDefaults(), api.listCvFonts()]);
      templateId = defaults.template_id;
      style = defaults.style;
      margins = defaults.margins;
      fonts = fontList;
      status = 'ready';
    } catch (e) {
      loadError = e instanceof ApiError ? e.message : 'Could not load your appearance defaults.';
      status = 'error';
    }
  }

  async function save() {
    saving = true;
    saveError = null;
    saved = false;
    try {
      const result = await api.setCvAppearanceDefaults({ template_id: templateId, style, margins });
      templateId = result.template_id;
      style = result.style;
      margins = result.margins;
      saved = true;
      clearTimeout(savedTimer);
      savedTimer = setTimeout(() => (saved = false), 2000);
    } catch (e) {
      saveError = e instanceof ApiError ? e.message : 'Could not save your appearance defaults.';
    } finally {
      saving = false;
    }
  }
</script>

<svelte:head>
  <title>CV appearance defaults — freehire</title>
</svelte:head>

<div class="max-w-3xl space-y-6">
  <div>
    <h1 class="text-2xl font-semibold">CV appearance defaults</h1>
    <p class="text-sm text-muted-foreground">
      The template, typography, and margins a new CV starts with. Changing these only affects CVs
      you create from now on — CVs you already have keep their own appearance.
    </p>
  </div>

  {#if status === 'loading'}
    <p class="text-muted-foreground">Loading…</p>
  {:else if status === 'error'}
    <p class="text-sm text-destructive">{loadError}</p>
  {:else}
    <div class="space-y-6">
      <section class="space-y-2">
        <h2 class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">Template</h2>
        <TemplateGallery bind:value={templateId} />
      </section>
      <section class="space-y-2">
        <h2 class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">Typography</h2>
        <StyleSettings bind:style {fonts} />
      </section>
      <section class="space-y-2">
        <h2 class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          Page margins <span class="font-normal normal-case tracking-normal">(inches)</span>
        </h2>
        <MarginSettings bind:margins />
      </section>

      <div class="flex flex-wrap items-center gap-2">
        <Button variant="primary" disabled={saving} onclick={save}>Save defaults</Button>
      </div>
      {#if saveError}
        <p class="text-sm text-destructive">{saveError}</p>
      {:else if saved}
        <p class="text-xs text-muted-foreground">Saved.</p>
      {/if}
    </div>
  {/if}
</div>
