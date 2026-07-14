<script module lang="ts">
  // Self-hosted toolbar icons (Lucide, MIT) as inline SVG. EasyMDE otherwise pulls
  // FontAwesome from a third-party CDN for its toolbar glyphs — the only external
  // asset in an otherwise self-hosted frontend, and blank buttons offline or under a
  // future font-src CSP (issue #632). `stroke="currentColor"` lets each glyph inherit
  // the toolbar button's themed color, so light/dark needs no extra rule.
  const ICON_CLASS = 'ffh-toolbar-icon';
  const svgIcon = (paths: string) =>
    `<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">${paths}</svg>`;
  const icons = {
    bold: svgIcon('<path d="M6 12h9a4 4 0 0 1 0 8H7a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1h7a4 4 0 0 1 0 8"/>'),
    italic: svgIcon('<line x1="19" x2="10" y1="4" y2="4"/><line x1="14" x2="5" y1="20" y2="20"/><line x1="15" x2="9" y1="4" y2="20"/>'),
    heading: svgIcon('<path d="M6 12h12"/><path d="M6 20V4"/><path d="M18 20V4"/>'),
    unorderedList: svgIcon('<path d="M3 5h.01"/><path d="M3 12h.01"/><path d="M3 19h.01"/><path d="M8 5h13"/><path d="M8 12h13"/><path d="M8 19h13"/>'),
    orderedList: svgIcon('<path d="M11 5h10"/><path d="M11 12h10"/><path d="M11 19h10"/><path d="M4 4h1v5"/><path d="M4 9h2"/><path d="M6.5 20H3.4c0-1 2.6-1.925 2.6-3.5a1.5 1.5 0 0 0-2.6-1.02"/>'),
    link: svgIcon('<path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/>'),
    preview: svgIcon('<path d="M2.062 12.348a1 1 0 0 1 0-.696 10.75 10.75 0 0 1 19.876 0 1 1 0 0 1 0 .696 10.75 10.75 0 0 1-19.876 0"/><circle cx="12" cy="12" r="3"/>'),
  };
</script>

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
        // Never fetch FontAwesome from the CDN; the buttons below carry their own icons.
        autoDownloadFontAwesome: false,
        // Built-in actions keep their default keyboard shortcuts (Cmd-B/I, bound to the
        // CodeMirror keymap by action name, independent of the toolbar array).
        toolbar: [
          { name: 'bold', action: EasyMDECtor.toggleBold, className: ICON_CLASS, title: 'Bold', icon: icons.bold },
          { name: 'italic', action: EasyMDECtor.toggleItalic, className: ICON_CLASS, title: 'Italic', icon: icons.italic },
          { name: 'heading', action: EasyMDECtor.toggleHeadingSmaller, className: ICON_CLASS, title: 'Heading', icon: icons.heading },
          '|',
          { name: 'unordered-list', action: EasyMDECtor.toggleUnorderedList, className: ICON_CLASS, title: 'Bulleted list', icon: icons.unorderedList },
          { name: 'ordered-list', action: EasyMDECtor.toggleOrderedList, className: ICON_CLASS, title: 'Numbered list', icon: icons.orderedList },
          '|',
          { name: 'link', action: EasyMDECtor.drawLink, className: ICON_CLASS, title: 'Create link', icon: icons.link },
          { name: 'preview', action: EasyMDECtor.togglePreview, className: ICON_CLASS, title: 'Toggle preview', icon: icons.preview, noDisable: true },
        ],
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
  /* Center the inline SVG glyphs (see the icon table in the module script). */
  :global(.editor-toolbar button.ffh-toolbar-icon) {
    display: inline-flex;
    align-items: center;
    justify-content: center;
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
