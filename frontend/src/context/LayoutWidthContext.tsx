import { createContext, useContext, useEffect, useState, type ReactNode } from "react";

interface LayoutWidth {
  wide: boolean;
  /** Asks for the wide layout until the returned function is called. */
  request: () => () => void;
}

const LayoutWidthContext = createContext<LayoutWidth>({ wide: false, request: () => () => {} });

/** Lets a page widen the signed-in layout's content past 2xl (1536px) up to 3xl (2000px), for pages that place
 * more side by side on large screens. Every other page keeps the narrower layout. */
export function LayoutWidthProvider({ children }: { children: ReactNode }) {
  // A count, not a flag: a page and a panel inside it may both ask, and leaving one mustn't undo the other.
  const [requests, setRequests] = useState(0);
  const [value] = useState(() => ({
    request: () => {
      setRequests((n) => n + 1);
      let released = false;
      return () => {
        if (released) return;
        released = true;
        setRequests((n) => n - 1);
      };
    },
  }));
  return <LayoutWidthContext.Provider value={{ wide: requests > 0, request: value.request }}>{children}</LayoutWidthContext.Provider>;
}

/** Whether the layout is wide now, for the layout itself. */
// eslint-disable-next-line react-refresh/only-export-components
export function useLayoutWide(): boolean {
  return useContext(LayoutWidthContext).wide;
}

/** Widens the layout while the calling component is shown. */
// eslint-disable-next-line react-refresh/only-export-components
export function useWideLayout(): void {
  const { request } = useContext(LayoutWidthContext);
  useEffect(() => request(), [request]);
}
