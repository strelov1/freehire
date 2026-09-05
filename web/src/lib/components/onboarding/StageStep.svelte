<script lang="ts">
  // "Where are you in your search?" — one of the two questions that describe the candidate
  // to us rather than filtering what they see, which is why both sit at the end of the
  // wizard: a run abandoned before here has still answered everything the product acts on.
  import OptionList from './OptionList.svelte';
  import { JOB_SEARCH_STAGE_OPTIONS } from '$lib/surveyOptions';

  interface Props {
    value: string | null;
    onChange: (value: string) => void;
  }

  let { value, onChange }: Props = $props();
</script>

<h2 class="text-xl font-semibold tracking-tight">Where are you in your job search?</h2>
<p class="mt-1 text-sm text-muted-foreground">
  It tells us how often to get in touch — and how loud to be about it.
</p>

<div class="mt-5">
  <OptionList
    options={JOB_SEARCH_STAGE_OPTIONS}
    {value}
    onSelect={onChange}
    label="Where you are in your job search"
  />
</div>
