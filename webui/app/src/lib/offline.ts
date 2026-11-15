// SPDX-License-Identifier: AGPL-3.0-or-later

import { apiFetch } from '#lib/api.js';
import { archiveUrl, createArchive, listArchives } from '#lib/archives.js';

// Pages saved for offline reading are kept in IndexedDB in this browser.

export interface OfflineEntry {
  key: string;
  url: string;
  documentId: string;
  title: string;
  summary: string;
  tags: string[];
  savedAt: number;
  /** A complete, script free HTML page shown in a sandboxed frame. */
  html: string;
  source: 'archive' | 'preview';
}

const DB_NAME = 'hister-offline';
const STORE = 'entries';

function openDB(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, 1);
    req.onupgradeneeded = () => {
      const db = req.result;
      if (!db.objectStoreNames.contains(STORE)) {
        db.createObjectStore(STORE, { keyPath: 'key' }).createIndex('savedAt', 'savedAt');
      }
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

async function withStore<T>(
  mode: IDBTransactionMode,
  fn: (store: IDBObjectStore) => IDBRequest<T>,
): Promise<T> {
  const db = await openDB();
  try {
    return await new Promise<T>((resolve, reject) => {
      const tx = db.transaction(STORE, mode);
      const req = fn(tx.objectStore(STORE));
      tx.oncomplete = () => resolve(req.result);
      tx.onerror = () => reject(tx.error);
      tx.onabort = () => reject(tx.error);
    });
  } finally {
    db.close();
  }
}

export function offlineKey(url: string, documentId = ''): string {
  return documentId || url;
}

export async function getOfflineEntry(key: string): Promise<OfflineEntry | undefined> {
  return withStore<OfflineEntry | undefined>('readonly', (s) => s.get(key));
}

export async function listOfflineEntries(): Promise<OfflineEntry[]> {
  const entries = await withStore<OfflineEntry[]>('readonly', (s) => s.getAll());
  return entries.sort((a, b) => b.savedAt - a.savedAt);
}

export async function putOfflineEntry(entry: OfflineEntry): Promise<void> {
  await withStore('readwrite', (s) => s.put(entry));
}

export async function removeOfflineEntry(key: string): Promise<void> {
  await withStore('readwrite', (s) => s.delete(key));
}

function escapeHTML(s: string): string {
  return s.replace(
    /[&<>"']/g,
    (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]!,
  );
}

/** Wraps sanitized preview content in a standalone reader page. */
export function previewPage(title: string, url: string, content: string): string {
  return `<!DOCTYPE html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>${escapeHTML(title)}</title>
<style>:root{color-scheme:light dark}body{margin:0 auto;max-width:42rem;padding:2rem 1rem;font:1.125rem/1.7 Georgia,serif}img{max-width:100%;height:auto}pre{overflow-x:auto}</style>
</head><body><h1>${escapeHTML(title)}</h1><p><a href="${escapeHTML(url)}" rel="noopener noreferrer">${escapeHTML(url)}</a></p>${content}</body></html>`;
}

interface SaveInput {
  url: string;
  documentId: string;
  title: string;
  summary: string;
  tags: string[];
  archivesEnabled: boolean;
}

/**
 * Saves a page for offline reading. The newest reader archive is used, or a
 * new one is created when archives are enabled; otherwise the stored preview
 * content is saved.
 */
export async function saveForOffline(input: SaveInput): Promise<OfflineEntry> {
  let html = '';
  let source: OfflineEntry['source'] = 'preview';
  if (input.archivesEnabled) {
    try {
      let archive = (await listArchives(input.url, input.documentId)).find(
        (a) => a.status === 'ready',
      );
      archive ??= await createArchive(input.url, input.documentId);
      if (archive.status === 'ready') {
        const res = await fetch(archiveUrl(archive.id), { credentials: 'include' });
        if (res.ok) {
          html = await res.text();
          source = 'archive';
        }
      }
    } catch {
      // Fall back to the preview content.
    }
  }
  if (!html) {
    const params = new URLSearchParams({ url: input.url });
    if (input.documentId) params.set('document_id', input.documentId);
    const res = await apiFetch(`/preview?${params}`);
    if (!res.ok) throw new Error('Could not load the page content');
    const data = (await res.json()) as { title?: string; content?: string };
    html = previewPage(data.title || input.title, input.url, data.content ?? '');
  }
  const entry: OfflineEntry = {
    key: offlineKey(input.url, input.documentId),
    url: input.url,
    documentId: input.documentId,
    title: input.title || input.url,
    summary: input.summary,
    // Copy reactive arrays: IndexedDB cannot clone proxies.
    tags: [...input.tags],
    savedAt: Date.now(),
    html,
    source,
  };
  await putOfflineEntry(entry);
  return entry;
}
