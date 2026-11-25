import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";
import { getCurrentUser, getStoredUser, storeUser, type UserProfile } from "../services/auth";
import { extension } from "../extensions";
import type { ExtensionNeed } from "../extensions/types";

/** What a page or menu item is for: one of core's capabilities (RequireCapability), or a grant of the extension. */
export type Need = ExtensionNeed | "scene";

/** What the signed-in user may do (docs/access-profiles-plan.md). The server decides; these only
 * keep the screens from offering what it would refuse. isAdmin is account management only. */
interface CurrentUserValue {
  user: UserProfile | null;
  isAdmin: boolean;
  isExaminer: boolean;
  isIntelligence: boolean;
  isCoordinator: boolean;
  /** Examines fingerprints: registers the scenes they examine on call. */
  isFingerprintExaminer: boolean;
  /** The names the extension grants the user (Extension.grants); null until read. */
  grants: string[] | null;
  /** Whether the user has what need names; false for a grant while the grants are still being read. */
  can: (need: Need) => boolean;
  /** Replaces the signed-in user after they edit their profile, stored and shown everywhere. */
  updateUser: (user: UserProfile) => void;
}

const CurrentUserContext = createContext<CurrentUserValue>({
  user: null,
  isAdmin: false,
  isExaminer: false,
  isFingerprintExaminer: false,
  isIntelligence: false,
  isCoordinator: false,
  grants: null,
  can: () => false,
  updateUser: () => {},
});

/** Holds the signed-in user, starting from the stored profile and refreshed from /me once, so
 * a role or capability change made by an admin shows without signing in again; and the
 * extension's grants, read once too. */
export function CurrentUserProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<UserProfile | null>(getStoredUser());
  const [grants, setGrants] = useState<string[] | null>(extension.grants ? null : []);

  useEffect(() => {
    if (!extension.grants) return;
    let cancelled = false;
    extension
      .grants()
      .catch(() => [] as string[])
      .then((names) => {
        if (!cancelled) setGrants(names);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    getCurrentUser()
      .then((fresh) => {
        if (!cancelled) {
          setUser(fresh);
          storeUser(fresh);
        }
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  const active = !!user?.is_active;
  const isAdmin = user?.account_role === "ADMIN" || user?.account_role === "SUPER_ADMIN";
  const isExaminer = active && (!!user?.examines_facial || !!user?.examines_fingerprint);
  const isIntelligence = active && !!user?.intelligence_analyst;
  const isCoordinator = active && !!user?.coordinator;
  const isFingerprintExaminer = active && !!user?.examines_fingerprint;
  // Stable while nothing it reads changes, so a list filtered with it can be memoized on it.
  const can = useCallback(
    (need: Need): boolean => {
      if (typeof need === "object") return active && !!grants?.includes(need.grant);
      switch (need) {
        case "admin":
          return isAdmin;
        case "coordinator":
          return isCoordinator;
        case "intelligence":
          return isIntelligence;
        case "persons":
          return isExaminer || isIntelligence;
        case "scene":
          return isCoordinator || isFingerprintExaminer;
      }
    },
    [active, grants, isAdmin, isCoordinator, isIntelligence, isExaminer, isFingerprintExaminer],
  );
  const value: CurrentUserValue = {
    user,
    isAdmin,
    isExaminer,
    isIntelligence,
    isCoordinator,
    isFingerprintExaminer,
    grants,
    can,
    updateUser: (next) => {
      storeUser(next);
      setUser(next);
    },
  };
  return <CurrentUserContext.Provider value={value}>{children}</CurrentUserContext.Provider>;
}

// eslint-disable-next-line react-refresh/only-export-components
export function useCurrentUser(): CurrentUserValue {
  return useContext(CurrentUserContext);
}
