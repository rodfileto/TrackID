import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import PageMeta from "../components/common/PageMeta";
import PageBreadcrumb from "../components/common/PageBreadCrumb";
import ComponentCard from "../components/common/ComponentCard";
import { Table, TableBody, TableCell, TableHeader, TableRow } from "../components/ui/table";
import Button from "../components/ui/button/Button";
import Badge from "../components/ui/badge/Badge";
import Checkbox from "../components/form/input/Checkbox";
import { Modal } from "../components/ui/modal";
import { useCurrentUser } from "../context/CurrentUserContext";
import type { UserProfile } from "../services/auth";
import {
  createUser,
  generatePassword,
  listUsers,
  minPasswordLength,
  setUserPassword,
  updateUserAccess,
  type AccountRole,
  type NewUser,
  type UserAccess,
} from "../services/users";

const headerClass = "px-4 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400";
const cellClass = "px-4 py-3 text-start text-theme-sm text-gray-700 dark:text-gray-300";
const inputClass =
  "h-11 w-full rounded-lg border border-gray-200 bg-transparent px-4 py-2.5 text-sm text-gray-800 shadow-theme-xs focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-800 dark:bg-gray-900 dark:text-white/90";
const labelClass = "flex flex-col gap-1 text-sm text-gray-700 dark:text-gray-400";

const roles: AccountRole[] = ["USER", "ADMIN", "SUPER_ADMIN"];

/** The capability flags, in the order the table shows them (docs/access-profiles-plan.md). */
const flags = ["examines_facial", "examines_fingerprint", "intelligence_analyst", "coordinator"] as const;

const isAdminRole = (role: AccountRole) => role === "ADMIN" || role === "SUPER_ADMIN";

const emptyUser = (): NewUser => ({
  first_name: "",
  last_name: "",
  staff_number: "",
  job_title: "",
  username: "",
  password: generatePassword(),
  account_role: "USER",
  examines_facial: false,
  examines_fingerprint: false,
  intelligence_analyst: false,
  coordinator: false,
});

/** Admin: every account, its role and capabilities, switched on and off in place; new accounts and
 * password resets. The server's auth.CheckAccountChange decides; the page only disables what it
 * would refuse: an ADMIN manages plain users, and nobody changes their own role or deactivates
 * themselves. */
