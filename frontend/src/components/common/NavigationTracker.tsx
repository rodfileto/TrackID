import { useEffect, useLayoutEffect, useRef } from "react";
import { useLocation, useNavigationType } from "react-router";
import { cameFrom, recordFrom, recordScroll, savedScroll, saveTrail } from "../../utils/navigationTrail";

/** How long a restored page may take to grow tall enough (its list is still loading). */
const restoreTimeoutMs = 3000;
/** Keeps holding the position this long even when it is reachable, while the page settles. */
const settleMs = 300;

/** Records where each page was reached from (BackLink) and restores the scroll position on back,
 * forward and reload; any other navigation to a new page starts at the top. BrowserRouter has no
 * <ScrollRestoration>, and the browser's own cannot wait for a list fetched after the page mounts. */
export function NavigationTracker() {
  const location = useLocation();
  const navigationType = useNavigationType();
  const current = useRef<{ key: string; path: string; pathname: string } | null>(null);

  // A layout effect so the entry changes before the browser clamps the scroll to the new page's
  // height: that scroll event must not overwrite what was saved for the page being left.
  useLayoutEffect(() => {
    const previous = current.current;
    const path = `${location.pathname}${location.search}`;
    if (previous?.key === location.key) return;
    current.current = { key: location.key, path, pathname: location.pathname };

    if (navigationType === "PUSH" && previous) recordFrom(location.key, previous.path);
    else if (navigationType === "REPLACE" && previous) recordFrom(location.key, cameFrom(previous.key));
    saveTrail();

    if (navigationType === "POP") {
      restoreScroll(savedScroll(location.key) ?? 0);
    } else if (previous && previous.pathname !== location.pathname) {
      window.scrollTo({ top: 0, left: 0, behavior: "instant" });
    }
  }, [location, navigationType]);

  useEffect(() => {
    window.history.scrollRestoration = "manual";
    const onScroll = () => {
      if (current.current) recordScroll(current.current.key, window.scrollY);
    };
    window.addEventListener("scroll", onScroll, { passive: true });
    window.addEventListener("pagehide", saveTrail);
    return () => {
      window.removeEventListener("scroll", onScroll);
      window.removeEventListener("pagehide", saveTrail);
    };
  }, []);

  return null;
}

let cancelRestore: (() => void) | null = null;

/** Scrolls to y, waiting while the page is too short to reach it. Moving the page by hand wins. */
function restoreScroll(y: number) {
  cancelRestore?.();
  const started = performance.now();
  let frame = 0;
  const events = ["wheel", "touchstart", "keydown", "mousedown"] as const;
  const stop = () => {
    cancelAnimationFrame(frame);
    events.forEach((event) => window.removeEventListener(event, stop));
    cancelRestore = null;
  };
  const attempt = () => {
    const reachable = document.documentElement.scrollHeight - window.innerHeight >= y;
    window.scrollTo({ top: y, left: 0, behavior: "instant" });
    if (reachable && performance.now() - started > settleMs) return stop();
    if (performance.now() - started > restoreTimeoutMs) return stop();
    frame = requestAnimationFrame(attempt);
  };
  events.forEach((event) => window.addEventListener(event, stop, { passive: true }));
  cancelRestore = stop;
  frame = requestAnimationFrame(attempt);
}
