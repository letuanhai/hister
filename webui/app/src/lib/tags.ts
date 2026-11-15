// SPDX-License-Identifier: AGPL-3.0-or-later

import { apiFetch } from '#lib/api.js';

export interface TagSuggestion {
  tag: string;
  count: number;
}

// Mirrors document.NormalizeTag on the server so chips show the stored value.
export function normalizeTag(tag: string): string {
  return tag
    .trim()
    .toLowerCase()
    .replace(/^#/, '')
    .replace(/["\p{Cc}]/gu, '')
    .split(/[\s,]+/)
    .filter(Boolean)
    .join('-')
    .slice(0, 64);
}

export async function fetchTagSuggestions(prefix: string, limit = 10): Promise<TagSuggestion[]> {
  const params = new URLSearchParams({ limit: String(limit) });
  const normalized = normalizeTag(prefix);
  if (normalized) params.set('prefix', normalized);
  const res = await apiFetch(`/tags?${params}`);
  if (!res.ok) throw new Error('Tag suggestions request failed');
  return ((await res.json()) as TagSuggestion[]) ?? [];
}

export interface EditableDocument {
  url: string;
  title: string;
  label: string;
  tags: string[];
  summary: string;
  summary_source: string;
  notes: string;
  user_edited: string[];
}

export interface DocumentChanges {
  title?: string;
  reset_title?: boolean;
  label?: string;
  tags?: string[];
  summary?: string;
  notes?: string;
}

export async function editDocument(
  url: string,
  documentId: string,
  changes: DocumentChanges,
): Promise<EditableDocument> {
  const res = await apiFetch('/document/edit', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ url, document_id: documentId, changes }),
  });
  if (!res.ok) {
    const body = await res.text();
    throw new Error(body.trim() || 'Saving the document failed');
  }
  return res.json();
}

export async function summarizeDocument(
  url: string,
  documentId: string,
): Promise<EditableDocument> {
  const res = await apiFetch('/document/summarize', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ url, document_id: documentId }),
  });
  if (!res.ok) {
    const body = await res.text();
    throw new Error(body.trim() || 'Generating the summary failed');
  }
  return res.json();
}
