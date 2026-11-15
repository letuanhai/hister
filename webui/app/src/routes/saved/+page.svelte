<!-- SPDX-License-Identifier: AGPL-3.0-or-later -->
<script lang="ts">
  import { onMount } from 'svelte';
  import { PageHeader } from '@hister/components';
  import { listOfflineEntries, removeOfflineEntry, type OfflineEntry } from '#lib/offline.js';
  import { ArrowLeft, Trash2, WifiOff } from '@lucide/svelte';

  let entries = $state<OfflineEntry[]>([]);
  let loading = $state(true);
  let error = $state('');
  let filter = $state('');
  let reading = $state<OfflineEntry | null>(null);
  let online = $state(true);

  const visible = $derived.by(() => {
    const q = filter.trim().toLowerCase();
    if (!q) return entries;
    return entries.filter((e) =>
      [e.title, e.url, e.summary, ...e.tags].some((v) => v.toLowerCase().includes(q)),
    );
  });

  onMount(() => {
    online = navigator.onLine;
    const update = () => (online = navigator.onLine);
    addEventListener('online', update);
    addEventListener('offline', update);
    listOfflineEntries()
      .then((list) => (entries = list))
      .catch(() => (error = 'Saved pages are not available in this browser.'))
      .finally(() => (loading = false));
    return () => {
      removeEventListener('online', update);
      removeEventListener('offline', update);
    };
  });

  async function remove(entry: OfflineEntry) {
    await removeOfflineEntry(entry.key);
    entries = entries.filter((e) => e.key !== entry.key);
    if (reading?.key === entry.key) reading = null;
  }
</script>

<svelte:head>
  <title>Hister - Saved for offline</title>
</svelte:head>

<div class="flex min-h-0 flex-1 flex-col">
  <header class="border-brutal-border bg-card-surface shrink-0 border-b-[3px] px-3 py-3 md:px-6">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <PageHeader color="hister-teal" size="sm" class="min-w-0 shrink-0" truncate>
        Saved for offline
      </PageHeader>
      {#if !online}
        <span class="font-inter text-text-brand-muted inline-flex items-center gap-1.5 text-sm">
          <WifiOff class="size-4" /> You are offline
        </span>
      {/if}
    </div>
  </header>

  {#if reading}
    <div class="flex min-h-0 flex-1 flex-col">
      <div
        class="border-border-brand-muted flex shrink-0 items-center justify-between gap-2 border-b px-3 py-2 md:px-6"
      >
        <button
          type="button"
          class="font-inter text-hister-teal inline-flex cursor-pointer items-center gap-1 text-sm hover:underline"
          onclick={() => (reading = null)}
        >
          <ArrowLeft class="size-4" /> All saved pages
        </button>
        <span class="font-inter text-text-brand-muted text-xs">
          Saved {new Date(reading.savedAt).toLocaleString()}
        </span>
      </div>
      <!-- Saved pages are untrusted content: no scripts, no same origin access. -->
      <iframe
        title={reading.title}
        srcdoc={reading.html}
        sandbox="allow-popups allow-popups-to-escape-sandbox"
        referrerpolicy="no-referrer"
        class="min-h-0 w-full flex-1 bg-white"
      ></iframe>
    </div>
  {:else}
    <div class="font-inter mx-auto w-full max-w-4xl flex-1 px-3 py-4 md:px-6">
      <p class="text-text-brand-muted mb-3 text-sm">
        Pages you save from a preview are kept in this browser and can be read without a connection.
      </p>
      <input
        type="search"
        bind:value={filter}
        placeholder="Filter saved pages"
        aria-label="Filter saved pages"
        class="bg-page-bg border-brutal-border text-text-brand mb-4 h-10 w-full border-[3px] px-3 text-sm outline-none"
      />
      {#if loading}
        <p role="status" class="text-text-brand-muted text-sm">Loading saved pages…</p>
      {:else if error}
        <p role="alert" class="text-sm text-red-500">{error}</p>
      {:else if visible.length === 0}
        <p class="text-text-brand-muted text-sm">
          {entries.length ? 'No saved page matches the filter.' : 'No pages saved yet.'}
        </p>
      {:else}
        <ul class="divide-border-brand-muted flex flex-col divide-y">
          {#each visible as entry (entry.key)}
            <li class="flex items-start justify-between gap-3 py-3">
              <button
                type="button"
                class="min-w-0 cursor-pointer text-left"
                onclick={() => (reading = entry)}
              >
                <span class="font-outfit text-text-brand block font-bold hover:underline"
                  >{entry.title}</span
                >
                <span class="text-text-brand-muted block truncate text-xs">{entry.url}</span>
                {#if entry.summary}
                  <span class="text-text-brand-secondary mt-1 line-clamp-2 block text-sm"
                    >{entry.summary}</span
                  >
                {/if}
                <span class="text-text-brand-muted mt-1 block text-xs">
                  Saved {new Date(entry.savedAt).toLocaleDateString()} · {entry.source === 'archive'
                    ? 'reader archive'
                    : 'stored preview'}
                  {#each entry.tags as tag (tag)}<span class="ml-1">#{tag}</span>{/each}
                </span>
              </button>
              <button
                type="button"
                class="text-text-brand-muted shrink-0 cursor-pointer hover:text-red-500"
                aria-label={`Remove ${entry.title}`}
                title="Remove from offline storage"
                onclick={() => remove(entry)}
              >
                <Trash2 class="size-4" />
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  {/if}
</div>
