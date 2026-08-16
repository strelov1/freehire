<script lang="ts">
  import { Camera, Trash2 } from '@lucide/svelte';
  import { api, ApiError, PHOTO_ACCEPT, PHOTO_MAX_MB } from '$lib/api';
  import type { PhotoMeta } from '$lib/types';
  import HeadshotSilhouette from './HeadshotSilhouette.svelte';

  // The profile's headshot control: preview, upload/replace, remove. One photo per member,
  // reused by every CV whose template prints a portrait — so it lives here rather than in
  // the CV builder. The server normalizes whatever is uploaded to a square JPEG; this side
  // only pre-checks type and size so an obvious mistake costs no upload.
  //
  // The circle itself is the upload control — a camera badge overlays its corner rather
  // than a separate "Upload photo" button beside it, the same click-the-avatar pattern
  // most profile photos use. Remove stays a distinct small action underneath, since it is
  // destructive and must not share a hit target with "change it".
  //
  // Nothing renders at all when object storage is unconfigured: an upload control that can
  // only fail is worse than no control.

  let meta = $state<PhotoMeta | null>(null);
  let busy = $state(false);
  let error = $state<string | null>(null);
  let fileInput = $state<HTMLInputElement | null>(null);

  // The upload time doubles as a cache buster: the image URL is stable, so without it a
  // replacement would keep showing the old photo for the life of the cached response.
  const imageSrc = $derived(
    meta?.present ? `/api/v1/me/photo/image?v=${encodeURIComponent(meta.uploaded_at ?? '')}` : null,
  );

  $effect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const loaded = await api.getPhoto();
        if (!cancelled) meta = loaded;
      } catch {
        // Best-effort: a failed status read leaves the control hidden rather than
        // showing an error on a section the member did not ask about.
      }
    })();
    return () => {
      cancelled = true;
    };
  });

  async function upload(file: File) {
    if (!PHOTO_ACCEPT.split(',').includes(file.type)) {
      error = 'Choose a JPEG, PNG, or WebP image.';
      return;
    }
    busy = true;
    error = null;
    try {
      meta = await api.putPhoto(file);
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not upload the photo. Please try again.';
    } finally {
      busy = false;
    }
  }

  async function remove() {
    busy = true;
    error = null;
    try {
      await api.deletePhoto();
      meta = { enabled: true, present: false };
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not remove the photo. Please try again.';
    } finally {
      busy = false;
    }
  }

  function onFile(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    // Clear the input so picking the same file twice still fires a change event.
    input.value = '';
    if (file) void upload(file);
  }
</script>

{#if meta?.enabled}
  <div class="flex flex-col gap-1.5">
    <span class="text-sm font-medium">Your photo</span>
    <div class="flex items-center gap-4">
      <input
        type="file"
        accept={PHOTO_ACCEPT}
        class="hidden"
        bind:this={fileInput}
        onchange={onFile}
      />
      <div class="relative size-20 shrink-0">
        {#if imageSrc}
          <img src={imageSrc} alt="Your headshot" class="size-20 rounded-full border border-border object-cover" />
        {:else}
          <HeadshotSilhouette class="size-20 rounded-full border border-border" />
        {/if}
        <button
          type="button"
          disabled={busy}
          onclick={() => fileInput?.click()}
          aria-label={meta.present ? 'Replace photo' : 'Upload photo'}
          class="absolute -bottom-1 -right-1 flex size-7 items-center justify-center rounded-full border-2 border-background bg-brand text-brand-foreground transition-opacity hover:opacity-90 disabled:opacity-70"
        >
          <Camera class="size-3.5" />
        </button>
      </div>
      <div class="flex flex-col items-start gap-2">
        <span class="text-xs text-muted-foreground">
          {#if busy}
            Working…
          {:else}
            JPEG, PNG, or WebP up to {PHOTO_MAX_MB} MB · cropped to a square. Printed only by
            the CV templates that show a photo.
          {/if}
        </span>
        {#if meta.present}
          <button
            type="button"
            disabled={busy}
            onclick={remove}
            class="flex items-center gap-1 text-xs text-muted-foreground hover:text-destructive disabled:opacity-70"
          >
            <Trash2 class="size-3.5" />
            Remove photo
          </button>
        {/if}
      </div>
    </div>
    {#if error}
      <p class="text-sm text-destructive" aria-live="polite">{error}</p>
    {/if}
  </div>
{/if}
