<script lang="ts">
  import { Bookmark, X } from '@lucide/svelte';
  import SaveSearchAlert from '../filters/SaveSearchAlert.svelte';

  // The ephemeral post-onboarding nudge: after the wizard configures a feed, offer to
  // save it (and then get its new jobs in Telegram). Hosts the shared SaveSearchAlert
  // (quick variant); `autostart` resumes a pending save after sign-in. Purely a
  // container — the save/alert logic lives in SaveSearchAlert. Always actionable: save
  // works even when Telegram is off, so there's no dead state to gate on.
  let {
    query,
    autostart = false,
    onDismiss,
  }: {
    query: string;
    autostart?: boolean;
    onDismiss: () => void;
  } = $props();
</script>

<div class="mb-3 flex items-start gap-3 rounded-xl border border-border bg-card p-3 pl-4">
  <div class="mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-lg bg-brand/10 text-brand-strong">
    <Bookmark class="size-4.5" />
  </div>
  <div class="min-w-0 flex-1">
    <p class="text-sm font-semibold tracking-tight">Save this feed</p>
    <p class="mb-2 text-xs text-muted-foreground">Keep it — then get its new jobs in Telegram, no need to check back.</p>
    <SaveSearchAlert {query} {autostart} variant="quick" />
  </div>
  <button
    type="button"
    onclick={onDismiss}
    aria-label="Dismiss"
    class="flex size-8 shrink-0 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
  >
    <X class="size-4" />
  </button>
</div>
