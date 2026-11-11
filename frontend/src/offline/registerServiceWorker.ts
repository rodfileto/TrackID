// Registers the service worker that keeps the application's files (vite.config.ts, offlineShell),
// in a build only: the dev server has none. A browser without service workers just works online.
export function registerServiceWorker(): void {
  if (!import.meta.env.PROD || !("serviceWorker" in navigator)) return;
  window.addEventListener("load", () => {
    navigator.serviceWorker.register("/sw.js").catch(() => {
      // Not a secure context, or blocked: the application still works with a network.
    });
  });
}
