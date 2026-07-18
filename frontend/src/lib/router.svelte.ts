/**
 * A minimal history-based router built on Svelte 5 runes. Keeping routing in a
 * tiny module (instead of a dependency) suits an embedded SPA: the Go server
 * serves index.html for any unknown path, so deep links and refreshes resolve
 * to the app shell and this module reads the location on load.
 */

function normalize(path: string): string {
  if (path.length > 1 && path.endsWith("/")) {
    return path.slice(0, -1);
  }
  return path;
}

export const router = $state({
  path: normalize(window.location.pathname),
});

/** Navigate to an in-app path, pushing a new history entry. An optional state
 *  object is stored on the history entry (e.g. a one-shot post-action notice). */
export function navigate(to: string, state: Record<string, unknown> = {}): void {
  const path = normalize(to);
  window.history.pushState(state, "", path);
  router.path = path;
}

window.addEventListener("popstate", () => {
  router.path = normalize(window.location.pathname);
});
