<script lang="ts">
  import { companyLogoUrl } from '#lib/logo.js';
  import { EntityLogo } from '#lib/ui/index.js';

  // An overlapping row of company logos (an avatar stack), the company counterpart of
  // CountryFlagStack: each logo laps over the previous one, earlier ones on top so the
  // row reads left to right, and a ring in the card colour keeps neighbours legible
  // where they touch. Decorative — the text beside it names the collection.
  let { names }: { names: string[] } = $props();
</script>

<span class="flex shrink-0 items-center" aria-hidden="true">
  {#each names as name, i (name)}
    <span class={['relative', i > 0 && '-ml-2.5']} style:z-index={names.length - i}>
      <EntityLogo
        {name}
        src={companyLogoUrl(name) ?? undefined}
        shape="circle"
        class="size-7 bg-background ring-2 ring-background"
      />
    </span>
  {/each}
</span>
