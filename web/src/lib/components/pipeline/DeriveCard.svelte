<script lang="ts">
  import { Chip } from '#lib/ui/index.js';
  // Same raw title as IngestCard, now legible, with the facets a dictionary lookup
  // actually found underneath it — nothing here is a guess, so there are exactly four
  // chips because exactly four terms in this title matched something.
</script>

<div class="flex h-28 flex-col justify-center gap-2" aria-hidden="true">
  <div class="rounded-md border border-border bg-card px-3 py-2">
    <p class="truncate text-xs font-medium">Senior Software Engineer — Berlin, DE</p>
  </div>
  <div class="flex flex-wrap gap-1 pl-1">
    <Chip variant="brand">Senior</Chip>
    <Chip variant="brand">Software Engineer</Chip>
    <Chip variant="brand">Berlin, DE</Chip>
    <Chip variant="brand">Remote-friendly</Chip>
  </div>
</div>
