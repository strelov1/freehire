<script lang="ts">
  // Link tracing for one CV: the consent switch and, once there is anything to show, what each
  // traced link has been doing.
  //
  // Unlike the two blocks above it in this pane, this does NOT write into the shared document —
  // it is not a presentation choice that autosave carries along. Consent is its own decision with
  // its own endpoint, so the switch writes immediately and reports its own failure.
  import { api } from '$lib/api';
  import type { CvTracerLink } from '$lib/cv';
  import { SettingRow } from '$lib/ui';

  let { cvId, enabled = $bindable() }: { cvId: string; enabled: boolean } = $props();

  let links = $state<CvTracerLink[]>([]);
  let includeBots = $state(false);
  let error = $state('');
  let saving = $state(false);

  // Counts are only worth fetching for a CV that has been traced. A CV that never was has no
  // links to report, and asking would answer with an empty list every time the pane opens.
  $effect(() => {
    if (!enabled) {
      links = [];
      return;
    }
    void api
      .listCvTracerLinks(cvId)
      .then((rows) => (links = rows))
      .catch(() => (links = []));
  });

  async function toggle(next: boolean) {
    saving = true;
    error = '';
    try {
      await api.setCvTracerLinks(cvId, next);
      enabled = next;
    } catch (e) {
      // The one refusal worth explaining: enabling needs a configured visitor salt, and without
      // one the count would be a guess dressed as a fact.
      error =
        e instanceof Error && e.message.includes('not configured')
          ? 'Link tracking is not available on this deployment.'
          : 'Could not change link tracking.';
    } finally {
      saving = false;
    }
  }

  function shown(link: CvTracerLink): number {
    return includeBots ? link.clicks + link.bot_clicks : link.clicks;
  }

  function whenLast(link: CvTracerLink): string {
    if (!link.last_click_at) return 'never opened';
    const days = Math.floor((Date.now() - Date.parse(link.last_click_at)) / 86_400_000);
    if (days <= 0) return 'opened today';
    if (days === 1) return 'opened yesterday';
    return `opened ${days}d ago`;
  }
</script>

<div class="space-y-3">
  <SettingRow
    label="Track link opens"
    hint="Rewrites this CV's links through freehire so you can see if they were opened."
  >
    {#snippet control()}
      <input
        type="checkbox"
        class="size-4 accent-primary"
        checked={enabled}
        disabled={saving}
        aria-label="Track link opens"
        onchange={(e) => toggle(e.currentTarget.checked)}
      />
    {/snippet}
  </SettingRow>

  {#if error}
    <p class="text-xs text-destructive">{error}</p>
  {/if}

  {#if enabled}
    <p class="text-xs text-muted-foreground">
      Applies to PDFs you download from now on. Links in a PDF you already sent keep working.
    </p>

    {#if links.length > 0}
      <ul class="space-y-2">
        {#each links as link (link.traced_url)}
          <li class="rounded-md border p-2 text-xs">
            <p class="truncate font-medium text-foreground">{link.destination_url}</p>
            <p class="text-muted-foreground">
              {shown(link)}
              {shown(link) === 1 ? 'open' : 'opens'}
              {#if link.distinct_visitors !== undefined}
                · {link.distinct_visitors}
                {link.distinct_visitors === 1 ? 'person' : 'people'}
              {/if}
              · {whenLast(link)}
            </p>
          </li>
        {/each}
      </ul>

      <label class="flex items-center gap-2 text-xs text-muted-foreground">
        <input type="checkbox" class="size-3.5 accent-primary" bind:checked={includeBots} />
        Include likely bots
      </label>

      <!-- Said plainly, because the number invites the opposite reading. Mail security scanners
           follow links automatically with ordinary browser identities, so an open is evidence the
           link was fetched — not proof a person read the CV. -->
      <p class="text-xs text-muted-foreground">
        An open means the link was fetched. Company mail scanners do this automatically, so this is
        a hint, not proof someone read your CV.
      </p>
    {:else}
      <p class="text-xs text-muted-foreground">No opens recorded yet.</p>
    {/if}
  {/if}
</div>
