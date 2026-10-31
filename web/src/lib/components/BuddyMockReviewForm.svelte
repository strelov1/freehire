<script lang="ts">
  import { api } from '#lib/api.js';
  import { errorMessage } from '#lib/utils.js';
  import { Button } from '#lib/ui/index.js';

  let { id, onreviewed }: { id: string; onreviewed: () => Promise<void> } = $props();

  let rating = $state(0);
  let comment = $state('');
  let sending = $state(false);
  let error = $state('');

  async function send() {
    sending = true;
    error = '';
    try {
      await api.reviewBuddyMock(id, rating, comment.trim());
      await onreviewed();
    } catch (e) {
      error = errorMessage(e, 'Your review could not be saved.');
    } finally {
      sending = false;
    }
  }
</script>

<div class="flex flex-col gap-2 border-t pt-3">
  <p class="text-sm font-medium">How did it go?</p>
  <p class="text-muted-foreground text-xs">
    They see your rating only once they have rated you too, or a week after the mock — so
    neither of you can answer the other's.
  </p>
  <div class="flex gap-1" role="radiogroup" aria-label="Rating">
    {#each [1, 2, 3, 4, 5] as value (value)}
      <button
        type="button"
        role="radio"
        aria-checked={rating === value}
        class="rounded-md border px-2 py-1 text-sm {rating === value ? 'border-primary bg-primary/5' : 'border-input'}"
        onclick={() => (rating = value)}>{value} ★</button
      >
    {/each}
  </div>
  <textarea
    bind:value={comment}
    rows="3"
    maxlength="1000"
    aria-label="Comment"
    class="border-input bg-background w-full rounded-md border px-3 py-2 text-sm"
    placeholder="What went well, and what to practise next (optional)"
  ></textarea>
  <div>
    <Button size="sm" disabled={sending || rating === 0} onclick={send}>
      {sending ? 'Sending…' : 'Send review'}
    </Button>
  </div>
  {#if error}<p class="text-destructive text-sm">{error}</p>{/if}
</div>
