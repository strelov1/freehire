<script lang="ts">
  import type { JobCopy } from '$lib/api';

  // The per-city openings folded under this collapsed role. `copies` is a capped page;
  // `total` is the whole cluster's open size. Only worth a section for a genuinely
  // mass-posted role (more than one opening).
  let { copies, total }: { copies: JobCopy[]; total: number } = $props();
</script>

{#if total > 1}
  <section class="mt-10">
    <h2 class="mb-4 text-lg font-semibold">{total} openings across locations</h2>
    <ul class="divide-y divide-gray-100 overflow-hidden rounded-lg border border-gray-100">
      {#each copies as copy (copy.public_slug)}
        <li>
          <a
            href={`/jobs/${copy.public_slug}`}
            class="flex items-center justify-between px-4 py-2.5 text-sm hover:bg-gray-50"
          >
            <span class="text-gray-800">{copy.location || 'Location not specified'}</span>
            <span class="text-xs text-gray-400">View →</span>
          </a>
        </li>
      {/each}
    </ul>
  </section>
{/if}
