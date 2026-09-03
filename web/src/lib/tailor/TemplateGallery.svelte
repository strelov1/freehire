<script lang="ts">
  import { api, ApiError } from '$lib/api';
  import type { CvTemplate } from '$lib/cv';

  // The template gallery: a grid of static preview thumbnails (served from
  // /cv-previews/<id>.svg) with the current template highlighted. Two modes:
  //  - cvId given: self-persisting, exactly as the tailoring workspace has always used it —
  //    picking a thumbnail persists it via the set-template endpoint and calls onSelected(id)
  //    so the host can keep its own template id in step (autosave writes it too) and
  //    cache-bust the PDF.
  //  - cvId omitted: controlled — the highlighted thumbnail mirrors the bindable `value`, and
  //    picking one only updates `value`; no API call. This is the appearance-defaults settings
  //    screen's mode, where there is no CV yet to persist onto.
  // A template that prints a headshot says so when none is stored — the render would otherwise
  // silently fall back to the placeholder, which is easy to send without noticing.
  let {
    cvId,
    onSelected,
    value = $bindable(),
  }: { cvId?: string; onSelected?: (id: string) => void; value?: string } = $props();

  let status = $state<'loading' | 'error' | 'ready'>('loading');
  let templates = $state<CvTemplate[]>([]);
  // Whether the member has a headshot. Unknown (null) until the read lands, and false when
  // object storage is off — in both cases the nudge stays hidden rather than guessing.
  let hasPhoto = $state<boolean | null>(null);
  let current = $state('');
  // While a switch is in flight, disable the grid so a double-click can't race two saves.
  let saving = $state(false);
  let error = $state<string | null>(null);

  $effect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const [list, photo] = await Promise.all([
          api.listCvTemplates(),
          api.getPhoto().catch(() => null),
        ]);
        if (cancelled) return;
        templates = list;
        hasPhoto = photo === null ? null : photo.enabled && photo.present;
        if (cvId) {
          const rec = await api.getCv(cvId);
          if (cancelled) return;
          current = rec.template_id;
        }
        status = 'ready';
      } catch (e) {
        if (cancelled) return;
        error = e instanceof ApiError ? e.message : 'Could not load templates.';
        status = 'error';
      }
    })();
    return () => {
      cancelled = true;
    };
  });

  // Controlled mode: the highlighted thumbnail always mirrors the caller's bound value.
  $effect(() => {
    if (!cvId) current = value ?? '';
  });

  async function select(id: string) {
    if (id === current || saving) return;
    if (!cvId) {
      value = id;
      return;
    }
    const previous = current;
    saving = true;
    current = id; // optimistic highlight
    error = null;
    try {
      await api.setCvTemplate(cvId, id);
      onSelected?.(id);
    } catch (e) {
      current = previous; // roll back the highlight on failure
      error = e instanceof ApiError ? e.message : 'Could not switch template.';
    } finally {
      saving = false;
    }
  }
</script>

{#if status === 'loading'}
  <p class="text-sm text-muted-foreground">Loading templates…</p>
{:else if status === 'error'}
  <p class="text-sm text-destructive">{error}</p>
{:else}
  <div class="space-y-3">
    {#if error}
      <p class="text-sm text-destructive" aria-live="polite">{error}</p>
    {/if}
    <div class="grid grid-cols-2 gap-3">
      {#each templates as t (t.id)}
        <button
          type="button"
          onclick={() => select(t.id)}
          disabled={saving}
          aria-pressed={t.id === current}
          class={[
            'group flex flex-col overflow-hidden rounded-lg border text-left transition-colors disabled:opacity-60',
            t.id === current
              ? 'border-primary ring-2 ring-primary/40'
              : 'border-border hover:border-foreground/40',
          ]}
        >
          <img
            src="/cv-previews/{t.id}.svg"
            alt="{t.label} template preview"
            loading="lazy"
            class="aspect-[1/1.414] w-full border-b border-border bg-white object-cover object-top"
          />
          <span class="flex flex-col gap-0.5 px-2.5 py-2">
            <span class="text-sm font-medium text-foreground">{t.label}</span>
            <span class="text-xs text-muted-foreground">{t.style}</span>
            {#if t.photo && hasPhoto === false}
              <span class="mt-0.5 text-[11px] leading-tight text-muted-foreground">
                Add a photo in your profile — this template shows one
              </span>
            {/if}
          </span>
        </button>
      {/each}
    </div>
  </div>
{/if}
