<script lang="ts">
  import { resolve } from '$app/paths';
  import { Bookmark, ExternalLink, EyeOff, FileText } from '@lucide/svelte';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { Button } from '#lib/ui/index.js';

  // JobRow's corner controls on a normal (not compact) card: Apply, Hide and Save.
  let {
    slug,
    title,
    directApply,
    applyUrl,
    onApplyClick,
    onHide,
    hiding,
    saved,
    saving,
    onToggleSave,
  }: {
    slug: string;
    title: string;
    directApply: boolean;
    /** The employer's own posting, when the card can link out to it. */
    applyUrl: string | null;
    onApplyClick: (e: MouseEvent) => void;
    /** Absent where the feed offers no "hide this job". */
    onHide: (() => void) | undefined;
    hiding: boolean;
    saved: boolean;
    saving: boolean;
    onToggleSave: () => void;
  } = $props();
</script>

<!-- The card's controls ride its top-right corner, beside the date, rather than a row of
   their own at its foot: that row added ~40px to every card in a feed of dozens. Save
   is a bookmark glyph with its title and accessible name; Apply and
   Hide, where the card has them, hover-reveal beside it (over the date, which they cover
   only while the pointer is on the card). `pointer-coarse`
   keeps Hide visible on touch, which has no hover to reveal anything; there Apply shrinks
   to its icon and the date moves to the meta line, so the company name is not cut to
   two letters. The company rail reserves the corner (see its `pr-*`), so the
   date never slides under it. -->
<div class="absolute right-3 top-3 z-10 flex items-center gap-1">
  <!-- Direct-apply shortcut (freehire#2755): a way out to the posting's own site
     without opening the job page first. Outline, never the brand-green primary —
     that loud slot is reserved for auto-apply on the job page itself, and a list
     card outshouting the page it links to states the wrong priority. `bg-transparent`
     drops the outline variant's default `bg-background` fill: unlike a lone button on
     a page, this one sits on the card's own background, and that fill read as a solid
     chip louder than every other control on the card — Save carries the same border
     with no fill, and Apply should read as its equal, not its loudest element.
     Hover-reveal matches Hide; `pointer-coarse` is load-bearing, not decoration — a
     phone has no hover, so without it the button is invisible to every touch visitor.
     `nofollow`: the destination is the poster's own site, which the catalogue never
     vetted (the same stance JobView.svelte's own Apply link takes) — without it a
     submitted vacancy buys a followed link from every card in every list. -->
  {#if directApply}
    <Button
      variant="outline"
      href={resolve('/jobs/[slug]', { slug })}
      aria-label="Apply to {title}"
      class="h-8 shrink-0 bg-transparent opacity-0 focus-visible:opacity-100 group-hover:opacity-100 pointer-coarse:w-8 pointer-coarse:px-0 pointer-coarse:opacity-100"
    >
      <FileText class="hidden size-4 pointer-coarse:block" aria-hidden="true" />
      <span class="pointer-coarse:sr-only">Apply</span>
    </Button>
  {/if}
  {#if applyUrl}
    <Button
      variant="outline"
      href={isAuthenticated() ? applyUrl : undefined}
      target="_blank"
      rel="nofollow noopener noreferrer"
      onclick={onApplyClick}
      aria-label={isAuthenticated()
        ? `Apply to ${title} on the employer's site (opens in a new tab)`
        : `Choose how to apply to ${title}`}
      class="h-8 shrink-0 bg-transparent opacity-0 focus-visible:opacity-100 group-hover:opacity-100 pointer-coarse:w-8 pointer-coarse:px-0 pointer-coarse:opacity-100"
    >
      <ExternalLink class="size-4" aria-hidden="true" />
      <span class="pointer-coarse:sr-only">Apply</span>
    </Button>
  {/if}

  {#if onHide}
    <button
      type="button"
      onclick={onHide}
      disabled={hiding}
      aria-label="Hide this job"
      title="Not interested — hide this job"
      class="grid size-8 place-items-center rounded-lg border border-transparent text-muted-foreground opacity-0 transition hover:border-border hover:bg-accent hover:text-foreground focus-visible:opacity-100 group-hover:opacity-100 pointer-coarse:opacity-100 disabled:pointer-events-none disabled:opacity-50"
    >
      <EyeOff class="size-3.5" aria-hidden="true" />
    </button>
  {/if}
  <!-- No `aria-pressed`, unlike the compact glyph: this button's visible name already
     changes with the state, so the toggle role would announce the same fact twice.
     "Saved" is a state, though, and on its own leaves a screen reader user with a
     button whose effect is unstated — so each state's name says what activating does,
     and keeps the visible word in it. -->
  <button
    type="button"
    onclick={onToggleSave}
    disabled={saving}
    aria-label={saved ? 'Remove from saved' : 'Save to board'}
    title={saved ? 'On your board' : 'Save to board'}
    class={[
      'grid size-8 shrink-0 place-items-center rounded-lg transition disabled:pointer-events-none disabled:opacity-50',
      saved
        ? 'bg-brand/10 text-brand hover:bg-brand/15'
        : 'text-muted-foreground hover:bg-background hover:text-foreground',
    ]}
  >
    <Bookmark class="size-4 {saved ? 'fill-current' : ''}" aria-hidden="true" />
    <!-- Icon only, with its title and aria-label carrying the action: twenty labelled Save
       buttons down a feed were repeated chrome beside each title. -->
    <span class="sr-only">{saved ? 'Saved' : 'Save'}</span>
  </button>
</div>
