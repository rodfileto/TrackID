import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import ComponentCard from "./ComponentCard";
import {
  clearTeamsWebhook,
  getTeamsWebhook,
  setTeamsWebhook,
  testTeamsWebhook,
  type TeamsWebhookStatus,
} from "../../services/teams";

const inputClass =
  "h-10 w-full rounded-lg border border-gray-300 bg-transparent px-3 text-sm text-gray-800 shadow-theme-xs focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90";
const buttonClass =
  "inline-flex h-10 items-center rounded-lg bg-brand-500 px-4 text-sm font-medium text-white hover:bg-brand-600 disabled:opacity-50";
const secondaryClass =
  "inline-flex h-10 items-center rounded-lg border border-gray-300 px-4 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-white/[0.03]";

function message(err: unknown) {
  return err instanceof Error ? err.message : String(err);
}

/** The user's Microsoft Teams webhook: with one, each notification also arrives in Teams, saying only that there is
 * something to see. The URL is a credential, so it is never shown again once saved: it can only be replaced. */
export default function TeamsWebhookCard() {
  const { t, i18n } = useTranslation();
  const [status, setStatus] = useState<TeamsWebhookStatus | null>(null);
  const [url, setUrl] = useState("");
  const [busy, setBusy] = useState<"" | "saving" | "removing" | "testing">("");
  const [loadFailed, setLoadFailed] = useState(false);

  useEffect(() => {
    getTeamsWebhook()
      .then(setStatus)
      .catch(() => setLoadFailed(true));
  }, []);

  if (loadFailed) {
    return (
      <ComponentCard title={t("teams.title")} desc={t("teams.desc")}>
        <p className="text-sm text-error-500">{t("teams.loadError")}</p>
      </ComponentCard>
    );
  }
  if (!status) return null;

  const run = async (kind: typeof busy, action: () => Promise<void>) => {
    setBusy(kind);
    try {
      await action();
    } catch (err) {
      toast.error(message(err));
      getTeamsWebhook().then(setStatus).catch(() => {});
    } finally {
      setBusy("");
    }
  };

  const save = (e: React.FormEvent) => {
    e.preventDefault();
    void run("saving", async () => {
      setStatus(await setTeamsWebhook(url.trim()));
      setUrl("");
      toast.success(t("teams.saved"));
    });
  };

  const remove = () =>
    run("removing", async () => {
      await clearTeamsWebhook();
      setStatus({ ...status, configured: false, setAt: undefined, lastError: undefined, failedAt: undefined });
      toast.success(t("teams.removed"));
    });

  const test = () =>
    run("testing", async () => {
      await testTeamsWebhook();
      setStatus(await getTeamsWebhook());
      toast.success(t("teams.testSent"));
    });

  const when = (iso?: string) => (iso ? new Date(iso).toLocaleString(i18n.language) : "");

  return (
    <ComponentCard title={t("teams.title")} desc={t("teams.desc")}>
      {!status.available ? (
        <p className="text-sm text-gray-500 dark:text-gray-400">{t("teams.unavailable")}</p>
      ) : (
        <div className="flex max-w-xl flex-col gap-4">
          <p className="text-sm text-gray-700 dark:text-gray-300">
            {status.configured ? t("teams.configured", { when: when(status.setAt) }) : t("teams.notConfigured")}
          </p>
          {status.lastError && (
            <p className="text-sm text-error-500">
              {t("teams.lastError", { when: when(status.failedAt), error: status.lastError })}
            </p>
          )}
          <form onSubmit={save} className="flex flex-col gap-3">
            <label className="flex flex-col gap-1.5 text-sm">
              <span className="text-gray-700 dark:text-gray-400">
                {status.configured ? t("teams.replace") : t("teams.url")}
              </span>
              <input
                type="url"
                required
                autoComplete="off"
                spellCheck={false}
                placeholder="https://…"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                className={inputClass}
              />
            </label>
            <p className="text-xs text-gray-500 dark:text-gray-400">{t("teams.hint")}</p>
            <div className="flex flex-wrap gap-3">
              <button type="submit" disabled={!url.trim() || busy !== ""} className={buttonClass}>
                {busy === "saving" ? t("common.saving") : t("common.save")}
              </button>
              {status.configured && (
                <>
                  <button type="button" onClick={test} disabled={busy !== ""} className={secondaryClass}>
                    {busy === "testing" ? t("teams.testing") : t("teams.test")}
                  </button>
                  <button type="button" onClick={remove} disabled={busy !== ""} className={secondaryClass}>
                    {busy === "removing" ? t("teams.removing") : t("teams.remove")}
                  </button>
                </>
              )}
            </div>
          </form>
        </div>
      )}
    </ComponentCard>
  );
}
