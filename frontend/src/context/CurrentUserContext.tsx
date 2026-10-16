import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { getCurrentUser, getStoredUser, type UserProfile } from "../services/auth";

interface CurrentUserValue {
  user: UserProfile | null;
  isAdmin: boolean;
}

const CurrentUserContext = createContext<CurrentUserValue>({ user: null, isAdmin: false });

/** Holds the signed-in user, starting from the stored profile and refreshed from /me once, so
 * a role or examiner change made by an admin shows without signing in again. */
export function CurrentUserProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<UserProfile | null>(getStoredUser());

  useEffect(() => {
    let cancelled = false;
    getCurrentUser()
      .then((fresh) => {
        if (!cancelled) setUser(fresh);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  const isAdmin = user?.account_role === "ADMIN" || user?.account_role === "SUPER_ADMIN";
  return (
    <CurrentUserContext.Provider value={{ user, isAdmin }}>{children}</CurrentUserContext.Provider>
  );
}

// eslint-disable-next-line react-refresh/only-export-components
export function useCurrentUser(): CurrentUserValue {
  return useContext(CurrentUserContext);
}
