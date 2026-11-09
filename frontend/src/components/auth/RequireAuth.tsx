import { useEffect, useState } from "react";
import { Navigate, Outlet, useLocation } from "react-router";
import { getToken, onSessionEnded, requestSessionFromOtherTabs, type SessionEndReason } from "../../services/auth";
import { CurrentUserProvider } from "../../context/CurrentUserContext";
import SessionExpiredDialog from "./SessionExpiredDialog";

export default function RequireAuth() {
  const location = useLocation();
  // A tab with no token may just be new: another tab can hand it the session (services/auth.ts).
  const [status, setStatus] = useState<"in" | "asking" | "out">(() => (getToken() ? "in" : "asking"));

  useEffect(() => {
    if (status !== "asking") return;
    let cancelled = false;
    void requestSessionFromOtherTabs().then((granted) => {
      if (!cancelled) setStatus(granted ? "in" : "out");
    });
    return () => {
      cancelled = true;
    };
  }, [status]);

  const [endReason, setEndReason] = useState<SessionEndReason>();

  useEffect(
    () =>
      onSessionEnded((reason) => {
        setEndReason(reason);
        setStatus("out");
      }),
    [],
  );

  if (status === "asking") return null;
  if (status === "out" || !getToken()) {
    // A deactivated account has nowhere to come back to.
    return <Navigate to="/signin" replace state={endReason ? { endReason } : { from: location }} />;
  }

  return (
    <CurrentUserProvider>
      <Outlet />
      <SessionExpiredDialog />
    </CurrentUserProvider>
  );
}
