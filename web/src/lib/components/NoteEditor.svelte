<script lang="ts">
  import { onMount } from 'svelte';
  import 'easymde/dist/easymde.min.css';
  import type EasyMDE from 'easymde';

  // A small markdown editor for notes: EasyMDE (the maintained SimpleMDE fork) over a
  // textarea, markdown in and out. EasyMDE touches `window`, so it is dynamically
  // imported on mount (never on the server) and torn down on unmount. `onsave` fires
  // on blur with the current markdown; the parent persists it. The component is
  // re-mounted per job by JobDrawer's {#key}, so the initial value never needs to be
  // pushed reactively.
  let { value = '', onsave }: { value?: string; onsave: (v: string) => void } = $props();

  let el = $state<HTMLTextAreaElement>();

  onMount(() => {
    let editor: EasyMDE | undefined;
    let cancelled = false;
    // Persist on blur AND on teardown — closing via Escape (or any unmount) never
    // blurs the editor, so a blur-only save would drop the last edit. Dedup against
    // the last persisted value so an untouched open/close doesn't fire a redundant save.
    let lastSaved = value;
    const persist = () => {
      if (!editor) return;
      const current = editor.value();
      if (current === lastSaved) return;
      lastSaved = current;
      onsave(current);
    };

    void (async () => {
      const { default: EasyMDECtor } = await import('easymde');
      if (cancelled || !el) return;
      editor = new EasyMDECtor({
        element: el,
        initialValue: value,
        placeholder: 'Notes…',
        spellChecker: false,
        status: false,
        minHeight: '120px',
        toolbar: ['bold', 'italic', 'heading', '|', 'unordered-list', 'ordered-list', '|', 'link', 'preview'],
      });
      editor.codemirror.on('blur', persist);
    })();

    return () => {
      cancelled = true;
      persist();
      // toTextArea() reverts the CodeMirror DOM and detaches listeners.
      editor?.toTextArea();
      editor = undefined;
    };
  });
</script>

<textarea bind:this={el}></textarea>

<style>
  /* Theme EasyMDE to the app tokens (raw CSS vars adapt to light/dark automatically). */
  :global(.EasyMDEContainer .CodeMirror) {
    background: var(--card);
    color: var(--foreground);
    border-color: var(--input);
    border-bottom-left-radius: 0.5rem;
    border-bottom-right-radius: 0.5rem;
    font-size: 0.875rem;
  }
  :global(.EasyMDEContainer .CodeMirror-cursor) {
    border-color: var(--foreground);
  }
  :global(.editor-toolbar) {
    border-color: var(--input);
    border-top-left-radius: 0.5rem;
    border-top-right-radius: 0.5rem;
    opacity: 1;
  }
  :global(.editor-toolbar button) {
    color: var(--muted-foreground) !important;
    font-weight: 600;
    border-color: transparent;
  }
  :global(.editor-toolbar button:hover),
  :global(.editor-toolbar button.active) {
    background: var(--accent);
    border-color: var(--border);
  }
  :global(.editor-toolbar i.separator) {
    border-color: var(--border);
  }
  :global(.editor-preview) {
    background: var(--muted);
    color: var(--foreground);
  }
  :global(.CodeMirror-selected) {
    background: var(--accent) !important;
  }
</style>
