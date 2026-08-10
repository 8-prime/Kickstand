/** Registers the shell cache so the app opens with no signal.
 *
 *  Only in a production build: in development Vite serves modules that change
 *  constantly, and a stale shell cache there is pure confusion. */
export function registerServiceWorker() {
  if (!import.meta.env.PROD) return;
  if (!("serviceWorker" in navigator)) return;

  window.addEventListener("load", () => {
    navigator.serviceWorker.register("/sw.js").catch((err) => {
      // Not fatal: the app works, it just will not survive a reload offline.
      console.warn("offline shell unavailable", err);
    });
  });
}
