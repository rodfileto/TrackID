import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import PageBreadcrumb from "../components/common/PageBreadCrumb";
import PageMeta from "../components/common/PageMeta";
import ComponentCard from "../components/common/ComponentCard";
import UserAvatar from "../components/common/UserAvatar";
import Badge from "../components/ui/badge/Badge";
import { useCurrentUser } from "../context/CurrentUserContext";
import { changeMyPassword, removeAvatar, updateMe, uploadAvatar } from "../services/auth";

const inputClass =
  "h-10 w-full rounded-lg border border-gray-300 bg-transparent px-3 text-sm text-gray-800 shadow-theme-xs focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90";
const readOnlyClass = "h-10 w-full rounded-lg border border-gray-200 bg-gray-50 px-3 text-sm leading-10 text-gray-500 dark:border-gray-800 dark:bg-white/[0.03] dark:text-gray-400";
const buttonClass = "inline-flex h-10 items-center rounded-lg bg-brand-500 px-4 text-sm font-medium text-white hover:bg-brand-600 disabled:opacity-50";

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="flex flex-col gap-1.5 text-sm">
      <span className="text-gray-700 dark:text-gray-400">{label}</span>
      {children}
    </label>
  );
}

function message(err: unknown) {
  return err instanceof Error ? err.message : String(err);
}

/** The signed-in account: the user's picture, name and job title, and password are theirs to
 * change; username, staff number and email are fixed (decisions name their decider by username),
 * and role and capabilities are an admin's (Users page). */
