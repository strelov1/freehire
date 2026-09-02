<script lang="ts">
  // The attribution Adzuna's API terms require beside every advert we display from their feed:
  //
  //   "An API user shall label each displayed advert with the phrase 'Jobs by Adzuna' at least
  //    116 X 23 pixels in size, wherein the word 'Jobs' shall be hyperlinked to
  //    http://www.adzuna.co.uk or the relevant local domain and the word 'Adzuna' shall be the
  //    Adzuna Logo Image and shall also be hyperlinked to http://www.adzuna.co.uk or the
  //    relevant local domain."
  //
  // So this is not a decorative credit and its parts are not ours to restyle: the phrase, the
  // two links, the logo standing in for the word, and the floor on the rendered size are all
  // stated. The 116x23 floor is enforced here rather than left to the surrounding layout,
  // because a caller that shrank it would put us out of compliance silently.
  //
  // The logo is served from our own origin (static/adzuna-logo.svg, their official mark) rather
  // than hotlinked from their CDN: a third-party CDN path is theirs to change without telling
  // us, and hotlinking would hand every reader's IP to it just to render a credit.

  import { adzunaLocalDomain } from '$lib/adzuna';

  interface Props {
    /** The posting's outbound URL — its host names which local Adzuna domain to link to. */
    jobUrl: string;
  }

  const { jobUrl }: Props = $props();

  const domain = $derived(adzunaLocalDomain(jobUrl));
</script>

<!-- eslint-disable svelte/no-navigation-without-resolve -- external Adzuna domain required by their API terms, not an internal route -->
<span class="inline-flex min-h-[23px] min-w-[116px] items-center justify-center gap-1.5 align-middle">
  <!-- The terms hyperlink the word "Jobs" specifically, so "by" stays plain text between the
       two links rather than riding along inside the first one. -->
  <span class="text-xs leading-none text-muted-foreground">
    <a href={domain} target="_blank" rel="noopener" class="underline-offset-2 hover:underline"
      >Jobs</a
    > by
  </span>
  <a href={domain} target="_blank" rel="noopener" class="inline-flex items-center">
    <!-- The mark stands in for the word "Adzuna", so the alt text IS that word. -->
    <img src="/adzuna-logo.svg" alt="Adzuna" width="76" height="20" class="h-5 w-auto" />
  </a>
</span>
