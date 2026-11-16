<script lang="ts">
  // The experience bank's notice that the assistant recorded achievements the candidate has
  // not confirmed yet; clicking jumps to one.
  let { count, onJump }: { count: number; onJump: () => void } = $props();
</script>

<button
  type="button"
  onclick={onJump}
  class="rounded-lg bg-warning/5 px-4 py-3 text-left text-sm transition-colors hover:bg-warning/10"
>
  <strong class="font-medium">{count} not confirmed.</strong>
  The assistant recorded these as its own reading of something you said. They will not appear on any CV until you confirm
  them — click to jump to one, then confirm it as-is, edit it to change it first, or remove it.
</button>
