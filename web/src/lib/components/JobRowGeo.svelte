<script lang="ts">
  import { MapPin } from '@lucide/svelte';
  import CountryFlagStack from './CountryFlagStack.svelte';

  // A job card's geography line: the arrangement and region (`geoTags`), the eligible
  // countries as flags (at most `maxFlags`), and the place (`place`, from `location`) the
  // flags do not name.
  let {
    geoTags,
    countries,
    maxFlags,
    place,
    location,
  }: {
    geoTags: string[];
    countries: string[] | null | undefined;
    maxFlags: number;
    place: string | null | undefined;
    location: string | null | undefined;
  } = $props();
</script>

<div class="mt-1.5 flex flex-wrap items-center gap-x-1.5 gap-y-1 text-xs font-medium text-muted-foreground">
  {#each geoTags as tag, i (tag)}
    {#if i > 0}<span aria-hidden="true">·</span>{/if}
    <span>{tag}</span>
  {/each}
  <!-- Eligible countries as an overlapping flag cluster. Display-only here: the
       stretched title link handles navigation, so the flags carry no filter links. -->
  {#if countries?.length}
    <CountryFlagStack codes={countries} max={maxFlags} class={geoTags.length ? 'ml-1 text-base' : 'text-base'} />
  {/if}
  <!-- The place the flags do not name: one employer's postings in seven cities
       otherwise read identically until opened. -->
  {#if place}
    <span title={location} class="flex min-w-0 max-w-56 items-center gap-1">
      <MapPin class="size-3.5 shrink-0" aria-hidden="true" />
      <span class="truncate">{place}</span>
    </span>
  {/if}
</div>
