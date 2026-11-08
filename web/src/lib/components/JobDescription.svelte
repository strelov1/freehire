<script lang="ts">
  // A job's server-sanitized description HTML with consistent typographic styles. Reused by the
  // job page (JobView), the tracker/drawer, and the tailor artifact panel — one home for the CSS
  // so the description reads the same everywhere.
  //
  // `lang` marks the body as a foreign-language subtree of an English page (see
  // foreignContentLang in #lib/seo). Optional: callers that don't know the posting's
  // language omit it and the body simply inherits the document's.
  //
  // Headings are shifted so the body's top level is <h3> (see descriptionHeadings.ts): the
  // body sits under the host page's own <h1> and beside its <h2> sections.
  import { demoteHeadings } from '#lib/descriptionHeadings.js';

  let { html, lang }: { html: string; lang?: string } = $props();

  const body = $derived(html ? demoteHeadings(html) : '');
</script>

{#if body}
  <!-- Description is server-sanitized HTML (see internal/sources), safe to render. -->
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- server-sanitized; the rule flags every {@html} regardless -->
  <div class="job-description text-sm leading-relaxed" {lang}>{@html body}</div>
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
  .job-description :global(h4),
  .job-description :global(h5),
  .job-description :global(h6) {
    margin-top: 1.25rem;
    margin-bottom: 0.5rem;
    font-weight: 600;
  }

  /* The sanitizer keeps <pre> and <table>, and neither wraps: a code sample or a wide
     table would otherwise push the whole page sideways on a phone. Each scrolls inside
     the column instead. `display: block` is what lets a table take overflow at all; its
     rows and cells still lay out as a table inside the anonymous table box the browser
     wraps them in. */
  .job-description :global(pre),
  .job-description :global(table) {
    display: block;
    max-width: 100%;
    overflow-x: auto;
  }

  .job-description :global(th),
  .job-description :global(td) {
    padding: 0.25rem 0.75rem 0.25rem 0;
    text-align: left;
    vertical-align: top;
  }

  /* Many sources space their paragraphs with a <br> (or a stray </br>) between the blocks,
     on top of the blocks' own margins, which read as a blank line after every paragraph.
     A break right after a block, and an empty paragraph, carry no text of their own. */
  .job-description :global(:is(p, ul, ol, li, div, table, pre, h3, h4, h5, h6) + br),
  .job-description :global(:is(p, ul, ol, li, div, table, pre, h3, h4, h5, h6) + br + br),
  .job-description :global(:is(ul, ol) > br),
  .job-description :global(p:empty) {
    display: none;
  }

  /* The column's own gap spaces the body from what precedes it; a first block's margin
     added a blank line on top of it. */
  .job-description > :global(:first-child) {
    margin-top: 0;
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

  .job-description :global(b),
  .job-description :global(strong) {
    font-weight: 600;
  }
</style>
