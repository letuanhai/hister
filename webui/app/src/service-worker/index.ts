// SPDX-License-Identifier: AGPL-3.0-or-later
import { self as workerSelf } from '$app/service-worker';
import { assets, immutable } from '$app/manifest';
import { resolve } from '$app/paths';
import { version } from '$app/env';

// Offline support for the web app. The app shell and assets are cached so the
// app opens without a network; saved pages live in IndexedDB (see
// lib/offline.ts). API responses are never cached, except the app
// configuration needed to start offline.
const sw = workerSelf;
const CACHE = `hister-${version}`;
// The root path, including the base path the server substitutes at runtime.
const SHELL: string = resolve('/');
const base = SHELL.replace(/\/$/, '');
const CONFIG = `${base}/api/config`;
const ASSETS = new Set(
  [...immutable, ...assets].map(({ path }) => `${base}/${path.replace(/^\//, '')}`),
);
// Fonts come in many unicode subsets; only the ones a page uses are cached, on first use.
const PRECACHE = [...ASSETS].filter((path) => !/\.woff2?$/.test(path));

sw.addEventListener('install', (event) => {
  event.waitUntil(
    (async () => {
      const cache = await caches.open(CACHE);
      await cache.addAll(PRECACHE);
      // The shell is best effort: it needs a session when access is protected.
      try {
        const shell = await fetch(SHELL, { credentials: 'include' });
        if (shell.ok) await cache.put(SHELL, shell);
      } catch {
        // Cached on the next navigation instead.
      }
      await sw.skipWaiting();
    })(),
  );
});

sw.addEventListener('activate', (event) => {
  event.waitUntil(
    (async () => {
      for (const key of await caches.keys()) {
        if (key !== CACHE && key.startsWith('hister-')) await caches.delete(key);
      }
      await sw.clients.claim();
    })(),
  );
});

async function networkFirst(request: Request, cacheKey: string): Promise<Response> {
  const cache = await caches.open(CACHE);
  try {
    const response = await fetch(request);
    if (response.ok && response.type === 'basic') await cache.put(cacheKey, response.clone());
    return response;
  } catch (err) {
    const cached = await cache.match(cacheKey);
    if (cached) return cached;
    throw err;
  }
}

sw.addEventListener('fetch', (event) => {
  const request = event.request;
  if (request.method !== 'GET') return;
  const url = new URL(request.url);
  if (url.origin !== sw.location.origin || !url.pathname.startsWith(base)) return;

  if (url.pathname === CONFIG) {
    event.respondWith(networkFirst(request, CONFIG));
    return;
  }
  // API responses, including archive pages opened in a tab, are never cached.
  if (url.pathname.startsWith(`${base}/api/`) || url.pathname === `${base}/search`) return;
  if (request.mode === 'navigate') {
    // Every app route renders from the same shell.
    event.respondWith(networkFirst(request, SHELL));
    return;
  }
  if (ASSETS.has(url.pathname)) {
    event.respondWith((async () => (await caches.match(url.pathname)) ?? fetch(request))());
  }
});
