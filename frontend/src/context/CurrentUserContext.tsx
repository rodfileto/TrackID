import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { useLocation } from "react-router";
import { getCurrentUser, getStoredUser, storeUser, type UserProfile } from "../services/auth";
import { extension } from "../extensions";
import type { ExtensionNeed, PreviewProfile } from "../extensions/types";
import { corePreviewProfiles } from "./previewProfiles";

/** The profiles an admin can preview `/` and the menu as: core's, then the extension's. */
// eslint-disable-next-line react-refresh/only-export-components
export const previewProfiles: PreviewProfile[] = [...corePreviewProfiles, ...(extension.previewProfiles ?? [])];

/** What a page or menu item is for: one of core's capabilities (RequireCapability), or a grant of the extension.
 * "profile" is any capability: casework and its tools, which an account with none is not offered. */
export type Need = ExtensionNeed | "scene" | "profile";

/** What the signed-in user may do (docs/access-profiles-plan.md). The server decides; these only
 * keep the screens from offering what it would refuse. isAdmin is account management only. While
 * an admin previews a profile, all of it answers as that profile. */
interface CurrentUserValue {
  /** The signed-in user; while previewing, with the previewed profile's capabilities and no admin role. */
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
  /** The signed-in user is an admin, who may preview `/` and the menu as another profile. */
  canPreview: boolean;
  /** The profile being previewed; null when the screens are the user's own. */
  preview: PreviewProfile | null;
  /** Starts previewing a profile, or stops with null. Leaving `/` stops it too. */
  setPreview: (profile: PreviewProfile | null) => void;
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
  canPreview: false,
  preview: null,
  setPreview: () => {},
});

/** Holds the signed-in user, starting from the stored profile and refreshed from /me once, so
 * a role or capability change made by an admin shows without signing in again; and the
 * extension's grants, read once too. */
export function CurrentUserProvider({ children }: { children: ReactNode }) {
  const [realUser, setUser] = useState<UserProfile | null>(getStoredUser());
  const [realGrants, setGrants] = useState<string[] | null>(extension.grants ? null : []);

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

  const canPreview = !!realUser?.is_active && (realUser.account_role === "ADMIN" || realUser.account_role === "SUPER_ADMIN");
  // A preview shows `/` and the menu only: anywhere else the user's requests would answer for the admin, not the profile.
  const { pathname } = useLocation();
  const [preview, setPreview] = useState<PreviewProfile | null>(null);
  const previewing = canPreview && pathname === "/" ? preview : null;
  useEffect(() => {
    if (pathname !== "/") setPreview(null);
  }, [pathname]);
  const user = useMemo<UserProfile | null>(
    () =>
      previewing && realUser
        ? {
            ...realUser,
            account_role: "USER",
            examines_fingerprint: !!previewing.access?.examines_fingerprint,
            examines_facial: !!previewing.access?.examines_facial,
            intelligence_analyst: !!previewing.access?.intelligence_analyst,
            coordinator: !!previewing.access?.coordinator,
          }
        : realUser,
    [previewing, realUser],
  );
  const grants = useMemo(() => (previewing ? (previewing.grants ?? []) : realGrants), [previewing, realGrants]);

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
        case "profile":
          return isExaminer || isIntelligence || isCoordinator;
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
    canPreview,
    preview: previewing,
    setPreview,
  };
  return <CurrentUserContext.Provider value={value}>{children}</CurrentUserContext.Provider>;
}

// eslint-disable-next-line react-refresh/only-export-components
export function useCurrentUser(): CurrentUserValue {
  return useContext(CurrentUserContext);
}
