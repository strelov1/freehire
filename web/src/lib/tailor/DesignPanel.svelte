<script lang="ts">
  import MarginSettings from '#lib/components/cv/MarginSettings.svelte';
  import StyleSettings from '#lib/components/cv/StyleSettings.svelte';
  import TracerLinksSettings from '#lib/components/cv/TracerLinksSettings.svelte';
  import type { CvFont } from '#lib/cv.js';
  import type { Document } from '#lib/generated/contracts.js';
  import TemplateGallery from './TemplateGallery.svelte';

  // The workspace's Design tab: everything that decides how the CV looks, in one place —
  // the template first, since it sets the defaults every control below adjusts. All of it
  // writes straight into the shared document, so the centre preview re-renders live and
  // autosave persists it on the same debounce as any other edit.
  let {
    cvId,
    doc = $bindable(),
    fonts,
    tracerLinksEnabled = $bindable(),
    onTemplateSelected,
  }: {
    cvId: string;
    doc: Document;
    fonts: CvFont[];
    tracerLinksEnabled: boolean;
    onTemplateSelected: (id: string) => void;
  } = $props();
</script>

<div class="space-y-8">
  <section class="space-y-3">
    <h2 class="text-sm font-semibold text-foreground">Template</h2>
    <TemplateGallery {cvId} onSelected={onTemplateSelected} />
  </section>
  <section class="space-y-3">
    <h2 class="text-sm font-semibold text-foreground">Typography</h2>
    <StyleSettings bind:style={doc.style} {fonts} />
  </section>
  <section class="space-y-3">
    <h2 class="text-sm font-semibold text-foreground">
      Page margins <span class="font-normal text-muted-foreground">(inches)</span>
    </h2>
    <MarginSettings bind:margins={doc.margins} />
  </section>
  <!-- Not a presentation choice, and deliberately last: consent to track whoever opens
       this CV is a different kind of decision from a font size, and it writes on its
       own rather than riding the document's autosave. -->
  <section class="space-y-3 border-t border-border pt-6">
    <h2 class="text-sm font-semibold text-foreground">Link tracking</h2>
    <TracerLinksSettings {cvId} bind:enabled={tracerLinksEnabled} />
  </section>
</div>
