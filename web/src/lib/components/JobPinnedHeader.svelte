<script lang="ts">
  import type { Snippet } from 'svelte';
  import { resolve } from '$app/paths';
  import { companyLogoUrl } from '#lib/logo.js';
  import type { Job } from '#lib/types.js';
  import { EntityLogo } from '#lib/ui/index.js';

  // `lang` is the posting's own language for its title (undefined when English).
  let { job, lang, actions }: { job: Job; lang: string | undefined; actions: Snippet } = $props();

  // The pinned header. Once the posting's own header has scrolled under the top bar,
  // a one-line copy of it — company, title, apply — takes its place, so "who is this
  // for?" and the button stay a glance away through a description that routinely runs
  // several screens.
  //
  // It rides a zero-height sticky rail inside an absolutely-positioned overlay, so the
  // bar occupies no space in the flow and appearing can never shift the text under it.
  // The obvious alternative — pinning the real header and collapsing it — cannot say
  // that: its flow box loses ~90px the instant it pins, jerking the paragraph the
  // reader is mid-sentence in, and again in reverse on the way back up.
  const PINNED_HEADER_TOP = 56; // `top-14` on the rail below, and the top bar's own `h-14`.
  let headerSentinel: HTMLElement | undefined = $state();
  let headerPinned = $state(false);
  $effect(() => {
    const el = headerSentinel;
    if (!el) return;
    const io = new IntersectionObserver(
      (entries) => {
        const entry = entries[0];
        if (!entry) return;
        // `isIntersecting` alone also reads false while the sentinel is BELOW the fold,
        // which is where it sits on a short viewport before anything has been scrolled.
        // The rect settles which edge it left by.
        headerPinned = !entry.isIntersecting && entry.boundingClientRect.top < PINNED_HEADER_TOP;
      },
      { rootMargin: `-${PINNED_HEADER_TOP}px 0px 0px 0px` },
    );
    io.observe(el);
    return () => io.disconnect();
  });
</script>

<!-- The pinned header (see PINNED_HEADER_TOP). The overlay spans JobView's content column, which
     is what gives the rail inside it something to travel: a `sticky` element is
     clamped to its containing block, and the column is exactly the stretch the bar
     should stay pinned over — it releases at the end of the posting, not over the
     related jobs below. The `bottom-14` inset is the bar's own height: the rail is
     zero-height, so without it the bar hangs a further ~52px past the end of the
     overlay and spills over the "See also" strip on the last screen.
     `pointer-events-none` on the overlay keeps the description selectable through
     it; the bar itself takes them back.
     `invisible` rather than a conditional block: keeping the bar mounted lets it
     fade, and visibility:hidden takes it out of the focus order and the a11y tree
     while it is away, so the duplicate apply button is not tabbable from the top
     of the page. -->
<div class="pointer-events-none absolute inset-x-0 top-0 bottom-14">
  <div bind:this={headerSentinel} aria-hidden="true" class="h-px w-full"></div>
  <div class="sticky top-14 z-20 h-0">
    <div
      class={[
        // Opaque, not frosted: the description slides directly under this and a
        // translucent bar leaves a blurred ghost of the text inside it, which reads
        // as a rendering fault rather than as glass. The mobile apply bar below can
        // afford the frost because nothing scrolls under it at speed.
        'pointer-events-auto flex items-center gap-3 border-b border-border bg-background py-2.5 transition-opacity duration-150',
        headerPinned ? 'opacity-100' : 'invisible opacity-0',
      ]}
    >
      <EntityLogo
        name={job.company || 'Unknown company'}
        src={companyLogoUrl(job.company) ?? undefined}
        shape="square"
        size="sm"
      />
      <p class="min-w-0 flex-1 truncate text-sm">
        {#if job.company_slug}
          <a
            href={resolve('/companies/[slug]', { slug: job.company_slug })}
            class="text-muted-foreground hover:text-foreground hover:underline"
          >
            {job.company || 'Unknown company'}
          </a>
        {:else}
          <span class="text-muted-foreground">{job.company || 'Unknown company'}</span>
        {/if}
        <span class="px-1 text-muted-foreground" aria-hidden="true">·</span>
        <span class="font-semibold" {lang}>{job.title}</span>
      </p>
      <!-- `actions`: JobView's save and CTA buttons, the same ones as its header. -->
      <div class="hidden shrink-0 items-center gap-2 lg:flex">
        {@render actions()}
      </div>
    </div>
  </div>
</div>
