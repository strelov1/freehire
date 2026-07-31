<script lang="ts">
  // Three applications, three outcomes — only one of which is evidence.
  //
  // The hollow third lane is the correctness core rather than an edge case. Without a
  // connected mailbox a reply could never have been seen, so silence is indistinguishable
  // from a gap in our own data and counts for nothing. Drawing it in a reassuring tone
  // would claim it had been checked and cleared; drawing it like the first would claim
  // evidence we do not have. It is hollow because it is unobserved.
  const LANES = [
    { label: 'no reply — counts', state: 'counts' },
    { label: 'reply arrived', state: 'answered' },
    { label: 'no mailbox', state: 'unobserved' },
  ] as const;
</script>

<div class="flex h-24 flex-col justify-center gap-2.5 text-[10px]" aria-hidden="true">
  {#each LANES as lane (lane.label)}
    <div class="flex items-center gap-2">
      <!-- The apply date. Hollow where the application can never be evidence. -->
      <span
        class="size-2 shrink-0 rounded-full {lane.state === 'unobserved'
          ? 'border border-dashed border-muted-foreground/60'
          : 'bg-muted-foreground'}"
      ></span>

      <!-- The window the stage tolerates. -->
      <span
        class="h-1.5 flex-1 rounded-full {lane.state === 'unobserved'
          ? 'border border-dashed border-muted-foreground/40'
          : 'bg-muted-foreground/25'}"
      ></span>

      <span
        class="w-24 shrink-0 whitespace-nowrap {lane.state === 'counts'
          ? 'text-warning-strong'
          : 'text-muted-foreground'}"
      >
        {lane.state === 'counts' ? '⌀ ' : lane.state === 'answered' ? '↩ ' : ''}{lane.label}
      </span>
    </div>
  {/each}
</div>
