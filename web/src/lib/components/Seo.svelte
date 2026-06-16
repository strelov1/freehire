<script lang="ts">
  // Per-page document metadata: title, description, canonical, and Open
  // Graph / Twitter Card tags. One <Seo> per route; rendered server-side so
  // crawlers and link-preview bots get it in the initial HTML.
  let {
    title,
    description,
    canonical,
    ogType = 'website',
    image,
  }: {
    title: string;
    description?: string;
    canonical?: string;
    ogType?: string;
    // Absolute URL of a preview image. When set, the page emits an og:image and
    // upgrades the Twitter card to summary_large_image; when absent it stays a
    // plain summary card with no image.
    image?: string;
  } = $props();
</script>

<svelte:head>
  <title>{title}</title>
  {#if description}
    <meta name="description" content={description} />
  {/if}
  {#if canonical}
    <link rel="canonical" href={canonical} />
    <meta property="og:url" content={canonical} />
  {/if}
  <meta property="og:title" content={title} />
  {#if description}
    <meta property="og:description" content={description} />
  {/if}
  <meta property="og:type" content={ogType} />
  <meta property="og:site_name" content="freehire" />
  {#if image}
    <meta property="og:image" content={image} />
    <meta property="og:image:type" content="image/png" />
    <meta property="og:image:width" content="1200" />
    <meta property="og:image:height" content="630" />
    <meta property="og:image:alt" content={title} />
  {/if}
  <meta name="twitter:card" content={image ? 'summary_large_image' : 'summary'} />
  <meta name="twitter:title" content={title} />
  {#if description}
    <meta name="twitter:description" content={description} />
  {/if}
  {#if image}
    <meta name="twitter:image" content={image} />
  {/if}
</svelte:head>
