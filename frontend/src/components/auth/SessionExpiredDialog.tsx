import { useEffect, useRef, useState, useSyncExternalStore, type FormEvent } from "react";
import { useNavigate } from "react-router";
import { useTranslation } from "react-i18next";
import Label from "../form/Label";
import Input from "../form/input/InputField";
import Button from "../ui/button/Button";
import { getStoredUser, isSessionExpired, signInAgain, signOut, subscribeToSessionExpiry } from "../../services/auth";

/** Asks for the password when the token runs out, over the page rather than instead of it, so
 * nothing typed there is lost and its waiting requests go on (services/auth.ts). */
export default function SessionExpiredDialog() {
  const expired = useSyncExternalStore(subscribeToSessionExpiry, isSessionExpired);
  return expired ? <PasswordPrompt /> : null;
}

function PasswordPrompt() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const dialog = useRef<HTMLDialogElement>(null);
  const [user] = useState(getStoredUser);
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  // A native modal dialog: it sits above any modal the page has open, and keeps focus away from
  // the page behind it.
  useEffect(() => {
    if (!dialog.current?.open) dialog.current?.showModal();
  }, []);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      await signInAgain(password);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("auth.signInError"));
      setSubmitting(false);
    }
  }

  function signInAsSomeoneElse() {
    signOut();
    navigate("/signin", { replace: true });
  }

  return (
    <dialog
      ref={dialog}
      aria-labelledby="session-expired-title"
      // It cannot be dismissed: the browser may close it on a repeated Esc whatever onCancel says.
      onCancel={(event) => event.preventDefault()}
      onClose={(event) => event.currentTarget.showModal()}
      // Keys typed here must not reach the page's shortcuts, or its own modal's Esc.
      onKeyDown={(event) => event.stopPropagation()}
      className="m-auto w-full max-w-md rounded-3xl bg-white p-6 backdrop:bg-gray-400/50 backdrop:backdrop-blur-[32px] dark:bg-gray-900 sm:p-8"
    >
      <h2 id="session-expired-title" className="mb-2 text-lg font-semibold text-gray-800 dark:text-white/90">
        {t("auth.sessionExpiredTitle")}
      </h2>
      <p className="mb-5 text-sm text-gray-500 dark:text-gray-400">
        {t("auth.sessionExpiredBody", {
          name: `${user?.first_name ?? ""} ${user?.last_name ?? ""}`.trim(),
          staffNumber: user?.staff_number ?? "",
        })}
      </p>
      <form onSubmit={handleSubmit} className="space-y-5">
        <input type="hidden" name="staff_number" autoComplete="username" value={user?.staff_number ?? ""} />
        <div>
          <Label htmlFor="session-password">{t("auth.password")}</Label>
          <Input
            id="session-password"
            name="password"
            type="password"
            placeholder={t("auth.passwordPlaceholder")}
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
        </div>
        {error && (
          <p className="text-sm text-error-500" role="alert">
            {error}
          </p>
        )}
        <Button className="w-full" size="sm" disabled={!password || submitting}>
          {submitting ? t("auth.signingIn") : t("auth.sessionExpiredContinue")}
        </Button>
        <button
          type="button"
          onClick={signInAsSomeoneElse}
          className="block w-full text-center text-sm text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
        >
          {t("auth.sessionExpiredOtherUser")}
        </button>
      </form>
    </dialog>
  );
}
