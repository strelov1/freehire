<script lang="ts">
  /**
   * Frames a rendered email from `static/email-previews/`.
   *
   * The emails are not Svelte components and never will be: they are produced by
   * Go templates in `internal/mailtpl`, because that is what actually runs when a
   * mail is sent. Rebuilding them here would create a second design that drifts
   * from the one recipients receive. This component only frames the real output.
   *
   * Regenerate the files with `go run ./cmd/mail-preview` from the repo root.
   */
  interface Props {
    /** File stem under static/email-previews/, e.g. "verify-email". */
    name: string;
    /** Frame height in px. Emails vary a lot; the stories set this per mail. */
    height?: number;
    /**
     * Which copy to frame.
     *
     * `auto` is the document that actually gets sent — it follows your OS colour
     * setting, so it shows one design or the other depending on where you sit.
     * `light` and `dark` are pinned copies, which is the only way to compare the
     * two: a page cannot tell a framed document to ignore the system preference,
     * so the toggle swaps files instead of restyling anything.
     */
    scheme?: 'light' | 'dark' | 'auto';
  }

  const { name, height = 620, scheme = 'light' }: Props = $props();

  const suffix = $derived(
    scheme === 'auto' ? '.html' : scheme === 'dark' ? '.dark.html' : '.light.html',
  );
</script>

<!--
  sandbox without allow-scripts: the previews are our own output, but they are
  static documents with nothing to run, so the frame grants nothing.
-->
<iframe
  src="/email-previews/{name}{suffix}"
  title="Email preview: {name}"
  style="width:100%;max-width:680px;height:{height}px;border:1px solid #e4e4e4;border-radius:8px;background:#f0f0f0;"
  sandbox="allow-same-origin"
></iframe>
