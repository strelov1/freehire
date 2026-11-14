<script lang="ts">
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { messages } from './AssistantChat.messages';

  // The questions an empty plain chat offers to start from; picking one sends it as the
  // candidate's own message.
  let {
    starters,
    disabled,
    onPick,
  }: { starters: readonly string[]; disabled: boolean; onPick: (text: string) => void } = $props();

  const s = $derived(t(messages, locale()));
</script>

<ul class="flex flex-wrap gap-2" aria-label={s.startersLabel}>
  {#each starters as starter (starter)}
    <li>
      <button
        type="button"
        class="rounded-lg border border-border bg-background px-3 py-2 text-left text-sm text-foreground transition-colors hover:bg-muted disabled:opacity-50"
        {disabled}
        onclick={() => onPick(starter)}
      >
        {starter}
      </button>
    </li>
  {/each}
</ul>
