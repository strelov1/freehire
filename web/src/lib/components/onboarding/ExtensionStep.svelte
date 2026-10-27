<script lang="ts">
  // The tour's browser-extension step. The extension is free to install; filling in
  // application forms is Pro. The claims follow /features/extension.
  import { resolve } from '$app/paths';
  import { ArrowRight, Keyboard, ListChecks, MousePointerClick, ScanText } from '@lucide/svelte';
  import { EXTENSION_FIREFOX_URL, EXTENSION_STORE_URL } from '#lib/extensionLinks.js';
  import { Button, ProviderIcon } from '#lib/ui/index.js';
  import ExtensionPanelPreview from '#lib/components/ExtensionPanelPreview.svelte';
  import FeatureGrid from './FeatureGrid.svelte';
  import ProBar from './ProBar.svelte';
</script>

<h2 class="text-xl font-semibold tracking-tight text-balance">Fill any application from the side panel</h2>
<p class="mt-1 text-sm text-muted-foreground text-pretty">
  Our browser extension sits next to the job you're reading — on any site, not just the ones we list. It's free to
  install; filling in application forms is part of Pro.
</p>

<div class="mt-6" aria-hidden="true" inert>
  <ExtensionPanelPreview />
</div>

<FeatureGrid
  items={[
    { icon: ScanText, title: 'Reads the posting', body: 'Next to the job you are on, on any site.' },
    { icon: ListChecks, title: 'Finds every question', body: 'Dropdowns included.', pro: true },
    { icon: Keyboard, title: 'Fills it from your profile', body: 'One field at a time, in front of you.', pro: true },
    { icon: MousePointerClick, title: 'You press Submit', body: 'It never sends anything itself.' },
  ]}
/>

<div class="mt-6 flex flex-wrap items-center gap-3">
  <Button href={EXTENSION_STORE_URL} target="_blank" variant="outline" size="sm">
    <ProviderIcon provider="chrome" class="size-4" /> Add to Chrome
  </Button>
  <Button href={EXTENSION_FIREFOX_URL} target="_blank" variant="outline" size="sm">
    <ProviderIcon provider="firefox" class="size-4" /> Add to Firefox
  </Button>
  <a
    href={resolve('/features/extension')}
    target="_blank"
    class="ml-auto inline-flex items-center gap-1 text-sm font-medium text-brand-strong hover:underline"
  >
    How the extension works <ArrowRight class="size-3.5" aria-hidden="true" />
  </a>
</div>
<ProBar note="Filling forms is part of Pro. Search, alerts, the AI connector and the extension itself stay free." />
