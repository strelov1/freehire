<script lang="ts">
  // The field hands its control four values through the children snippet, and
  // the wiring is the whole point of the primitive — a story that faked the
  // control with raw markup would show none of it.
  import FormField from '../../src/form-field.svelte';
  import Input from '../../src/input.svelte';

  let {
    label = 'Work email',
    hint,
    error,
    required = false,
  }: { label?: string; hint?: string; error?: string; required?: boolean } = $props();
</script>

<div class="max-w-sm">
  <FormField {label} {hint} {error} {required}>
    {#snippet children({ id, describedBy, required: isRequired, invalid })}
      <Input
        {id}
        required={isRequired}
        aria-describedby={describedBy}
        aria-invalid={invalid || undefined}
        type="email"
        placeholder="you@company.com"
        class="w-full"
      />
    {/snippet}
  </FormField>
</div>
