<script lang="ts">
  // A free-text token (chip) input for open-vocabulary facets like skills and
  // countries. Enter adds the draft; Backspace on an empty field removes the last
  // chip; the × removes a specific one. Stateless except for the in-progress draft.
  let {
    tokens,
    onAdd,
    onRemove,
    placeholder,
    label,
  }: {
    tokens: string[];
    onAdd: (value: string) => void;
    onRemove: (value: string) => void;
    placeholder?: string;
    // The field's accessible name. A placeholder is not one, so it is only the
    // fallback — pass the visible field label (e.g. "Skills") where there is one.
    label?: string;
  } = $props();

  let draft = $state('');

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && draft.trim()) {
      e.preventDefault();
      onAdd(draft);
      draft = '';
    } else if (e.key === 'Backspace' && draft === '') {
      const last = tokens.at(-1);
      if (last !== undefined) onRemove(last);
    }
  }
</script>

<div
  class="flex flex-wrap items-center gap-1.5 rounded-lg border border-input bg-transparent px-2 py-1.5 transition-colors focus-within:border-ring focus-within:ring-2 focus-within:ring-ring/50 dark:bg-input/30"
>
  {#each tokens as token (token)}
    <span class="inline-flex items-center gap-1 rounded-full bg-secondary px-2 py-0.5 text-xs font-medium text-secondary-foreground">
      {token}
      <button type="button" class="text-muted-foreground transition-colors hover:text-foreground" onclick={() => onRemove(token)} aria-label={`Remove ${token}`}>
        ×
      </button>
    </span>
  {/each}
  <input
    bind:value={draft}
    onkeydown={onKeydown}
    {placeholder}
    aria-label={label ?? placeholder ?? 'Add a value'}
    class="min-w-[6rem] flex-1 bg-transparent outline-none placeholder:text-muted-foreground"
  />
</div>
