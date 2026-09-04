<script lang="ts">
  // The dark brand panel shown beside every credential form (register, sign-in,
  // password recovery — see /signin/+page.svelte). A small text+illustration
  // carousel rather than one static pitch, so a visitor sitting on the form for a
  // while sees more than the one line "every job in one place" — dot indicators
  // sit on the illustration itself (not the bullets) since that's the part that
  // visually reads as a "slide".
  import { onMount } from 'svelte';
  import { resolve } from '$app/paths';
  import BrandMark from '$lib/components/BrandMark.svelte';
  import GithubStars from '$lib/components/GithubStars.svelte';
  import { SOCIAL_LINKS } from '$lib/socialLinks';
  import { ProviderIcon } from '$lib/ui';

  // GithubStars already covers GitHub (icon + live star count); the rest render as
  // plain icon links, same as the Footer's row.
  const otherSocials = SOCIAL_LINKS.filter((s) => s.provider !== 'github');

  type Slide = { headline: string; bullets: string[] };
  // A fixed-length tuple (not just an array) so `SLIDES[0]` below type-checks as
  // `Slide`, not `Slide | undefined` — that's what lets the fallback in `slide`
  // resolve without a non-null assertion.
  const SLIDES: readonly [Slide, Slide] = [
    {
      headline: 'Every IT job in one place — deduplicated, enriched, and searchable in seconds.',
      bullets: [
        'Advanced search across every source',
        "CV tailoring for the role you're applying to",
        'Application tracking, end to end',
      ],
    },
    {
      headline: 'Track every application on your board.',
      bullets: [
        'Save a search, get notified the moment a match appears',
        'CV tailored to the role before you apply',
        'One board for every stage, from applied to offer',
      ],
    },
  ];

  const AUTOPLAY_MS = 6000;
  let active = $state(0);
  const slide = $derived(SLIDES[active] ?? SLIDES[0]);
  let timer: ReturnType<typeof setInterval> | undefined;

  function stop() {
    clearInterval(timer);
    timer = undefined;
  }

  function play() {
    stop();
    if (typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches) return;
    timer = setInterval(() => {
      active = (active + 1) % SLIDES.length;
    }, AUTOPLAY_MS);
  }

  // A manual pick restarts the clock rather than leaving it running on the old
  // schedule, so the slide someone just chose gets the full AUTOPLAY_MS on screen.
  function select(i: number) {
    active = i;
    play();
  }

  onMount(() => {
    play();
    return stop;
  });
</script>

<!-- Hidden on narrow viewports (the form is what matters there). Hover/focus pause
     the autoplay — the dots stay independently operable for keyboard/AT users, but
     a moving illustration behind a form someone is reading should stop under a
     pointer or focus too. -->
<div
  class="relative hidden w-5/12 shrink-0 flex-col overflow-hidden bg-foreground p-10 text-background lg:flex"
  role="group"
  aria-label="freehire"
  onmouseenter={stop}
  onmouseleave={play}
  onfocusin={stop}
  onfocusout={play}
>
  <a href={resolve('/')} class="flex items-center gap-2 text-sm font-semibold tracking-tight">
    <BrandMark />
    freehire
  </a>

  <!-- `my-auto` (not the parent's justify-between) centers this block vertically: it
       absorbs all the leftover space evenly above and below itself, while the logo
       and footer stay pinned to the panel's own top/bottom edges regardless of how
       tall the panel is. `mx-auto` centers it horizontally too, off the left edge
       the logo/footer still hug. -->
  <div class="mx-auto my-auto max-w-sm">
    <p class="text-2xl font-semibold leading-snug tracking-tight">
      {slide.headline}
    </p>
    <ul class="mt-6 flex flex-col gap-2 text-sm text-background/70">
      {#each slide.bullets as bullet (bullet)}
        <li>{bullet}</li>
      {/each}
    </ul>

    <!-- Landscape, not square: the tallest slide is three stacked rows (~160px), so a
         square box left the board slide with a third of its height empty. A fixed
         height rather than an aspect ratio — the column is max-w-sm, so this is the
         3/2 the ratio would have given, without an arbitrary Tailwind value. -->
    <div class="relative mt-6 h-64 w-full overflow-hidden rounded-2xl border border-background/15 bg-background/5 p-4">
      {#if active === 0}
        <div class="flex h-full flex-col justify-center gap-3">
          {#each [100, 72, 85] as w (w)}
            <div class="flex items-center gap-3 rounded-lg bg-background/10 p-3">
              <div class="size-8 shrink-0 rounded-md bg-background/20"></div>
              <div class="flex-1 space-y-1.5">
                <div class="h-2 rounded-full bg-background/40" style:width="{w}%"></div>
                <div class="h-2 w-1/3 rounded-full bg-background/20"></div>
              </div>
            </div>
          {/each}
        </div>
      {:else}
        <div class="grid h-full grid-cols-3 gap-2">
          {#each [1, 2, 1] as n, ci (ci)}
            <div class="flex flex-col gap-1.5">
              <div class="h-1.5 w-3/4 rounded-full bg-background/30"></div>
              {#each { length: n } as _, ri (ri)}
                <div class="h-8 rounded-md bg-background/10"></div>
              {/each}
            </div>
          {/each}
        </div>
      {/if}

      <div class="absolute inset-x-0 bottom-3 flex justify-center gap-1.5" role="tablist" aria-label="Highlights">
        {#each SLIDES as _, i (i)}
          <button
            type="button"
            role="tab"
            aria-selected={active === i}
            aria-label={`Show highlight ${i + 1} of ${SLIDES.length}`}
            onclick={() => select(i)}
            class={[
              'h-1.5 rounded-full transition-all',
              active === i ? 'w-6 bg-background' : 'w-1.5 bg-background/40 hover:bg-background/60',
            ]}
          ></button>
        {/each}
      </div>
    </div>
  </div>

  <div class="flex items-center gap-3 text-xs text-background/50">
    <span>Open source —</span>
    <GithubStars
      variant="inline"
      class="min-h-0 gap-1 px-0 text-xs text-background/50 hover:bg-transparent hover:text-background/80"
    />
    {#each otherSocials as social (social.provider)}
      <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external profile URL opened in a new tab; not an internal route -->
      <a href={social.href}
        target="_blank"
        rel="noopener noreferrer"
        aria-label={social.label}
        class="text-background/50 transition-colors hover:text-background/80"
      >
        <ProviderIcon provider={social.provider} />
      </a>
    {/each}
  </div>
</div>
