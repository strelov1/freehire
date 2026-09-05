<script lang="ts">
  // The candidate's LinkedIn and GitHub, pre-filled from whatever the CV extract found.
  //
  // Two named boxes over ONE stored list: the server keeps a flat `links` array whether a
  // link was extracted or typed, and naming the two here is a presentation choice — see
  // profileLinks.ts for why the sorting lives in the frontend and why the host rule is an
  // exact match rather than a suffix test.
  //
  // A link the classifier did not recognise is neither shown nor lost: it rides along in
  // `other` and is written back untouched.
  import type { ProfileLinks } from '$lib/profileLinks';

  interface Props {
    value: ProfileLinks;
    onChange: (next: ProfileLinks) => void;
    /** True when at least one of the two arrived from the CV rather than being typed, which
     *  is the only reason to explain where the text came from. */
    prefilled: boolean;
  }

  let { value, onChange, prefilled }: Props = $props();
</script>

<div class="mb-2 flex min-h-6 items-center justify-between gap-2">
  <span class="text-sm font-medium">Profile links</span>
  {#if prefilled}
    <span class="text-xs text-muted-foreground">Found on your CV</span>
  {/if}
</div>
<div class="flex flex-col gap-2">
  <div>
    <label for="link-linkedin" class="sr-only">Your LinkedIn profile</label>
    <input
      id="link-linkedin"
      value={value.linkedin}
      oninput={(e) => onChange({ ...value, linkedin: e.currentTarget.value })}
      type="text"
      inputmode="url"
      autocomplete="url"
      placeholder="linkedin.com/in/your-name"
      class="w-full rounded-xl border border-input bg-card px-3 py-2.5 text-sm outline-none focus:ring-2 focus:ring-ring"
    />
  </div>
  <div>
    <label for="link-github" class="sr-only">Your GitHub profile</label>
    <input
      id="link-github"
      value={value.github}
      oninput={(e) => onChange({ ...value, github: e.currentTarget.value })}
      type="text"
      inputmode="url"
      autocomplete="url"
      placeholder="github.com/your-name"
      class="w-full rounded-xl border border-input bg-card px-3 py-2.5 text-sm outline-none focus:ring-2 focus:ring-ring"
    />
  </div>
</div>
