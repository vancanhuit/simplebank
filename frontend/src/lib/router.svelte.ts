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

function navigationTarget(to: string): URL {
  return new URL(to, window.location.origin);
}

/** Validate a history-state return target before using it for navigation. */
export function safeReturnPath(value: unknown): string | null {
  if (typeof value !== "string" || !value.startsWith("/") || value.startsWith("//")) {
    return null;
  }
  for (const character of value) {
    const codePoint = character.codePointAt(0);
    if (codePoint !== undefined && (codePoint <= 0x1f || codePoint === 0x7f)) {
      return null;
    }
  }

  const url = navigationTarget(value);
  const path = normalize(url.pathname);
  if (url.origin !== window.location.origin || path === "/login" || path === "/register") {
    return null;
  }
  return `${url.pathname}${url.search}${url.hash}`;
}

function historyState(): Record<string, unknown> {
  const state: unknown = window.history.state;
  return state !== null && typeof state === "object" ? { ...state } : {};
}

export const router = $state({
  path: normalize(window.location.pathname),
  state: historyState(),
});

/** Navigate to an in-app path, pushing a new history entry. An optional state
 *  object is stored on the history entry (e.g. a one-shot post-action notice). */
export function navigate(to: string, state: Record<string, unknown> = {}): void {
  const url = navigationTarget(to);
  window.history.pushState(state, "", `${url.pathname}${url.search}${url.hash}`);
  router.path = normalize(url.pathname);
  router.state = state;
}

/** Navigate to an in-app path by replacing the current history entry. */
export function replaceNavigation(to: string, state: Record<string, unknown> = {}): void {
  const url = navigationTarget(to);
  window.history.replaceState(state, "", `${url.pathname}${url.search}${url.hash}`);
  router.path = normalize(url.pathname);
  router.state = state;
}

/** Replace state on the current entry and publish the same reactive snapshot. */
export function replaceNavigationState(state: Record<string, unknown>): void {
  window.history.replaceState(state, "", window.location.href);
  router.state = state;
}

window.addEventListener("popstate", () => {
  router.path = normalize(window.location.pathname);
  router.state = historyState();
});
