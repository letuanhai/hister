<!-- SPDX-License-Identifier: AGPL-3.0-or-later -->
<script lang="ts">
  import {
    archiveUrl,
    createArchive,
    deleteArchive,
    fetchArchiverStatus,
    formatBytes,
    listArchives,
    type ArchiveOptions,
    type DocumentArchive,
  } from '#lib/archives.js';
  import { Archive, ExternalLink, Trash2 } from '@lucide/svelte';
  import { onMount } from 'svelte';

  interface Props {
    url: string;
    documentId?: string;
    oncount?: (n: number) => void;
  }

  let { url, documentId = '', oncount }: Props = $props();

  let archives = $state<DocumentArchive[]>([]);
  let loading = $state(true);
  let archiving = $state(false);
  let error = $state('');
  let options = $state<ArchiveOptions>({});

  onMount(async () => {
    try {
      const [status, list] = await Promise.all([
        fetchArchiverStatus(),
        listArchives(url, documentId),
      ]);
      options = {
        engine: status.engine,
        keep_images: status.keep_images,
        mobile: status.mobile,
        adblock: status.adblock,
        video: status.video,
      };
      archives = list;
      oncount?.(archives.filter((a) => a.status === 'ready').length);
    } catch {
      error = 'Could not load archives. Please try again.';
    } finally {
      loading = false;
    }
  });

  async function archiveNow() {
    archiving = true;
    error = '';
    try {
      const archive = await createArchive(url, documentId, options);
      archives = [archive, ...archives];
      oncount?.(archives.filter((a) => a.status === 'ready').length);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Archiving failed';
      archives = await listArchives(url, documentId).catch(() => archives);
    } finally {
      archiving = false;
    }
  }

  async function remove(archive: DocumentArchive) {
    try {
      await deleteArchive(archive.id);
      archives = archives.filter((a) => a.id !== archive.id);
      oncount?.(archives.filter((a) => a.status === 'ready').length);
    } catch {
      error = 'Could not delete the archive. Please try again.';
    }
  }

  const engineLabels: Record<string, string> = {
    stored: 'Stored page',
    browser: 'Headless browser',
    singlefile: 'SingleFile',
  };
</script>

<div class="font-inter p-4 text-sm">
  <p class="font-outfit text-text-brand-muted mb-3 text-xs font-bold tracking-widest uppercase">
    Reader archives
  </p>
  <p class="text-text-brand-muted mb-3 text-xs">
    An archive keeps the main text of the page, like reader view, so you can read it offline even
    when the page changes or disappears.
  </p>
  <form
    class="border-border-brand-muted mb-4 flex flex-wrap items-center gap-x-4 gap-y-2 border p-3"
    onsubmit={(e) => {
      e.preventDefault();
      void archiveNow();
    }}
  >
    <label class="flex items-center gap-1.5 text-xs">
      Engine
      <select
        bind:value={options.engine}
        class="bg-page-bg border-brutal-border border-[2px] px-1 py-0.5 text-xs"
      >
        {#each Object.entries(engineLabels) as [value, label] (value)}
          <option {value}>{label}</option>
        {/each}
      </select>
    </label>
    <label class="flex items-center gap-1.5 text-xs"
      ><input type="checkbox" bind:checked={options.keep_images} /> Images</label
    >
    <label class="flex items-center gap-1.5 text-xs"
      ><input type="checkbox" bind:checked={options.mobile} /> Mobile version</label
    >
    <label class="flex items-center gap-1.5 text-xs"
      ><input type="checkbox" bind:checked={options.adblock} /> Block ads</label
    >
    <label class="flex items-center gap-1.5 text-xs"
      ><input type="checkbox" bind:checked={options.video} /> Video</label
    >
    <button
      type="submit"
      disabled={archiving || loading}
      class="bg-hister-teal font-space border-brutal-border inline-flex cursor-pointer items-center gap-1.5 border-[2px] px-3 py-1 text-xs font-bold text-white uppercase disabled:opacity-50"
    >
      <Archive class="size-3.5" />
      {archiving ? 'Archiving…' : 'Archive now'}
    </button>
  </form>
  {#if error}
    <p role="alert" class="mb-3 text-xs text-red-500">{error}</p>
  {/if}
  {#if loading}
    <p role="status" class="text-text-brand-muted text-sm">Loading archives…</p>
  {:else if archives.length === 0}
    <p class="text-text-brand-muted text-sm">No archives yet.</p>
  {:else}
    <ul class="divide-border-brand-muted flex flex-col divide-y">
      {#each archives as archive (archive.id)}
        <li class="flex items-center justify-between gap-3 py-2">
          <div class="min-w-0">
            <p class="text-text-brand text-xs font-semibold">
              {new Date(archive.created_at).toLocaleString()}
            </p>
            <p class="text-text-brand-muted text-xs">
              {engineLabels[archive.engine] ?? archive.engine}
              {#if archive.status === 'ready'}
                · {formatBytes(archive.size)}{archive.images ? '' : ' · no images'}{archive.mobile
                  ? ' · mobile'
                  : ''}
                {#if archive.video_status === 'ready'}· video{:else if archive.video_status === 'pending'}·
                  downloading video…{:else if archive.video_status === 'failed'}· video failed{/if}
              {:else if archive.status === 'failed'}
                · <span class="text-red-500">failed: {archive.error}</span>
              {:else}
                · pending
              {/if}
            </p>
          </div>
          <div class="flex shrink-0 items-center gap-2">
            {#if archive.status === 'ready'}
              <a
                href={archiveUrl(archive.id)}
                target="_blank"
                rel="noopener noreferrer"
                class="text-hister-teal inline-flex items-center gap-1 text-xs hover:underline"
                ><ExternalLink class="size-3" /> Read</a
              >
            {/if}
            <button
              type="button"
              class="text-text-brand-muted cursor-pointer hover:text-red-500"
              aria-label="Delete archive"
              title="Delete archive"
              onclick={() => remove(archive)}
            >
              <Trash2 class="size-3.5" />
            </button>
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</div>
