<script lang="ts">
  // The live centre preview: renders a CV Document as ATS-style HTML paginated into discrete A4
  // sheets (page 1, page 2, …), mirroring how Typst paginates the PDF. It measures each top-level
  // block (header, one per experience/project entry, each short section) in a hidden layer, then
  // greedily packs blocks onto A4 page bodies via paginateBlocks so a section never straddles the
  // inter-page gap. Page geometry — content width, sheet padding, and the page body height used for
  // pagination — is derived from the document's per-side margins, so preview and PDF agree. The look
  // tracks the selected template (classic / centered / modern-sans / sidebar / portrait / headshot).
  // `zoom` scales the whole stack. String composition lives in $lib/cv (unit-tested); this file is
  // layout only — `photoSrc` is handed in rather than fetched here, since the headshot belongs to the
  // profile, not to the document being rendered.
  import type { Document, ExperienceItem, Project } from '$lib/generated/contracts';
  import { experienceHeader, educationLine, languageLabel, certificationLine, type CvFont } from '$lib/cv';
  import HeadshotSilhouette from '$lib/components/HeadshotSilhouette.svelte';
  import { paginateBlocks, previewTypography } from './geometry';
  import { must } from '$lib/utils';
  import { keepIndex, makeHighlighter, makeContainerHighlighter } from './revisions';

  let {
    doc,
    templateId = 'classic-ats',
    zoom = 1,
    fonts = [],
    photoSrc = null,
    highlightPaths = [],
  }: {
    doc: Document;
    templateId?: string;
    zoom?: number;
    fonts?: CvFont[];
    photoSrc?: string | null;
    /** Addresses a revision touched, e.g. `experience[2].bullets[1]`. The nodes they name are
     *  underlined, so "what changed" is answerable by looking at the page. */
    highlightPaths?: string[];
  } = $props();

  // Whether a given address was touched by whatever the history has selected.
  const lit = $derived(makeHighlighter(highlightPaths));
  // A project renders as one line with its bullets inline, so anything changed inside it
  // underlines the line — there is no finer node to put the mark on.
  const litInside = $derived(makeContainerHighlighter(highlightPaths));

  // A4 at 96dpi, and the inch→pixel factor margins convert through.
  const PAGE_W = 794;
  const PAGE_H = 1123;
  const PX_PER_IN = 96;
  const COL_GAP = 24; // matches the sidebar grid's gap-6

  // Per-template presentation flags (default to classic for any unknown id). `portrait` is the
  // sidebar layout with a photo above the narrow column; `headshot` is the sans single column with
  // one in the header — so each reuses the flags of the template it is a variant of.
  const isCentered = $derived(templateId === 'centered');
  const isHeadshot = $derived(templateId === 'headshot');
  const isPortrait = $derived(templateId === 'portrait');
  const isSans = $derived(templateId === 'modern-sans' || isHeadshot);
  const twoColumn = $derived(templateId === 'sidebar' || isPortrait);
  const ruled = $derived(isCentered || isSans || twoColumn); // a rule under each section heading
  const contactSep = $derived(isSans ? '·' : '|');
  // The candidate's name renders at a different scale per template — must track each template's
  // own `text(weight: "bold", size: (N/9.5)*1em)` on `full_name` in templates/<id>.typ. classic-ats
  // prints a compact single-line ATS header; every other template sets a larger display name, and
  // a single shared ratio for all six (as this used to be) overstated classic-ats specifically.
  const nameSizeRatio = $derived(twoColumn ? 17 / 9.5 : isCentered || isSans ? 18 / 9.5 : 12 / 9.5);

  // Page geometry from the document's margins (inches → px). A missing or zero side falls back to
  // 0.5in, matching the backend clampMargin / Typst mg rule so preview and PDF never diverge.
  const mt = $derived((doc.margins?.top || 0.5) * PX_PER_IN);
  const mr = $derived((doc.margins?.right || 0.5) * PX_PER_IN);
  const mb = $derived((doc.margins?.bottom || 0.5) * PX_PER_IN);
  const ml = $derived((doc.margins?.left || 0.5) * PX_PER_IN);
  // The registry id each template falls back to when the candidate has not chosen a font — must
  // match that template's own Typst default face (`fontFamily` in templates/<id>.typ).
  const defaultFontId = $derived(isSans ? 'liberation-sans' : 'libertinus-serif');

  // Typography, resolved once and spread onto BOTH the visible sheets and the hidden
  // measurement layer below. One value, two consumers — measuring in one type and drawing in
  // another would make pagination disagree with what is on screen, and nothing would report it.
  //
  // A candidate's explicit alternate (Tinos/Liberation Sans/Carlito) ships no webfont: its CSS
  // stack falls through to whatever the machine has, which on Windows is the very face the
  // registry entry matches (Times New Roman, Arial, Calibri) and elsewhere may be a generic —
  // an accepted approximation for the minority of CVs that touch the style picker at all.
  //
  // The unset case is different, and gets a real font: `defaultFontId` below resolves to the
  // active template's own Typst default (see AGENTS.md § Fonts), and this component's own style
  // block `@font-face`s exactly those two faces (Libertinus Serif, Liberation Sans) as
  // small Latin-subset WOFF2s. Most CVs never touch the style picker, so approximating THIS case
  // with an OS default (e.g. macOS's "New York" for `font-serif`) was measurably wrong: it wraps
  // text onto more lines than Typst does, and a dense CV's preview overflowed onto a second page
  // a whole section before the real PDF would.
  const typography = $derived(
    previewTypography(
      doc.style ?? {},
      fonts.find((f) => f.id === (doc.style?.font_family || defaultFontId))?.css ?? '',
    ),
  );
  const typeStyle = $derived(
    `font-size: ${typography.fontSizePx}px; line-height: ${typography.lineHeight};` +
      (typography.fontFamily ? ` font-family: ${typography.fontFamily};` : ''),
  );
  // `typography.fontFamily` is empty only in the brief window before `fonts` (GET /cv-fonts) has
  // loaded, since `defaultFontId` above always resolves once the registry is in — this Tailwind
  // generic is a loading-state fallback, not a steady-state one.
  const useTemplateFace = $derived(typography.fontFamily === '');

  const contentWidth = $derived(PAGE_W - ml - mr);
  const pageBodyHeight = $derived(PAGE_H - mt - mb);
  // Width the paginating (main) column renders at — full content width, or the sidebar's wide column.
  const mainWidth = $derived(twoColumn ? Math.round(contentWidth * 0.65 - COL_GAP) : contentWidth);

  const header = $derived(doc.header ?? {});
  const contacts = $derived(
    [header.phone, header.email, header.location, ...(header.links ?? [])]
      .map((c) => (c ?? '').trim())
      .filter((c) => c !== ''),
  );
  // keepIndex, not filter: these carry each entry's position in the STORED document, which is
  // what a revision addresses. Renumbering what survives would send a highlight for
  // experience[2] to whichever entry happened to land third on the page.
  const experience = $derived(
    keepIndex(doc.experience ?? [], (e) => experienceHeader(e) !== '' || (e.bullets ?? []).length > 0),
  );
  const projects = $derived(keepIndex(doc.projects ?? [], (p) => (p.name ?? '').trim() !== ''));
  const education = $derived(keepIndex((doc.education ?? []).map(educationLine), (l) => l !== ''));
  const skills = $derived((doc.skills ?? []).flatMap((g) => g.items ?? []).map((s) => s.trim()).filter((s) => s !== ''));
  const languages = $derived((doc.languages ?? []).map(languageLabel).filter((l) => l !== ''));
  const certifications = $derived((doc.certifications ?? []).map(certificationLine).filter((l) => l !== ''));
  const isLink = (c: string) => /^https?:\/\//i.test(c);

  // A Block is the atomic unit pagination distributes across sheets. Experience/projects split per
  // entry (the section heading rides with the first entry); short sections are one block each.
  type Block =
    | { id: string; kind: 'header' }
    | { id: string; kind: 'exp'; item: ExperienceItem; at: number; heading: boolean }
    | { id: string; kind: 'proj'; item: Project; at: number; heading: boolean }
    | { id: string; kind: 'education' }
    | { id: string; kind: 'list'; title: string; items: string[]; sep: string; path: string };

  const blocks = $derived.by<Block[]>(() => {
    const bl: Block[] = [{ id: 'header', kind: 'header' }];
    experience.forEach(({ index, item }, i) =>
      bl.push({ id: `exp-${index}`, kind: 'exp', item, at: index, heading: i === 0 }),
    );
    projects.forEach(({ index, item }, i) =>
      bl.push({ id: `proj-${index}`, kind: 'proj', item, at: index, heading: i === 0 }),
    );
    if (education.length) bl.push({ id: 'education', kind: 'education' });
    // In the two-column layouts skills/languages/certs live in the narrow column, not the main flow.
    if (!twoColumn) {
      if (skills.length) bl.push({ id: 'skills', kind: 'list', title: 'Skills', items: skills, sep: ', ', path: 'skills' });
      if (languages.length) bl.push({ id: 'languages', kind: 'list', title: 'Languages', items: languages, sep: ', ', path: 'languages' });
      if (certifications.length) bl.push({ id: 'certs', kind: 'list', title: 'Certifications', items: certifications, sep: '; ', path: 'certifications' });
    }
    return bl;
  });

  // Measured pixel height of each block, kept in sync with a ResizeObserver over the hidden layer.
  // The effect writes heights but never reads them, so it can't re-trigger itself.
  let measureRefs = $state<(HTMLElement | null)[]>([]);
  let heights = $state<number[]>([]);
  $effect(() => {
    const els = measureRefs.slice(0, blocks.length);
    if (els.length !== blocks.length || els.some((el) => !el)) return;
    const present = els as HTMLElement[];
    // Each block's flow advance = the gap to the next block's top (so collapsed inter-block margins
    // count), and the last block's own height. offsetHeight alone would drop those margins and let a
    // page pack slightly past its body.
    const measure = () => {
      const tops = present.map((el) => el.offsetTop);
      heights = present.map((el, i) =>
        i + 1 < present.length ? must(tops[i + 1]) - must(tops[i]) : el.offsetHeight,
      );
    };
    const ro = new ResizeObserver(measure);
    present.forEach((el) => ro.observe(el));
    measure();
    return () => ro.disconnect();
  });

  // Before measurement completes, fall back to zero heights (everything on one sheet), then
  // re-paginate once real heights land.
  const measured = $derived(heights.length === blocks.length ? heights : blocks.map(() => 0));
  const pages = $derived(paginateBlocks(measured, pageBodyHeight));
  // Resolve each page's block indices to the blocks themselves for type-safe iteration.
  const pageBlocks = $derived<Block[][]>(pages.map((pg) => pg.map((i) => blocks[i]).filter((b): b is Block => !!b)));
