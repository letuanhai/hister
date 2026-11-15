// SPDX-License-Identifier: AGPL-3.0-or-later

import { apiFetch, apiPath } from '#lib/api.js';

export interface DocumentArchive {
  id: number;
  created_at: string;
  url: string;
  user_id: number;
  title: string;
  engine: string;
  status: 'ready' | 'failed' | 'pending';
  error?: string;
  size: number;
  images: boolean;
  mobile: boolean;
  video_status?: string;
  video_file?: string;
}

export interface ArchiveOptions {
  engine?: string;
  keep_images?: boolean;
  mobile?: boolean;
  adblock?: boolean;
  video?: boolean;
}

export interface ArchiverStatus {
  enabled: boolean;
  engine: string;
  keep_images: boolean;
  mobile: boolean;
  adblock: boolean;
  video: boolean;
  has_cookies: boolean;
  has_profile: boolean;
}

export function archiveUrl(id: number): string {
  return apiPath(`/archive/${id}/`);
}

export async function fetchArchiverStatus(): Promise<ArchiverStatus> {
  const res = await apiFetch('/archiver');
  if (!res.ok) throw new Error('Archiver status request failed');
  return res.json();
}

export async function listArchives(url: string, documentId = ''): Promise<DocumentArchive[]> {
  const params = new URLSearchParams({ url });
  if (documentId) params.set('document_id', documentId);
  const res = await apiFetch(`/archives?${params}`);
  if (!res.ok) throw new Error('Archive list request failed');
  const data = (await res.json()) as { archives?: DocumentArchive[] };
  return data.archives ?? [];
}

export async function listAllArchives(limit = 1000): Promise<DocumentArchive[]> {
  const res = await apiFetch(`/archives?limit=${limit}`);
  if (!res.ok) throw new Error('Archive list request failed');
  const data = (await res.json()) as { archives?: DocumentArchive[] };
  return data.archives ?? [];
}

export async function createArchive(
  url: string,
  documentId: string,
  options: ArchiveOptions = {},
): Promise<DocumentArchive> {
  const res = await apiFetch('/archive', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ url, document_id: documentId, ...options }),
  });
  const contentType = res.headers.get('Content-Type') ?? '';
  if (contentType.includes('json')) {
    const archive = (await res.json()) as DocumentArchive;
    if (!res.ok) throw new Error(archive.error || 'Archiving failed');
    return archive;
  }
  const body = await res.text();
  throw new Error(body.trim() || 'Archiving failed');
}

export async function deleteArchive(id: number): Promise<void> {
  const res = await apiFetch('/archives/delete', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id }),
  });
  if (!res.ok) throw new Error('Deleting the archive failed');
}

export async function uploadArchiverCookies(content: string): Promise<number> {
  const res = await apiFetch('/archiver/cookies', {
    method: 'POST',
    headers: { 'Content-Type': 'text/plain' },
    body: content,
  });
  if (!res.ok) {
    const body = await res.text();
    throw new Error(body.trim() || 'Saving cookies failed');
  }
  if (res.status === 204) return 0;
  const data = (await res.json()) as { cookies: number };
  return data.cookies;
}

export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(0)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`;
  return `${(n / 1024 / 1024 / 1024).toFixed(1)} GB`;
}
