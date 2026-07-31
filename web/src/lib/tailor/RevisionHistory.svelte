<script lang="ts">
  // The history of what changed this CV: every edit, whose hand it was, and a control to undo
  // it on its own. An assistant run folds into one entry that can be undone whole or opened to
  // undo any single edit inside it.
  //
  // Hovering an entry underlines what it touched in the live preview, and clicking pins that
  // highlight — which is how "what did it actually change" gets answered by looking at the
  // page rather than by reading a description of it.
  //
  // An entry's description is the server's own words, generated from the operations. The note
  // is the assistant's, and is rendered as such: attributed, in quotes, never as the entry's
  // description. Text a model wrote must not appear as the application speaking.
  import { ApiError } from '$lib/api';
  import type { RevisionView } from '$lib/generated/contracts';
  import { groupByBatch, actorLabel } from './revisions';

  let {
    revisions,
    pinned = $bindable(null),
    onUndo,
    onUndoRun,
  }: {
    revisions: RevisionView[];
    /** The entry whose edits stay underlined in the preview until another is picked. */
    pinned?: RevisionView | null;
    /** Undo one entry. The page owns this rather than the panel, because the document is
     *  saved on a debounce: undoing without flushing the pending save first lets the timer
     *  write the old text back a second later, and the undo silently reverses itself. */
    onUndo: (revision: RevisionView) => Promise<void>;
    /** Undo every standing edit of one run, under the same ordering rule. */
    onUndoRun: (batchId: string) => Promise<void>;
  } = $props();

  const groups = $derived(groupByBatch(revisions));

  let busy = $state('');
  let error = $state('');

  function pin(revision: RevisionView) {
    pinned = pinned?.id === revision.id ? null : revision;
  }

  async function undoOne(revision: RevisionView) {
    busy = revision.id;
    error = '';
    try {
      await onUndo(revision);
      if (pinned?.id === revision.id) pinned = null;
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not undo that edit.';
    } finally {
      busy = '';
    }
  }

  async function undoRun(batchId: string) {
    busy = batchId;
    error = '';
    try {
      await onUndoRun(batchId);
      pinned = null;
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not undo that run.';
    } finally {
      busy = '';
    }
  }

  const when = (iso: string) =>
    new Date(iso).toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
</script>

{#snippet entry(revision: RevisionView, nested: boolean)}
  <li
    class={['group rounded px-2 py-1.5', nested ? 'ml-3 border-l border-border pl-3' : '']}
    onmouseenter={() => (pinned ??= revision)}
    onmouseleave={() => {
      if (pinned?.id === revision.id && !nested) pinned = null;
    }}
  >
    <div class="flex items-start justify-between gap-2">
      <button type="button" class="min-w-0 flex-1 text-left" onclick={() => pin(revision)}>
        <p class={['text-sm', revision.reverted ? 'text-muted-foreground line-through' : 'text-foreground']}>
          {revision.title}
        </p>
        <p class="text-xs text-muted-foreground">
          {actorLabel(revision.actor)} · {when(revision.created_at)}
        </p>
        {#if revision.note}
          <!-- The assistant's own words, attributed. Never the entry's description. -->
          <p class="mt-1 border-l-2 border-brand-muted pl-2 text-xs italic text-muted-foreground">
            {revision.note}
          </p>
        {/if}
      </button>
      {#if revision.reverted}
        <span class="shrink-0 text-xs text-muted-foreground">undone</span>
      {:else}
        <button
          type="button"
          class="shrink-0 rounded px-1.5 py-0.5 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-50"
          disabled={busy !== ''}
          onclick={() => undoOne(revision)}
        >
          {busy === revision.id ? 'Undoing…' : 'Undo'}
        </button>
      {/if}
    </div>
  </li>
{/snippet}

<div class="p-4">
  {#if error}
    <p class="mb-3 rounded border border-destructive/40 bg-destructive/10 px-2 py-1.5 text-xs text-destructive">
      {error}
    </p>
  {/if}

  {#if groups.length === 0}
    <p class="text-sm text-muted-foreground">
      Nothing has changed this CV yet. Every edit — yours or the assistant's — shows up here, and
      each one can be undone on its own.
    </p>
  {:else}
    <ul class="space-y-1">
      {#each groups as group (group.kind === 'batch' ? group.batchId : group.revision.id)}
        {#if group.kind === 'single'}
          {@render entry(group.revision, false)}
        {:else}
          <li class="rounded border border-border">
            <div class="flex items-center justify-between gap-2 border-b border-border px-2 py-1.5">
              <p class="text-sm font-semibold text-foreground">
                Assistant run · {group.revisions.length}
                {group.revisions.length === 1 ? 'edit' : 'edits'}
              </p>
              {#if group.undoable}
                <button
                  type="button"
                  class="shrink-0 rounded px-1.5 py-0.5 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-50"
                  disabled={busy !== ''}
                  onclick={() => undoRun(group.batchId)}
                >
                  {busy === group.batchId ? 'Undoing…' : 'Undo the run'}
                </button>
              {:else}
                <span class="shrink-0 text-xs text-muted-foreground">undone</span>
              {/if}
            </div>
            <ul class="py-1">
              {#each group.revisions as revision (revision.id)}
                {@render entry(revision, true)}
              {/each}
            </ul>
          </li>
        {/if}
      {/each}
    </ul>
  {/if}
</div>
