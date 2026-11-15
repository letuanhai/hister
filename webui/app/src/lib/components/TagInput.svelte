<!-- SPDX-License-Identifier: AGPL-3.0-or-later -->
<script lang="ts">
  import { fetchTagSuggestions, normalizeTag, type TagSuggestion } from '#lib/tags.js';
  import { createDebouncer } from '#lib/debounce.js';
  import { X } from '@lucide/svelte';

  interface Props {
    tags: string[];
    id?: string;
    placeholder?: string;
  }

  let { tags = $bindable(), id = 'tag-input', placeholder = 'Add a tag…' }: Props = $props();

  let input = $state('');
  let suggestions = $state<TagSuggestion[]>([]);
  let open = $state(false);
  let active = $state(-1);
  let requestId = 0;
  const debouncer = createDebouncer(() => void loadSuggestions(input), 150);
  const listId = $derived(`${id}-suggestions`);
  const visibleSuggestions = $derived(suggestions.filter((s) => !tags.includes(s.tag)));

  function addTag(raw: string) {
    const tag = normalizeTag(raw);
    if (tag && !tags.includes(tag)) tags = [...tags, tag].sort();
    input = '';
    active = -1;
    void loadSuggestions('');
  }

  function removeTag(tag: string) {
    tags = tags.filter((t) => t !== tag);
  }

  async function loadSuggestions(prefix: string) {
    const current = ++requestId;
    try {
      const result = await fetchTagSuggestions(prefix);
      if (current === requestId) suggestions = result;
    } catch {
      if (current === requestId) suggestions = [];
    }
  }

  function oninput() {
    open = true;
    active = -1;
    if (/[,\n]/.test(input)) {
      for (const part of input.split(/[,\n]/).slice(0, -1)) addTag(part);
      input = input.split(/[,\n]/).pop() ?? '';
    }
    debouncer.schedule();
  }

  function onkeydown(event: KeyboardEvent) {
    if (event.key === 'ArrowDown' && visibleSuggestions.length) {
      event.preventDefault();
      open = true;
      active = (active + 1) % visibleSuggestions.length;
    } else if (event.key === 'ArrowUp' && visibleSuggestions.length) {
      event.preventDefault();
      active = active <= 0 ? visibleSuggestions.length - 1 : active - 1;
    } else if (event.key === 'Enter' || event.key === 'Tab') {
      if (open && active >= 0 && visibleSuggestions[active]) {
        event.preventDefault();
        addTag(visibleSuggestions[active].tag);
      } else if (input.trim()) {
        event.preventDefault();
        addTag(input);
      }
    } else if (event.key === 'Backspace' && !input && tags.length) {
      tags = tags.slice(0, -1);
    } else if (event.key === 'Escape' && open) {
      event.stopPropagation();
      open = false;
    }
  }
</script>

<div class="relative">
  <div
    class="bg-page-bg border-brutal-border focus-within:border-hister-teal flex min-h-10 flex-wrap items-center gap-1.5 border-[3px] px-2 py-1.5"
  >
    {#each tags as tag (tag)}
      <span
        class="bg-muted-surface text-text-brand font-inter inline-flex items-center gap-1 px-1.5 py-0.5 text-xs"
      >
        #{tag}
        <button
          type="button"
          class="text-text-brand-muted hover:text-text-brand cursor-pointer"
          aria-label={`Remove tag ${tag}`}
          onclick={() => removeTag(tag)}
        >
          <X class="size-3" />
        </button>
      </span>
    {/each}
    <input
      {id}
      bind:value={input}
      {oninput}
      {onkeydown}
      onfocus={() => {
        open = true;
        void loadSuggestions(input);
      }}
      onblur={() => {
        // Delay so that a click on a suggestion is handled first.
        setTimeout(() => {
          open = false;
          if (input.trim()) addTag(input);
        }, 150);
      }}
      role="combobox"
      aria-expanded={open && visibleSuggestions.length > 0}
      aria-controls={listId}
      aria-autocomplete="list"
      aria-activedescendant={active >= 0 ? `${listId}-${active}` : undefined}
      autocomplete="off"
      {placeholder}
      class="font-inter text-text-brand placeholder:text-text-brand-muted min-w-24 flex-1 bg-transparent text-sm outline-none"
    />
  </div>
  {#if open && visibleSuggestions.length}
    <ul
      id={listId}
      role="listbox"
      class="border-brutal-border bg-card-surface absolute z-50 mt-1 max-h-48 w-full overflow-y-auto border-[3px] py-1 shadow-[4px_4px_0_var(--brutal-shadow)]"
    >
      {#each visibleSuggestions as suggestion, i (suggestion.tag)}
        <li
          id={`${listId}-${i}`}
          role="option"
          aria-selected={i === active}
          class="font-inter flex cursor-pointer items-center justify-between gap-2 px-3 py-1 text-sm {i ===
          active
            ? 'bg-muted-surface'
            : 'hover:bg-muted-surface'}"
          onmousedown={(event) => {
            event.preventDefault();
            addTag(suggestion.tag);
          }}
        >
          <span class="text-text-brand">#{suggestion.tag}</span>
          <span class="text-text-brand-muted text-xs">{suggestion.count}</span>
        </li>
      {/each}
    </ul>
  {/if}
</div>
