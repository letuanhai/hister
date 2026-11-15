<!-- SPDX-License-Identifier: AGPL-3.0-or-later -->
<script lang="ts">
  import { Button } from '@hister/components/ui/button';
  import * as Dialog from '@hister/components/ui/dialog';
  import { Input } from '@hister/components/ui/input';
  import { Label } from '@hister/components/ui/label';
  import { Textarea } from '@hister/components/ui/textarea';
  import TagInput from './TagInput.svelte';
  import {
    editDocument,
    summarizeDocument,
    type DocumentChanges,
    type EditableDocument,
  } from '#lib/tags.js';
  import { fetchConfig } from '#lib/api.js';
  import { untrack } from 'svelte';
  import { Pencil, Save, Sparkles, X } from '@lucide/svelte';

  interface Props {
    open: boolean;
    url: string;
    documentId?: string;
    initial: Omit<EditableDocument, 'url'>;
    onsaved?: (doc: EditableDocument) => void;
  }

  let { open = $bindable(), url, documentId = '', initial, onsaved }: Props = $props();

  let title = $state('');
  let label = $state('');
  let tags = $state<string[]>([]);
  let summary = $state('');
  let notes = $state('');
  let resetTitle = $state(false);
  let saving = $state(false);
  let error = $state('');
  let summariesEnabled = $state(false);
  let generating = $state(false);
  let summarySource = $state('');

  const titleEdited = $derived(initial.user_edited.includes('title'));
  const summaryEdited = $derived(initial.user_edited.includes('summary'));

  // Load the current values each time the dialog opens.
  $effect(() => {
    if (open) untrack(loadInitial);
  });

  function loadInitial() {
    title = initial.title;
    label = initial.label;
    tags = [...initial.tags];
    summary = initial.summary;
    notes = initial.notes;
    resetTitle = false;
    error = '';
    summarySource = initial.summary_source;
    fetchConfig()
      .then((cfg) => (summariesEnabled = !!cfg.summariesEnabled))
      .catch(() => (summariesEnabled = false));
  }

  async function generateSummary() {
    generating = true;
    error = '';
    try {
      const doc = await summarizeDocument(url, documentId);
      summary = doc.summary;
      summarySource = doc.summary_source;
      // The summary is stored already; the parent refreshes the stored values.
      onsaved?.({
        ...doc,
        title: initial.title,
        label: initial.label,
        tags: initial.tags,
        notes: initial.notes,
      });
    } catch (e) {
      error = e instanceof Error ? e.message : 'Generating the summary failed';
    } finally {
      generating = false;
    }
  }

  function changes(): DocumentChanges {
    const c: DocumentChanges = {};
    if (resetTitle) c.reset_title = true;
    else if (title.trim() !== initial.title) c.title = title.trim();
    if (label !== initial.label) c.label = label;
    if (tags.join(' ') !== initial.tags.join(' ')) c.tags = tags;
    if (summary.trim() !== initial.summary) c.summary = summary.trim();
    if (notes.trim() !== initial.notes) c.notes = notes.trim();
    return c;
  }

  async function save(event: SubmitEvent) {
    event.preventDefault();
    const c = changes();
    if (Object.keys(c).length === 0) {
      open = false;
      return;
    }
    saving = true;
    error = '';
    try {
      const doc = await editDocument(url, documentId, c);
      onsaved?.(doc);
      open = false;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Saving the document failed';
    } finally {
      saving = false;
    }
  }

  const fieldClass =
    'bg-page-bg border-brutal-border font-inter text-text-brand placeholder:text-text-brand-muted focus-visible:border-hister-teal w-full rounded-none border-[3px] text-sm';
  const labelClass = 'font-outfit text-text-brand text-sm font-bold';
</script>