</script>

{#snippet sectionHeading(title: string)}
  <h2 class={['mb-1 mt-1.5 font-bold uppercase tracking-wide', isCentered ? 'text-center' : '']}>{title}</h2>
  {#if ruled}<hr class="mb-2 -mt-0.5 border-neutral-300" />{/if}
{/snippet}

{#snippet contactLine()}
  <p class={['text-neutral-700', isCentered ? 'text-center' : '', isSans ? 'text-neutral-500' : '']}>
    {#each contacts as c, i (i)}
      {#if i > 0}<span class="mx-1.5 text-neutral-400">{contactSep}</span>{/if}
      {#if isLink(c)}
        <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external URL from the user's CV, not an internal route -->
        <a href={c} target="_blank" rel="noopener" class="text-[#2b6cb0] hover:underline">{c}</a>
      {:else}{c}{/if}
    {/each}
  </p>
{/snippet}

<!-- The photo frame: the profile headshot, or the same silhouette the Typst templates draw when
     none is stored. `size` is a Tailwind class so the two placements can differ — both sized to the
     millimetres the .typ templates use (26mm in the header, capped at 42mm in the sidebar) at 96dpi,
     so the preview and the PDF frame the face the same way. -->
{#snippet photoFrame(size: string)}
  {#if photoSrc}
    <img src={photoSrc} alt="" class="shrink-0 rounded-sm border border-neutral-200 object-cover {size}" />
  {:else}
    <HeadshotSilhouette class="shrink-0 rounded-sm {size}" />
  {/if}
{/snippet}

{#snippet headerBlock()}
  <header class={['mb-1', isCentered ? 'text-center' : '', isHeadshot ? 'flex items-start gap-5' : '']}>
    <div class={isHeadshot ? 'min-w-0 flex-1' : ''}>
    <h1
      class={['leading-[1.333] font-bold', isSans ? 'uppercase tracking-wider' : 'tracking-tight']}
      style="font-size: calc({nameSizeRatio} * 1em);"
    >
      {header.full_name || 'Your Name'}
    </h1>
    {#if !twoColumn && contacts.length}
      <div class="mt-1">{@render contactLine()}</div>
    {/if}
    {#if (doc.summary ?? '').trim()}
      <p
        class:cv-lit={lit('summary')}
        class={[
          'mt-1 text-neutral-800',
          isCentered ? 'mx-auto max-w-[62ch] italic' : '',
          twoColumn ? 'italic' : '',
        ]}
      >
        {doc.summary}
      </p>
    {/if}
    </div>
    {#if isHeadshot}{@render photoFrame('size-[98px]')}{/if}
  </header>
  <hr class={['my-0.5', isSans || twoColumn ? 'border-neutral-500' : 'border-neutral-300']} />
{/snippet}

{#snippet experienceItem(e: ExperienceItem, at: number)}
  {@const bullets = keepIndex(e.bullets ?? [], (b) => b.trim() !== '')}
  {@const stack = (e.stack ?? []).filter((s) => s.trim())}
  <div class="mb-1">
    <p class="font-bold" class:cv-lit={lit(`experience[${at}].role`) || lit(`experience[${at}].company`)}>
      {experienceHeader(e)}
    </p>
    {#if (e.summary ?? '').trim()}
      <p class="text-neutral-800" class:cv-lit={lit(`experience[${at}].summary`)}>{e.summary}</p>
    {/if}
    {#if bullets.length}
      <!-- No inter-item gap: Typst's list() advances consecutive bullets at the same rate as
           wrapped lines within one bullet — a PDF text layer shows both at 11.0pt, no extra
           list.spacing on top. A Tailwind space-y here would look nicer but drift pagination
           again on a bullet-heavy CV. -->
      <ul class="ml-4 list-disc">
        {#each bullets as b (b.index)}
          <li class:cv-lit={lit(`experience[${at}].bullets[${b.index}]`)}>{b.item}</li>
        {/each}
      </ul>
    {/if}
    {#if stack.length}
      <p class="mt-0.5" class:cv-lit={lit(`experience[${at}].stack`)}>
        <span class="font-bold">Stack:</span> {stack.join(', ')}
      </p>
    {/if}
  </div>
{/snippet}

{#snippet projectItem(p: Project, at: number)}
  {@const bullets = (p.bullets ?? []).filter((b) => b.trim())}
  <ul class="ml-4 list-disc">
    <li class="mb-0.5" class:cv-lit={lit(`projects[${at}]`) || lit(`projects[${at}].bullets`) || litInside(`projects[${at}]`)}>
      <span class="font-bold">{p.name}</span>{#if bullets.length}: {bullets.join(' ')}{/if}
      {#if (p.link ?? '').trim()}
        <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external URL from the user's CV, not an internal route -->
        (<a href={p.link} target="_blank" rel="noopener" class="text-[#2b6cb0] hover:underline">{p.link}</a>)
      {/if}
    </li>
  </ul>
{/snippet}

<!-- These sections are rendered as one composed line each (skills flattened across groups,
     education/languages/certifications through their own formatters), so their addresses stop
     at the section: a revision inside one underlines the section rather than the item. Going
     finer would mean rendering them per-entry, which is a change to the layout, not to the
     highlighting. -->
{#snippet listBlock(title: string, items: string[], sep: string, at: string)}
  <section class="mb-3">
    {@render sectionHeading(title)}
    <p class:cv-lit={lit(at)}>{items.join(sep)}</p>
  </section>
{/snippet}

{#snippet blockView(b: Block)}
  {#if b.kind === 'header'}
    {@render headerBlock()}
  {:else if b.kind === 'exp'}
    {#if b.heading}{@render sectionHeading('Experience')}{/if}
    {@render experienceItem(b.item, b.at)}
  {:else if b.kind === 'proj'}
    {#if b.heading}{@render sectionHeading('Projects')}{/if}
    {@render projectItem(b.item, b.at)}
  {:else if b.kind === 'education'}
    <section class="mb-3">
      {@render sectionHeading('Education')}
      {#each education as line (line.index)}
        <p class="mb-0.5" class:cv-lit={lit(`education[${line.index}]`)}>{line.item}</p>
      {/each}
    </section>
  {:else if b.kind === 'list'}
    {@render listBlock(b.title, b.items, b.sep, b.path)}
  {/if}
{/snippet}

{#snippet sidebarColumn()}
  {#if isPortrait}
    <div class="mb-3">{@render photoFrame('aspect-square w-full max-w-[159px]')}</div>
  {/if}
  {#if contacts.length}
    <section class="mb-3">
      {@render sectionHeading('Contact')}
      {#each contacts as c, i (i)}
        {#if isLink(c)}
          <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external URL from the user's CV, not an internal route -->
          <p class="mb-0.5 break-words"><a href={c} target="_blank" rel="noopener" class="text-[#2b6cb0] hover:underline">{c}</a></p>
        {:else}<p class="mb-0.5 break-words">{c}</p>{/if}
      {/each}
    </section>
  {/if}
  {#if skills.length}{@render listBlock('Skills', skills, ', ', 'skills')}{/if}
  {#if languages.length}{@render listBlock('Languages', languages, ', ', 'languages')}{/if}
  {#if certifications.length}{@render listBlock('Certifications', certifications, '; ', 'certifications')}{/if}
{/snippet}

<!-- Hidden measurement layer: renders every block once at the main column width so the effect can
     read each block's height. Off-screen and aria-hidden so it never affects layout or a11y. Kept
     OUTSIDE the zoomed stack below — otherwise CSS zoom would scale the measured heights. -->
<div
  aria-hidden="true"
  class={[
    'pointer-events-none invisible absolute -left-[9999px] top-0 text-neutral-900',
    useTemplateFace && (isSans ? 'font-sans' : 'font-serif'),
  ]}
  style="width: {mainWidth}px; {typeStyle}"
>
  {#each blocks as b, i (b.id)}
    <div bind:this={measureRefs[i]}>{@render blockView(b)}</div>
  {/each}
</div>

<!-- Visible A4 sheets, stacked. The CSS `zoom` property scales the whole stack — layout included —
     so the scroll container reserves the scaled size (transform: scale would keep the 794px box and
     clip the left edge on first paint). `items-center` centres each sheet when it fits and lets it
     overflow-scroll when zoomed past the column width. -->
<div class="flex flex-col items-center gap-6" style="zoom: {zoom};">
  {#each pageBlocks as page, p (p)}
    <article
      class={['bg-white text-neutral-900 shadow-sm', useTemplateFace && (isSans ? 'font-sans' : 'font-serif')]}
      style="width: {PAGE_W}px; min-height: {PAGE_H}px; padding: {mt}px {mr}px {mb}px {ml}px; {typeStyle}"
    >
      {#if twoColumn}
        <div class="grid grid-cols-[35%_1fr] gap-6">
          <div>{#if p === 0}{@render sidebarColumn()}{/if}</div>
          <div>
            {#each page as b (b.id)}{@render blockView(b)}{/each}
          </div>
        </div>
      {:else}
        {#each page as b (b.id)}{@render blockView(b)}{/each}
      {/if}
    </article>
  {/each}
</div>

<style>
  /* The exact faces the PDF renders with (Libertinus Serif is Typst's own default; Liberation
     Sans is templates/*.typ's declared default for the sans templates — see fonts.go), subset to
     Latin and WOFF2'd down to ~11-16KB each: small enough that shipping them beats approximating
     a browser default. Scoped to this component's own style block, so Vite code-splits them onto
     whatever route renders a CV preview rather than the whole app's initial bundle. Loading them
     under these exact family names is the whole fix — every `previewTypography` CSS stack
     already lists the family first, so no other code needs to change to pick them up once the
     font is actually resolvable. Regular and bold are separate files: Typst never synthesizes
     bold from regular (it has no such notion), and neither should the browser, or a bold run's
     glyph widths would stop matching what the PDF measures. */
  @font-face {
    font-family: 'Libertinus Serif';
    src: url('/fonts/LibertinusSerif-Regular.woff2') format('woff2');
    font-weight: 400;
    font-display: swap;
  }
  @font-face {
    font-family: 'Libertinus Serif';
    src: url('/fonts/LibertinusSerif-Bold.woff2') format('woff2');
    font-weight: 700;
    font-display: swap;
  }
  @font-face {
    font-family: 'Liberation Sans';
    src: url('/fonts/LiberationSans-Regular.woff2') format('woff2');
    font-weight: 400;
    font-display: swap;
  }
  @font-face {
    font-family: 'Liberation Sans';
    src: url('/fonts/LiberationSans-Bold.woff2') format('woff2');
    font-weight: 700;
    font-display: swap;
  }

  /* What a revision touched is marked with an underline rather than a filled background: the
     page has to keep reading as a CV while it answers "what changed". Wavy and offset so it
     is not mistaken for a link, and it prints away with the highlight since nothing here
     reaches the PDF — this is preview chrome.

     The colour is the brand token rather than a literal: the sheet itself is deliberately
     untokenised (it imitates the rendered PDF, which knows nothing about the app's theme),
     but the highlight is the APP speaking over that sheet, so it belongs to the app's
     palette. */
  .cv-lit {
    text-decoration-line: underline;
    text-decoration-style: dotted;
    text-decoration-thickness: 2px;
    text-underline-offset: 3px;
    text-decoration-color: var(--color-brand-strong);
  }
  .cv-lit :global(li) {
    text-decoration: inherit;
  }
</style>
