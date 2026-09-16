<script lang="ts">
  import { resolve } from '$app/paths';

  // The one link every spent-daily-allowance message points to. `/my/plan` shows the
  // candidate exactly what they've used today and where to upgrade — see PlanView.svelte —
  // so every refusal banner sends them there instead of stating the limit and nothing else.
</script>

<a href={resolve('/my/plan')} class="text-xs font-medium underline underline-offset-4">
  See your plan
</a>
