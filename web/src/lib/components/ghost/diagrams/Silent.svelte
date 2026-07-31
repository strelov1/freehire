<script lang="ts">
  // Three applications, one outcome that is evidence.
  //
  // The lane that counts is the loud one and the other two recede. Drawn at equal weight
  // — which is how this began — three grey tracks read as a loading skeleton rather than
  // a comparison, and the reader has toparse labels to find which row matters.
  //
  // The hollow third lane is the correctness core rather than an edge case. Without a
  // connected mailbox a reply could never have been seen, so silence is indistinguishable
  // from a gap in our own data and counts for nothing. It is hollow because it is
  // unobserved — not greyed as though it had been checked and cleared.
  const LANES = [
    { mark: '∅', label: 'no reply past the window', counts: true, hollow: false },
    { mark: '↩', label: 'a reply arrived', counts: false, hollow: false },
    { mark: '⊘', label: 'no mailbox connected', counts: false, hollow: true },
  ] as const;
</script>

<div class="flex h-24 flex-col justify-center gap-3" aria-hidden="true">
  {#each LANES as lane (lane.label)}
    <div class="flex items-center gap-2.5">
      <span
        class="size-2 shrink-0 rounded-full {lane.hollow
          ? 'border border-dashed border-muted-foreground/60'
          : lane.counts
            ? 'bg-warning'
            : 'bg-muted-foreground/60'}"
      ></span>
      <span
        class="h-px flex-1 {lane.hollow
          ? 'border-t border-dashed border-muted-foreground/40'
          : lane.counts
            ? 'bg-warning/50'
            : 'bg-muted-foreground/25'}"
      ></span>
      <span
        class="w-4 shrink-0 text-center text-base leading-none {lane.counts
          ? 'text-warning-strong'
          : 'text-muted-foreground/60'}"
      >
        {lane.mark}
      </span>
      <span
        class="w-32 shrink-0 text-xs {lane.counts
          ? 'font-medium text-foreground'
          : 'text-muted-foreground'}"
      >
        {lane.label}
      </span>
    </div>
  {/each}
</div>