export default function UserProfiles() {
  const { t } = useTranslation();
  const { user, updateUser } = useCurrentUser();
  const [details, setDetails] = useState({ first_name: "", last_name: "", job_title: "" });
  const [savingDetails, setSavingDetails] = useState(false);
  const [picture, setPicture] = useState<"idle" | "uploading" | "removing">("idle");
  const [passwords, setPasswords] = useState({ current: "", next: "", confirm: "" });
  const [savingPassword, setSavingPassword] = useState(false);

  useEffect(() => {
    if (user) setDetails({ first_name: user.first_name, last_name: user.last_name, job_title: user.job_title });
  }, [user]);

  if (!user) {
    return <p className="text-sm text-gray-500 dark:text-gray-400">{t("profile.notSignedIn")}</p>;
  }

  const changed =
    details.first_name !== user.first_name || details.last_name !== user.last_name || details.job_title !== user.job_title;
  const capabilities = (
    [
      ["examines_fingerprint", user.examines_fingerprint],
      ["examines_facial", user.examines_facial],
      ["intelligence_analyst", user.intelligence_analyst],
      ["coordinator", user.coordinator],
    ] as const
  ).filter(([, has]) => has);

  async function saveDetails(e: React.FormEvent) {
    e.preventDefault();
    setSavingDetails(true);
    try {
      updateUser(await updateMe(details));
      toast.success(t("profile.saved"));
    } catch (err) {
      toast.error(message(err));
    } finally {
      setSavingDetails(false);
    }
  }

  async function changePicture(file: File) {
    setPicture("uploading");
    try {
      updateUser(await uploadAvatar(file));
    } catch (err) {
      toast.error(message(err));
    } finally {
      setPicture("idle");
    }
  }

  async function dropPicture() {
    setPicture("removing");
    try {
      updateUser(await removeAvatar());
    } catch (err) {
      toast.error(message(err));
    } finally {
      setPicture("idle");
    }
  }

  async function savePassword(e: React.FormEvent) {
    e.preventDefault();
    if (passwords.next !== passwords.confirm) {
      toast.error(t("profile.password.mismatch"));
      return;
    }
    setSavingPassword(true);
    try {
      await changeMyPassword(passwords.current, passwords.next);
      setPasswords({ current: "", next: "", confirm: "" });
      toast.success(t("profile.password.changed"));
    } catch (err) {
      toast.error(message(err));
    } finally {
      setSavingPassword(false);
    }
  }

  return (
    <>
      <PageMeta title={`${t("profile.title")} | TrackID`} description={t("profile.desc")} />
      <PageBreadcrumb pageTitle={t("profile.title")} />
      <div className="flex flex-col gap-6">
        <ComponentCard title={t("profile.title")}>
          <div className="flex flex-col items-start gap-5 sm:flex-row sm:items-center">
            <UserAvatar user={user} className="h-24 w-24 text-3xl" />
            <div className="flex flex-col gap-2">
              <div>
                <h3 className="text-lg font-semibold text-gray-800 dark:text-white/90">
                  {`${user.first_name} ${user.last_name}`.trim()}
                </h3>
                <p className="text-sm text-gray-500 dark:text-gray-400">{user.job_title}</p>
              </div>
              <div className="flex flex-wrap gap-3 text-sm">
                <label className={`cursor-pointer font-medium text-brand-500 hover:underline ${picture !== "idle" ? "pointer-events-none opacity-50" : ""}`}>
                  {picture === "uploading" ? t("profile.picture.uploading") : t("profile.picture.change")}
                  <input
                    type="file"
                    accept="image/*"
                    className="hidden"
                    onChange={(e) => {
                      const file = e.target.files?.[0];
                      e.target.value = "";
                      if (file) void changePicture(file);
                    }}
                  />
                </label>
                {user.avatar_updated_at && (
                  <button type="button" onClick={dropPicture} disabled={picture !== "idle"} className="text-gray-500 hover:underline disabled:opacity-50 dark:text-gray-400">
                    {picture === "removing" ? t("profile.picture.removing") : t("profile.picture.remove")}
                  </button>
                )}
              </div>
              <p className="text-xs text-gray-500 dark:text-gray-400">{t("profile.picture.hint")}</p>
            </div>
          </div>
        </ComponentCard>

        <ComponentCard title={t("profile.details")}>
          <form onSubmit={saveDetails} className="flex flex-col gap-4">
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label={t("profile.firstName")}>
                <input required maxLength={100} value={details.first_name} onChange={(e) => setDetails({ ...details, first_name: e.target.value })} className={inputClass} />
              </Field>
              <Field label={t("profile.lastName")}>
                <input required maxLength={100} value={details.last_name} onChange={(e) => setDetails({ ...details, last_name: e.target.value })} className={inputClass} />
              </Field>
              <Field label={t("profile.jobTitle")}>
                <input maxLength={100} value={details.job_title} onChange={(e) => setDetails({ ...details, job_title: e.target.value })} className={inputClass} />
              </Field>
              <Field label={t("profile.staffNumber")}>
                <span className={readOnlyClass}>{user.staff_number}</span>
              </Field>
              <Field label={t("profile.username")}>
                <span className={readOnlyClass}>{user.username}</span>
              </Field>
              <Field label={t("profile.email")}>
                <span className={readOnlyClass}>{user.email || "—"}</span>
              </Field>
              <Field label={t("profile.role")}>
                <span className={readOnlyClass}>{t(`profile.roles.${user.account_role}`)}</span>
              </Field>
            </div>
            <p className="text-xs text-gray-500 dark:text-gray-400">{t("profile.fixedHint")}</p>
            <div>
              <button type="submit" disabled={!changed || savingDetails} className={buttonClass}>
                {savingDetails ? t("common.saving") : t("common.save")}
              </button>
            </div>
          </form>
        </ComponentCard>

        <ComponentCard title={t("profile.capabilities")}>
          <div className="flex flex-wrap gap-2">
            {capabilities.length === 0 && <span className="text-sm text-gray-500 dark:text-gray-400">{t("profile.none")}</span>}
            {capabilities.map(([key]) => (
              <Badge key={key} size="sm" color="primary">
                {t(`profile.capability.${key}`)}
              </Badge>
            ))}
          </div>
          <p className="text-xs text-gray-500 dark:text-gray-400">{t("profile.managedByAdmin")}</p>
        </ComponentCard>

        <ComponentCard title={t("profile.password.title")}>
          <form onSubmit={savePassword} className="grid max-w-xl gap-4">
            <Field label={t("profile.password.current")}>
              <input type="password" required autoComplete="current-password" value={passwords.current} onChange={(e) => setPasswords({ ...passwords, current: e.target.value })} className={inputClass} />
            </Field>
            <Field label={t("profile.password.new")}>
              <input type="password" required minLength={8} autoComplete="new-password" value={passwords.next} onChange={(e) => setPasswords({ ...passwords, next: e.target.value })} className={inputClass} />
            </Field>
            <Field label={t("profile.password.confirm")}>
              <input type="password" required minLength={8} autoComplete="new-password" value={passwords.confirm} onChange={(e) => setPasswords({ ...passwords, confirm: e.target.value })} className={inputClass} />
            </Field>
            <div>
              <button type="submit" disabled={savingPassword} className={buttonClass}>
                {savingPassword ? t("common.saving") : t("profile.password.submit")}
              </button>
            </div>
          </form>
        </ComponentCard>
      </div>
    </>
  );
}
