<script lang="ts">
  import { page } from '$app/state';
  import Seo from '$lib/components/Seo.svelte';
  import ExtensionLandingView from '$lib/components/ExtensionLandingView.svelte';
  import { EXTENSION_FAQ } from '$lib/extensionFaq';
  import { EXTENSION_STORE_URL } from '$lib/extensionLinks';
  import { breadcrumbJsonLd, extensionApplicationJsonLd, faqPageJsonLd, jsonLdScript } from '$lib/seo';

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/features/extension`);
  // The FAQ block and this payload render from the same EXTENSION_FAQ array, and the
  // install link from the same constant the buttons use, so the structured data
  // cannot disagree with what the page shows.
  const jsonLd = $derived(
    jsonLdScript([
      extensionApplicationJsonLd(origin, EXTENSION_STORE_URL),
      faqPageJsonLd(EXTENSION_FAQ),
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'Browser extension', url: canonical },
      ]),
    ])
  );
</script>

<Seo
  title="freehire for Chrome — a job-application agent in the side panel"
  description="Open the freehire side panel on any job posting: it reads the page itself, scores it against your CV, and fills the application form from your profile. Greenhouse, Lever, Workday, Ashby — or a career page nobody has heard of. You press Submit."
  {canonical}
/>

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <ExtensionLandingView />
</div>
