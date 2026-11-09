import { createContext, useContext, useMemo, useState, type ReactNode } from "react";

const PageMemoryContext = createContext<Map<string, unknown>>(new Map());

/** Keeps what a page cannot put in its URL (a photo and the matches it found) while the user visits
 * another page and comes back. It is held in memory only, under the signed-in layout: a reload or
 * signing out forgets it. */
export function PageMemoryProvider({ children }: { children: ReactNode }) {
  const [store] = useState(() => new Map<string, unknown>());
  return <PageMemoryContext.Provider value={store}>{children}</PageMemoryContext.Provider>;
}

// eslint-disable-next-line react-refresh/only-export-components
export function usePageMemory<T>(key: string) {
  const store = useContext(PageMemoryContext);
  return useMemo(
    () => ({
      read: () => store.get(key) as T | undefined,
      write: (value: T) => {
        store.set(key, value);
      },
    }),
    [store, key],
  );
}