<Dialog.Root bind:open>
  <Dialog.Content
    showCloseButton={false}
    class="border-border-brand bg-card-surface max-h-[90vh] max-w-lg gap-0 overflow-y-auto rounded-none border-[3px] p-0 shadow-[6px_6px_0px_var(--hister-teal)]"
  >
    <Dialog.Header class="bg-hister-teal flex-row items-center justify-between gap-2 px-5 py-4">
      <div class="min-w-0">
        <Dialog.Title class="flex items-center gap-2">
          <Pencil class="size-5 text-white" />
          <span class="font-outfit text-lg font-extrabold text-white">Edit entry</span>
        </Dialog.Title>
        <Dialog.Description class="font-inter mt-1 truncate text-sm text-white/80" title={url}>
          {url}
        </Dialog.Description>
      </div>
      <Dialog.Close class="p-0.5 text-white/70 hover:text-white" aria-label="Close edit dialog">
        <X class="size-5" />
      </Dialog.Close>
    </Dialog.Header>

    <form onsubmit={save}>
      <div class="space-y-4 px-5 py-5">
        <div class="space-y-1.5">
          <div class="flex items-center justify-between gap-2">
            <Label for="edit-title" class={labelClass}>Title</Label>
            {#if titleEdited}
              <label class="font-inter text-text-brand-muted flex items-center gap-1.5 text-xs">
                <input type="checkbox" bind:checked={resetTitle} />
                Use page title on next visit
              </label>
            {/if}
          </div>
          <Input
            id="edit-title"
            bind:value={title}
            disabled={resetTitle}
            class="{fieldClass} h-10"
          />
        </div>

        <div class="space-y-1.5">
          <Label for="edit-tags" class={labelClass}>Tags</Label>
          <TagInput id="edit-tags" bind:tags />
          <p class="font-inter text-text-brand-muted text-xs">
            Press Enter or comma to add. Search with <code>tag:name</code>.
          </p>
        </div>

        <div class="space-y-1.5">
          <div class="flex items-center justify-between gap-2">
            <Label for="edit-summary" class={labelClass}>Summary</Label>
            {#if summaryEdited || summarySource}
              <span class="font-inter text-text-brand-muted text-xs">
                {summaryEdited
                  ? 'Written by you'
                  : summarySource === 'llm'
                    ? 'Generated by a language model'
                    : 'Extracted automatically'}
              </span>
            {/if}
          </div>
          <Textarea
            id="edit-summary"
            bind:value={summary}
            rows={4}
            placeholder="Short description of this page"
            class="{fieldClass} p-3"
          />
          <div class="flex flex-wrap items-center gap-x-4 gap-y-1">
            {#if summariesEnabled}
              <button
                type="button"
                disabled={generating}
                class="font-inter text-hister-teal inline-flex cursor-pointer items-center gap-1 text-xs hover:underline disabled:opacity-50"
                onclick={generateSummary}
              >
                <Sparkles class="size-3" />
                {generating ? 'Generating…' : 'Generate with LLM'}
              </button>
            {/if}
            {#if summaryEdited}
              <button
                type="button"
                class="font-inter text-hister-teal cursor-pointer text-xs hover:underline"
                onclick={() => (summary = '')}
              >
                Return to the automatic summary
              </button>
            {/if}
          </div>
        </div>

        <div class="space-y-1.5">
          <Label for="edit-notes" class={labelClass}>Notes</Label>
          <Textarea
            id="edit-notes"
            bind:value={notes}
            rows={4}
            placeholder="Your notes, searchable with notes:"
            class="{fieldClass} p-3"
          />
        </div>

        <div class="space-y-1.5">
          <Label for="edit-label" class={labelClass}>Label</Label>
          <Input id="edit-label" bind:value={label} class="{fieldClass} h-10" />
        </div>

        {#if error}
          <p role="alert" class="font-inter text-sm text-red-500">{error}</p>
        {/if}
      </div>

      <Dialog.Footer class="border-border-brand-muted bg-muted-surface border-t-[3px] px-5 py-3">
        <Dialog.Close
          type="button"
          class="border-border-brand-muted bg-card-surface text-text-brand-secondary font-space h-9 border-[2px] px-4 text-xs font-bold uppercase"
        >
          Cancel
        </Dialog.Close>
        <Button
          type="submit"
          disabled={saving}
          class="bg-hister-teal font-space border-brutal-border brutal-press h-9 gap-2 rounded-none border-[3px] px-4 text-xs font-bold tracking-[0.5px] text-white uppercase"
        >
          <Save class="size-4" />
          {saving ? 'Saving…' : 'Save'}
        </Button>
      </Dialog.Footer>
    </form>
  </Dialog.Content>
</Dialog.Root>
