<!-- SPDX-License-Identifier: AGPL-3.0-or-later -->
<script lang="ts">
  import { fetchArchiverStatus, uploadArchiverCookies } from '#lib/archives.js';
  import { onMount } from 'svelte';

  let hasCookies = $state(false);
  let content = $state('');
  let message = $state('');
  let isError = $state(false);
  let saving = $state(false);

  onMount(async () => {
    try {
      hasCookies = (await fetchArchiverStatus()).has_cookies;
    } catch {
      // The status is informational; uploads still work.
    }
  });

  async function readFile(event: Event) {
    const file = (event.currentTarget as HTMLInputElement).files?.[0];
    if (file) content = await file.text();
  }

  async function save(remove = false) {
    saving = true;
    message = '';
    try {
      const n = await uploadArchiverCookies(remove ? '' : content);
      hasCookies = !remove;
      content = '';
      message = remove ? 'Cookies removed.' : `Saved ${n} cookies.`;
      isError = false;
    } catch (e) {
      message = e instanceof Error ? e.message : 'Saving cookies failed';
      isError = true;
    } finally {
      saving = false;
    }
  }
</script>

<section
  class="border-border-brand-muted space-y-2 border p-4"
  aria-labelledby="archiver-cookies-heading"
>
  <h3 id="archiver-cookies-heading" class="font-outfit text-text-brand text-sm font-bold">
    Your archive cookies
  </h3>
  <p class="text-text-brand-muted text-sm">
    Archive pages behind your logins by uploading cookies exported from your browser, as a Netscape
    <code>cookies.txt</code> or JSON file. They are stored on the server for your account only and
    sent with archive requests. Alternatively, sign in once in a persistent browser profile with
    <code>hister archive-login</code>.
    {hasCookies ? 'Cookies are stored.' : 'No cookies are stored.'}
  </p>
  <input
    type="file"
    accept=".txt,.json,text/plain,application/json"
    aria-label="Cookies file"
    onchange={readFile}
    class="text-sm"
  />
  <textarea
    bind:value={content}
    rows="4"
    aria-label="Cookies"
    placeholder="Or paste the cookies file here"
    class="bg-page-bg border-brutal-border font-fira text-text-brand w-full rounded-none border-[3px] p-2 text-xs"
  ></textarea>
  <div class="flex flex-wrap items-center gap-3">
    <button
      type="button"
      disabled={saving || !content.trim()}
      onclick={() => save()}
      class="bg-hister-teal font-space border-brutal-border cursor-pointer border-[2px] px-3 py-1 text-xs font-bold text-white uppercase disabled:opacity-50"
    >
      Save cookies
    </button>
    {#if hasCookies}
      <button
        type="button"
        disabled={saving}
        onclick={() => save(true)}
        class="text-text-brand-muted cursor-pointer text-xs hover:underline">Remove cookies</button
      >
    {/if}
    {#if message}
      <span role="status" class="text-xs {isError ? 'text-red-500' : 'text-text-brand-muted'}"
        >{message}</span
      >
    {/if}
  </div>
</section>
