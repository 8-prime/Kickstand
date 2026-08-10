/* Service worker: keeps the app itself available with no signal.
 *
 * The trip data already survives offline in IndexedDB, but that is no help if
 * the browser cannot load the HTML and JavaScript to read it with. Closing
 * the tab at a campsite in the Verdon and opening it again is exactly the
 * case this covers.
 *
 * Scope is deliberately narrow: the app shell only. API responses are not
 * cached here — the trip store owns that, and it knows about revisions and
 * pending writes in a way a URL cache cannot.
 */

const SHELL = "bike-trip-shell-v1";

self.addEventListener("install", (event) => {
  event.waitUntil(precache());
  // A new build should take over rather than wait for every tab to close.
  self.skipWaiting();
});

/* Caching index.html alone is not enough: without the JavaScript it names,
 * an offline reload renders an empty page. The asset filenames are
 * content-hashed and therefore unknown at write time, so they are read out of
 * the served HTML instead of being hardcoded here. */
async function precache() {
  const cache = await caches.open(SHELL);
  const res = await fetch("/index.html", { cache: "reload" });
  const html = await res.text();
  await cache.put("/index.html", new Response(html, { headers: res.headers }));

  const assets = [...html.matchAll(/(?:src|href)="(\/assets\/[^"]+)"/g)].map((m) => m[1]);
  await Promise.all(
    assets.map((url) =>
      cache.add(url).catch(() => {
        /* one missing asset must not fail the whole install */
      }),
    ),
  );
}

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) =>
        Promise.all(keys.filter((k) => k !== SHELL).map((k) => caches.delete(k))),
      )
      .then(() => self.clients.claim()),
  );
});

self.addEventListener("fetch", (event) => {
  const { request } = event;
  if (request.method !== "GET") return;

  const url = new URL(request.url);
  if (url.origin !== self.location.origin) return;

  // The API is the store's business. Never serve a stale trip from here —
  // the app would have no way to tell it was looking at an old revision.
  if (url.pathname.startsWith("/api/")) return;

  // Navigations: network first so a deploy is picked up, falling back to the
  // cached shell when there is nothing to reach.
  if (request.mode === "navigate") {
    event.respondWith(
      fetch(request)
        .then((res) => {
          const copy = res.clone();
          caches.open(SHELL).then((c) => c.put("/index.html", copy));
          return res;
        })
        .catch(() =>
          caches.match("/index.html").then((hit) => hit ?? Response.error()),
        ),
    );
    return;
  }

  // Build assets are content-hashed, so a cache hit is always correct.
  if (url.pathname.startsWith("/assets/")) {
    event.respondWith(
      caches.match(request).then(
        (hit) =>
          hit ??
          fetch(request).then((res) => {
            if (res.ok) {
              const copy = res.clone();
              caches.open(SHELL).then((c) => c.put(request, copy));
            }
            return res;
          }),
      ),
    );
  }
});
