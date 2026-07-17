<script lang="ts">
  // A job's server-sanitized description HTML with consistent typographic styles. Reused by the
  // job page (JobView), the tracker/drawer, and the tailor artifact panel — one home for the CSS
  // so the description reads the same everywhere.
  let { html }: { html: string } = $props();
</script>

{#if html}
  <!-- Description is server-sanitized HTML (see internal/sources), safe to render. -->
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- server-sanitized; the rule flags every {@html} regardless -->
  <div class="job-description text-sm leading-relaxed">{@html html}</div>
{/if}

<style>
  /* Descriptions are arbitrary scraped HTML: a long URL — or words glued by
     non-breaking spaces — must wrap instead of forcing a horizontal page scroll. */
  .job-description {
    overflow-wrap: break-word;
  }

  .job-description :global(h1),
  .job-description :global(h2),
  .job-description :global(h3),
  .job-description :global(h4) {
    margin-top: 1.25rem;
    margin-bottom: 0.5rem;
    font-weight: 600;
  }

  .job-description :global(p) {
    margin: 0.5rem 0;
  }

  .job-description :global(ul),
  .job-description :global(ol) {
    margin: 0.5rem 0;
    padding-left: 1.25rem;
  }

  .job-description :global(li) {
    display: list-item;
    list-style: disc outside;
    margin: 0.25rem 0;
  }

  /* ATS boards (e.g. Greenhouse) wrap each <li> in a block <p>; collapse its
     margins so the bullet sits beside the text instead of on its own line. */
  .job-description :global(li) > :global(p) {
    margin: 0;
  }

  .job-description :global(a) {
    text-decoration: underline;
  }

  .job-description :global(b),
  .job-description :global(strong) {
    font-weight: 600;
  }
</style>
