// TrackID's service worker (vite.config.ts writes it into each build as /sw.js, with the build's
// files). It keeps the application's own files so a page opens with no signal, and nothing else:
// requests to the API, and anything not a GET, always go to the network.
const VERSION = "__VERSION__";
const FILES = __FILES__;
const CACHE = "trackid-shell-" + VERSION;

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches
      .open(CACHE)
      .then((cache) => cache.addAll(["/", ...FILES].map((url) => new Request(url, { cache: "reload" }))))
      .then(() => self.skipWaiting()),
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((names) => Promise.all(names.filter((n) => n.startsWith("trackid-shell-") && n !== CACHE).map((n) => caches.delete(n))))
      .then(() => self.clients.claim()),
  );
});

// A page: the network's when it answers in time, since a new build has a new index.html; with no
// network, or a slow one, the kept one.
function page(request) {
  const kept = () => caches.match("/", { cacheName: CACHE });
  return new Promise((resolve) => {
    const timer = setTimeout(() => kept().then((r) => r && resolve(r)), 4000);
    fetch(request)
      .then((response) => {
        clearTimeout(timer);
        resolve(response);
      })
      .catch(() => {
        clearTimeout(timer);
        kept().then((r) => resolve(r || Response.error()));
      });
  });
}

self.addEventListener("fetch", (event) => {
  const request = event.request;
  if (request.method !== "GET") return;
  const url = new URL(request.url);
  if (url.origin !== self.location.origin) return;
  if (url.pathname.startsWith("/api/") || url.pathname.startsWith("/wordcraft/")) return;
  if (request.mode === "navigate") {
    event.respondWith(page(request));
    return;
  }
  // The build's files are named by their content, so a kept one is the right one.
  event.respondWith(caches.match(request, { cacheName: CACHE }).then((kept) => kept || fetch(request)));
});