export default function Users() {
  const { t } = useTranslation();
  const { user: me } = useCurrentUser();
  const [users, setUsers] = useState<UserProfile[] | null>(null);
  const [error, setError] = useState("");
  const [filter, setFilter] = useState("");
  const [busy, setBusy] = useState<number | null>(null);
  const [creating, setCreating] = useState<NewUser | null>(null);
  const [resetting, setResetting] = useState<{ user: UserProfile; password: string } | null>(null);
  const [saving, setSaving] = useState(false);

  const isSuper = me?.account_role === "SUPER_ADMIN";

  const load = useCallback(() => {
    listUsers()
      .then(setUsers)
      .catch((err) => setError(err instanceof Error ? err.message : t("users.loadError")));
  }, [t]);

  useEffect(load, [load]);

  const shown = useMemo(() => {
    const q = filter.trim().toLowerCase();
    if (!users || !q) return users ?? [];
    return users.filter((u) =>
      [u.first_name, u.last_name, u.staff_number, u.username, u.job_title].some((v) => v.toLowerCase().includes(q)),
    );
  }, [users, filter]);

  const manages = (u: UserProfile) => isSuper || !isAdminRole(u.account_role);

  async function change(u: UserProfile, access: Partial<UserAccess>) {
    setBusy(u.id);
    try {
      const updated = await updateUserAccess(u.id, access);
      setUsers((list) => list?.map((x) => (x.id === updated.id ? updated : x)) ?? null);
      toast.success(t("users.saved", { username: updated.username }));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("users.saveError"));
    } finally {
      setBusy(null);
    }
  }

  async function submitCreate(event: FormEvent) {
    event.preventDefault();
    if (!creating) return;
    setSaving(true);
    try {
      const created = await createUser({ ...creating, username: creating.username || creating.staff_number });
      toast.success(t("users.created", { username: created.username }));
      setCreating(null);
      load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("users.saveError"));
    } finally {
      setSaving(false);
    }
  }

  async function submitReset(event: FormEvent) {
    event.preventDefault();
    if (!resetting) return;
    setSaving(true);
    try {
      await setUserPassword(resetting.user.id, resetting.password);
      toast.success(t("users.passwordSet", { username: resetting.user.username }));
      setResetting(null);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("users.saveError"));
    } finally {
      setSaving(false);
    }
  }

  return (
    <>
      <PageMeta title={`${t("users.title")} | TrackID`} description={t("users.desc")} />
      <PageBreadcrumb pageTitle={t("users.title")} />
      {error && <p className="mb-4 text-sm text-error-500">{error}</p>}

      <ComponentCard title={t("users.accounts")} desc={t("users.accountsDesc", { count: users?.length ?? 0 })}>
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <input
            type="search"
            value={filter}
            onChange={(event) => setFilter(event.target.value)}
            placeholder={t("users.filter")}
            className={`${inputClass} max-w-xs`}
          />
          <Button size="sm" onClick={() => setCreating(emptyUser())}>
            {t("users.new")}
          </Button>
        </div>

        {users && shown.length === 0 && (
          <p className="text-sm text-gray-500 dark:text-gray-400">{t("users.none")}</p>
        )}
        {shown.length > 0 && (
          <div className="overflow-x-auto">
            <Table>
              <TableHeader className="border-b border-gray-100 dark:border-white/[0.05]">
                <TableRow>
                  <TableCell isHeader className={headerClass}>{t("users.columns.name")}</TableCell>
                  <TableCell isHeader className={headerClass}>{t("users.columns.staffNumber")}</TableCell>
                  <TableCell isHeader className={headerClass}>{t("users.columns.role")}</TableCell>
                  {flags.map((f) => (
                    <TableCell key={f} isHeader className={headerClass}>{t(`users.flags.${f}`)}</TableCell>
                  ))}
                  <TableCell isHeader className={headerClass}>{t("users.columns.active")}</TableCell>
                  <TableCell isHeader className={headerClass}>{""}</TableCell>
                </TableRow>
              </TableHeader>
              <TableBody className="divide-y divide-gray-100 dark:divide-white/[0.05]">
                {shown.map((u) => {
                  const self = u.id === me?.id;
                  const locked = !manages(u) || busy === u.id;
                  return (
                    <TableRow key={u.id} className={u.is_active ? "" : "opacity-60"}>
                      <TableCell className={cellClass}>
                        <div className="font-medium text-gray-800 dark:text-white/90">
                          {u.first_name} {u.last_name}
                          {self && (
                            <span className="ml-2">
                              <Badge size="sm" color="info">{t("users.you")}</Badge>
                            </span>
                          )}
                        </div>
                        <div className="text-theme-xs text-gray-500 dark:text-gray-400">
                          {u.username} · {u.job_title}
                        </div>
                      </TableCell>
                      <TableCell className={cellClass}>{u.staff_number}</TableCell>
                      <TableCell className={cellClass}>
                        {isSuper && !self ? (
                          <select
                            value={u.account_role}
                            disabled={locked}
                            onChange={(event) => void change(u, { account_role: event.target.value as AccountRole })}
                            className={`${inputClass} h-9 w-auto py-1`}
                          >
                            {roles.map((r) => (
                              <option key={r} value={r}>{t(`users.roles.${r}`)}</option>
                            ))}
                          </select>
                        ) : (
                          t(`users.roles.${u.account_role}`)
                        )}
                      </TableCell>
                      {flags.map((f) => (
                        <TableCell key={f} className={cellClass}>
                          <Checkbox
                            checked={u[f]}
                            disabled={locked}
                            onChange={(checked) => void change(u, { [f]: checked })}
                          />
                        </TableCell>
                      ))}
                      <TableCell className={cellClass}>
                        <Checkbox
                          checked={u.is_active}
                          disabled={locked || self}
                          onChange={(checked) => void change(u, { is_active: checked })}
                        />
                      </TableCell>
                      <TableCell className={cellClass}>
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={!manages(u)}
                          onClick={() => setResetting({ user: u, password: generatePassword() })}
                        >
                          {t("users.resetPassword")}
                        </Button>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </div>
        )}
        <p className="mt-4 text-theme-xs text-gray-500 dark:text-gray-400">{t("users.flagsHelp")}</p>
      </ComponentCard>

      <Modal isOpen={!!creating} onClose={() => setCreating(null)} className="max-w-[700px] m-4">
        {creating && (
          <form onSubmit={(event) => void submitCreate(event)} className="p-6 lg:p-10">
            <h4 className="mb-2 text-2xl font-semibold text-gray-800 dark:text-white/90">{t("users.new")}</h4>
            <p className="mb-6 text-sm text-gray-500 dark:text-gray-400">{t("users.newDesc")}</p>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              {(["first_name", "last_name", "staff_number", "job_title"] as const).map((field) => (
                <label key={field} className={labelClass}>
                  {t(`users.fields.${field}`)}
                  <input
                    required
                    value={creating[field]}
                    onChange={(event) => setCreating({ ...creating, [field]: event.target.value })}
                    className={inputClass}
                  />
                </label>
              ))}
              <label className={labelClass}>
                {t("users.fields.username")}
                <input
                  value={creating.username}
                  placeholder={creating.staff_number.toLowerCase()}
                  onChange={(event) => setCreating({ ...creating, username: event.target.value })}
                  className={inputClass}
                />
              </label>
              <PasswordField
                value={creating.password}
                onChange={(password) => setCreating({ ...creating, password })}
              />
              <label className={labelClass}>
                {t("users.columns.role")}
                <select
                  value={creating.account_role}
                  onChange={(event) => setCreating({ ...creating, account_role: event.target.value as AccountRole })}
                  className={inputClass}
                >
                  {roles
                    .filter((r) => isSuper || r === "USER")
                    .map((r) => (
                      <option key={r} value={r}>{t(`users.roles.${r}`)}</option>
                    ))}
                </select>
              </label>
            </div>
            <div className="mt-5 grid grid-cols-1 gap-3 sm:grid-cols-2">
              {flags.map((f) => (
                <Checkbox
                  key={f}
                  label={t(`users.flags.${f}`)}
                  checked={creating[f]}
                  onChange={(checked) => setCreating({ ...creating, [f]: checked })}
                />
              ))}
            </div>
            <p className="mt-4 text-theme-xs text-gray-500 dark:text-gray-400">{t("users.passwordHelp")}</p>
            <div className="mt-6 flex justify-end gap-3">
              <Button type="button" size="sm" variant="outline" onClick={() => setCreating(null)}>
                {t("users.cancel")}
              </Button>
              <Button type="submit" size="sm" disabled={saving}>
                {t("users.create")}
              </Button>
            </div>
          </form>
        )}
      </Modal>

      <Modal isOpen={!!resetting} onClose={() => setResetting(null)} className="max-w-[500px] m-4">
        {resetting && (
          <form onSubmit={(event) => void submitReset(event)} className="p-6 lg:p-10">
            <h4 className="mb-2 text-2xl font-semibold text-gray-800 dark:text-white/90">{t("users.resetPassword")}</h4>
            <p className="mb-6 text-sm text-gray-500 dark:text-gray-400">
              {t("users.resetDesc", { username: resetting.user.username })}
            </p>
            <PasswordField
              value={resetting.password}
              onChange={(password) => setResetting({ ...resetting, password })}
            />
            <p className="mt-4 text-theme-xs text-gray-500 dark:text-gray-400">{t("users.passwordHelp")}</p>
            <div className="mt-6 flex justify-end gap-3">
              <Button type="button" size="sm" variant="outline" onClick={() => setResetting(null)}>
                {t("users.cancel")}
              </Button>
              <Button type="submit" size="sm" disabled={saving}>
                {t("users.setPassword")}
              </Button>
            </div>
          </form>
        )}
      </Modal>
    </>
  );
}

/** A password shown in clear, since the admin hands it over, with a button for a new random one. */
function PasswordField({ value, onChange }: { value: string; onChange: (value: string) => void }) {
  const { t } = useTranslation();
  return (
    <label className={labelClass}>
      {t("users.fields.password")}
      <div className="flex gap-2">
        <input
          required
          minLength={minPasswordLength}
          value={value}
          onChange={(event) => onChange(event.target.value)}
          className={`${inputClass} font-mono`}
          autoComplete="off"
        />
        <Button type="button" size="sm" variant="outline" onClick={() => onChange(generatePassword())}>
          {t("users.generate")}
        </Button>
      </div>
    </label>
  );
}
