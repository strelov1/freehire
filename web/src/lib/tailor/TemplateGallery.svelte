<script lang="ts">
  import { api, ApiError } from '$lib/api';
  import type { CvTemplate } from '$lib/cv';

  // The template gallery for one CV: a grid of static preview thumbnails (served from
  // /cv-previews/<id>.svg) with the current template highlighted. Picking one persists it via
  // the set-template endpoint and calls onSelected(id) so the host can keep its own template id in
  // step (autosave writes it too) and cache-bust the PDF. Non-ATS-safe templates carry an inline
  // caution.
  let { cvId, onSelected }: { cvId: number; onSelected: (id: string) => void } = $props();

  let status = $state<'loading' | 'error' | 'ready'>('loading');
  let templates = $state<CvTemplate[]>([]);
  let current = $state('');
  // While a switch is in flight, disable the grid so a double-click can't race two saves.
  let saving = $state(false);
  let error = $state<string | null>(null);

  $effect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const [list, rec] = await Promise.all([api.listCvTemplates(), api.getCv(cvId)]);
        if (cancelled) return;
        templates = list;
        current = rec.template_id;
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

  async function select(id: string) {
    if (id === current || saving) return;
    const previous = current;
    saving = true;
    current = id; // optimistic highlight
    error = null;
    try {
      await api.setCvTemplate(cvId, id);
      onSelected(id);
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
            {#if !t.ats_safe}
              <span class="mt-0.5 text-[11px] leading-tight text-amber-600 dark:text-amber-500">
                May not parse cleanly in some ATS
              </span>
            {/if}
          </span>
        </button>
      {/each}
    </div>
  </div>
{/if}
