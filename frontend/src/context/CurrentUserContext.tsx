import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { getCurrentUser, getStoredUser, type UserProfile } from "../services/auth";

/** What the signed-in user may do (docs/access-profiles-plan.md). The server decides; these only
 * keep the screens from offering what it would refuse. isAdmin is account management only. */
interface CurrentUserValue {
  user: UserProfile | null;
  isAdmin: boolean;
  isExaminer: boolean;
  isIntelligence: boolean;
  isCoordinator: boolean;
}

const CurrentUserContext = createContext<CurrentUserValue>({
  user: null,
  isAdmin: false,
  isExaminer: false,
  isIntelligence: false,
  isCoordinator: false,
});

/** Holds the signed-in user, starting from the stored profile and refreshed from /me once, so
 * a role or capability change made by an admin shows without signing in again. */
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

  const active = !!user?.is_active;
  const value: CurrentUserValue = {
    user,
    isAdmin: user?.account_role === "ADMIN" || user?.account_role === "SUPER_ADMIN",
    isExaminer: active && (!!user?.examines_facial || !!user?.examines_fingerprint),
    isIntelligence: active && !!user?.intelligence_analyst,
    isCoordinator: active && !!user?.coordinator,
  };
  return <CurrentUserContext.Provider value={value}>{children}</CurrentUserContext.Provider>;
}

// eslint-disable-next-line react-refresh/only-export-components
export function useCurrentUser(): CurrentUserValue {
  return useContext(CurrentUserContext);
}
