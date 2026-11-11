import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import PageMeta from "../components/common/PageMeta";
import PageBreadcrumb from "../components/common/PageBreadCrumb";
import ComponentCard from "../components/common/ComponentCard";
import Button from "../components/ui/button/Button";
import Badge from "../components/ui/badge/Badge";
import Checkbox from "../components/form/input/Checkbox";
import { Modal } from "../components/ui/modal";
import { useCurrentUser } from "../context/CurrentUserContext";
import {
  createOrganization,
  createUnit,
  deleteOrganization,
  deleteUnit,
  effectiveHeader,
  listOrganizations,
  listUnits,
  updateOrganization,
  updateUnit,
  type Organization,
  type OrganizationInput,
  type Unit,
  type UnitInput,
} from "../services/organizations";

const inputClass =
  "h-11 w-full rounded-lg border border-gray-200 bg-transparent px-4 py-2.5 text-sm text-gray-800 shadow-theme-xs focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-800 dark:bg-gray-900 dark:text-white/90";
const textareaClass =
  "w-full rounded-lg border border-gray-200 bg-transparent px-4 py-2.5 text-sm text-gray-800 shadow-theme-xs focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-800 dark:bg-gray-900 dark:text-white/90";
const labelClass = "flex flex-col gap-1 text-sm text-gray-700 dark:text-gray-400";
const helpClass = "text-theme-xs text-gray-500 dark:text-gray-400";

type OrgForm = OrganizationInput & { id?: number };
type UnitForm = Omit<UnitInput, "laudo_header"> & { id?: number; organization_id: number; header: string; root: boolean };

const newUnit = (parent: Unit): UnitForm => ({
  organization_id: parent.organization_id,
  parent_id: parent.id,
  acronym: "",
  name: "",
  uf: parent.uf ?? "",
  municipality_code: "",
  header: "",
  laudo_name: "",
  active: true,
  receives_new_cases: false,
  root: false,
});

const editUnit = (u: Unit): UnitForm => ({
  id: u.id,
  organization_id: u.organization_id,
  parent_id: u.parent_id,
  acronym: u.acronym,
  name: u.name,
  uf: u.uf ?? "",
  municipality_code: u.municipality_code ?? "",
  header: u.laudo_header.join("\n"),
  laudo_name: u.laudo_name,
  active: u.active,
  receives_new_cases: u.receives_new_cases,
  root: u.parent_id === null,
});

const message = (err: unknown, fallback: string) => (err instanceof Error ? err.message : fallback);

/** Admin: organizations and the tree of units in each, with what a laudo says of each unit (its header,
 * inherited from the unit above when it has none, and the name its preâmbulo gives). Every admin manages
 * units; only a super admin manages organizations. Who works where is on the Users page. */
export default function Organizations() {
  const { t } = useTranslation();
  const { user: me } = useCurrentUser();
  const isSuper = me?.account_role === "SUPER_ADMIN";
  const [orgs, setOrgs] = useState<Organization[] | null>(null);
  const [units, setUnits] = useState<Unit[]>([]);
  const [selected, setSelected] = useState<number | null>(null);
  const [expanded, setExpanded] = useState<Set<number>>(new Set());
  const [filter, setFilter] = useState("");
  const [orgForm, setOrgForm] = useState<OrgForm | null>(null);
  const [unitForm, setUnitForm] = useState<UnitForm | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  const load = useCallback(() => {
    Promise.all([listOrganizations(), listUnits()])
      .then(([o, u]) => {
        setOrgs(o);
        setUnits(u);
        setSelected((s) => (s !== null && o.some((x) => x.id === s) ? s : (o[0]?.id ?? null)));
      })
      .catch((err) => setError(message(err, t("organizations.loadError"))));
  }, [t]);

  useEffect(load, [load]);

  const byId = useMemo(() => new Map(units.map((u) => [u.id, u])), [units]);
  const org = orgs?.find((o) => o.id === selected);
  const orgUnits = useMemo(() => units.filter((u) => u.organization_id === selected), [units, selected]);
  const children = useMemo(() => {
    const m = new Map<number | null, Unit[]>();
    for (const u of orgUnits) m.set(u.parent_id, [...(m.get(u.parent_id) ?? []), u]);
    for (const list of m.values()) list.sort((a, b) => a.acronym.localeCompare(b.acronym));
    return m;
  }, [orgUnits]);
  const root = children.get(null)?.[0];

  // With a filter, the matching units and the units above them, all open.
  const visible = useMemo(() => {
    const q = filter.trim().toLowerCase();
    if (!q) return null;
    const keep = new Set<number>();
    for (const u of orgUnits) {
      if ([u.acronym, u.name, u.laudo_name].some((v) => v.toLowerCase().includes(q))) {
        for (let x: Unit | undefined = u; x && !keep.has(x.id); x = x.parent_id ? byId.get(x.parent_id) : undefined) keep.add(x.id);
      }
    }
    return keep;
  }, [filter, orgUnits, byId]);

  useEffect(() => {
    if (root) setExpanded((e) => (e.size === 0 ? new Set([root.id]) : e));
  }, [root]);

  const toggle = (id: number) =>
    setExpanded((e) => {
      const next = new Set(e);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  // The units a unit may move below: its organization's, not itself nor any below it.
  const parentChoices = (form: UnitForm) => {
    if (form.root) return [];
    const below = new Set<number>();
    if (form.id !== undefined) {
      const walk = (id: number) => {
        below.add(id);
        for (const c of children.get(id) ?? []) walk(c.id);
      };
      walk(form.id);
    }
    return orgUnits.filter((u) => !below.has(u.id)).sort((a, b) => a.acronym.localeCompare(b.acronym));
  };

  async function run(action: () => Promise<unknown>, done: string) {
    setSaving(true);
    try {
      await action();
      toast.success(done);
      load();
      return true;
    } catch (err) {
      toast.error(message(err, t("organizations.saveError")));
      return false;
    } finally {
      setSaving(false);
    }
  }

  async function submitOrg(event: FormEvent) {
    event.preventDefault();
    if (!orgForm) return;
    const { id, ...input } = orgForm;
    const ok = await run(
      () => (id === undefined ? createOrganization(input) : updateOrganization(id, input)),
      t("organizations.saved", { name: input.acronym }),
    );
    if (ok) setOrgForm(null);
  }

  async function submitUnit(event: FormEvent) {
    event.preventDefault();
    if (!unitForm) return;
    const { id, header } = unitForm;
    const input: UnitInput = {
      parent_id: unitForm.parent_id,
      acronym: unitForm.acronym,
      name: unitForm.name,
      uf: unitForm.uf,
      municipality_code: unitForm.municipality_code,
      laudo_name: unitForm.laudo_name,
      active: unitForm.active,
      receives_new_cases: unitForm.receives_new_cases,
      laudo_header: header.split("\n").map((l) => l.trim()).filter(Boolean),
    };
    const ok = await run(
      () => (id === undefined ? createUnit(input) : updateUnit(id, input)),
      t("organizations.saved", { name: input.acronym }),
    );
    if (ok) {
      if (input.parent_id !== null) setExpanded((e) => new Set(e).add(input.parent_id as number));
      setUnitForm(null);
    }
  }

  function removeOrg(o: Organization) {
    if (!window.confirm(t("organizations.confirmDelete", { name: o.acronym }))) return;
    void run(() => deleteOrganization(o.id), t("organizations.deleted", { name: o.acronym }));
  }

  function removeUnit(u: Unit) {
    if (!window.confirm(t("organizations.confirmDelete", { name: u.acronym }))) return;
    void run(() => deleteUnit(u.id), t("organizations.deleted", { name: u.acronym }));
  }

  function renderUnit(u: Unit, depth: number) {
    if (visible && !visible.has(u.id)) return null;
    const kids = children.get(u.id) ?? [];
    const open = visible ? true : expanded.has(u.id);
    const header = org ? effectiveHeader(u, byId, org.name) : { lines: [] };
    const inherited = header.from?.id !== u.id;
    return (
      <div key={u.id}>
        <div
          className={`flex flex-wrap items-start justify-between gap-3 border-b border-gray-100 py-3 dark:border-white/[0.05] ${u.active ? "" : "opacity-60"}`}
          style={{ paddingLeft: depth * 20 }}
        >
          <div className="flex min-w-0 flex-1 gap-2">
            <button
              type="button"
              onClick={() => toggle(u.id)}
              disabled={kids.length === 0}
              className="mt-0.5 h-5 w-5 shrink-0 text-gray-500 disabled:invisible"
              aria-label={open ? t("organizations.collapse") : t("organizations.expand")}
            >
              {open ? "▾" : "▸"}
            </button>
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium text-gray-800 dark:text-white/90">{u.acronym}</span>
                <span className="text-sm text-gray-600 dark:text-gray-400">{u.name}</span>
                {!u.active && <Badge size="sm" color="light">{t("organizations.inactive")}</Badge>}
                {u.receives_new_cases && <Badge size="sm" color="info">{t("organizations.receivesNewCases")}</Badge>}
                {u.current_members > 0 && (
                  <Badge size="sm" color="success">{t("organizations.members", { count: u.current_members })}</Badge>
                )}
                {kids.length > 0 && <span className={helpClass}>{t("organizations.below", { count: kids.length })}</span>}
              </div>
              <div className={`${helpClass} mt-1`}>
                <span className={inherited ? "italic" : ""}>{header.lines.join(" · ")}</span>
                {inherited && header.from && <span> ({t("organizations.inheritedFrom", { unit: header.from.acronym })})</span>}
                {u.laudo_name && <span> — {u.laudo_name}</span>}
              </div>
            </div>
          </div>
          <div className="flex shrink-0 gap-2">
            <Button size="sm" variant="outline" onClick={() => setUnitForm(newUnit(u))}>
              {t("organizations.addBelow")}
            </Button>
            <Button size="sm" variant="outline" onClick={() => setUnitForm(editUnit(u))}>
              {t("organizations.edit")}
            </Button>
            {u.parent_id !== null && (
              <Button size="sm" variant="outline" onClick={() => removeUnit(u)}>
                {t("organizations.delete")}
              </Button>
            )}
          </div>
        </div>
        {open && kids.map((k) => renderUnit(k, depth + 1))}
      </div>
    );
  }

  const formHeaderPlaceholder = (form: UnitForm) => {
    if (!org || form.parent_id === null) return org?.name ?? "";
    const parent = byId.get(form.parent_id);
    return parent ? effectiveHeader(parent, byId, org.name).lines.join("\n") : "";
  };

  return (
    <>
      <PageMeta title={`${t("organizations.title")} | TrackID`} description={t("organizations.desc")} />
      <PageBreadcrumb pageTitle={t("organizations.title")} />
      {error && <p className="mb-4 text-sm text-error-500">{error}</p>}

      <div className="grid grid-cols-1 gap-6 xl:grid-cols-[320px_1fr]">
        <ComponentCard title={t("organizations.organizations")} desc={t("organizations.organizationsDesc")}>
          {isSuper && (
            <div className="mb-4">
              <Button
                size="sm"
                onClick={() => setOrgForm({ acronym: "", name: "", kind: "STATE", uf: "", active: true })}
              >
                {t("organizations.newOrganization")}
              </Button>
            </div>
          )}
          <ul className="flex flex-col gap-1">
            {orgs?.map((o) => (
              <li key={o.id}>
                <div
                  className={`flex items-center justify-between gap-2 rounded-lg px-3 py-2 ${
                    o.id === selected ? "bg-brand-50 dark:bg-brand-500/10" : "hover:bg-gray-50 dark:hover:bg-white/[0.03]"
                  } ${o.active ? "" : "opacity-60"}`}
                >
                  <button type="button" className="min-w-0 flex-1 text-start" onClick={() => setSelected(o.id)}>
                    <div className="text-sm font-medium text-gray-800 dark:text-white/90">{o.acronym}</div>
                    <div className={`${helpClass} truncate`}>
                      {o.name} · {t("organizations.unitCount", { count: o.unit_count })}
                    </div>
                  </button>
                  {isSuper && (
                    <div className="flex shrink-0 gap-1">
                      <button
                        type="button"
                        className="text-theme-xs text-brand-500 hover:underline"
                        onClick={() => setOrgForm({ id: o.id, acronym: o.acronym, name: o.name, kind: o.kind, uf: o.uf ?? "", active: o.active })}
                      >
                        {t("organizations.edit")}
                      </button>
                      <button type="button" className="text-theme-xs text-error-500 hover:underline" onClick={() => removeOrg(o)}>
                        {t("organizations.delete")}
                      </button>
                    </div>
                  )}
                </div>
              </li>
            ))}
          </ul>
        </ComponentCard>

        <ComponentCard title={org ? `${t("organizations.units")} — ${org.acronym}` : t("organizations.units")} desc={t("organizations.unitsDesc")}>
          <input
            type="search"
            value={filter}
            onChange={(event) => setFilter(event.target.value)}
            placeholder={t("organizations.filter")}
            className={`${inputClass} mb-2 max-w-xs`}
          />
          {root ? renderUnit(root, 0) : orgs && <p className={helpClass}>{t("organizations.noUnits")}</p>}
        </ComponentCard>
      </div>

      <Modal isOpen={!!orgForm} onClose={() => setOrgForm(null)} className="max-w-[600px] m-4">
        {orgForm && (
          <form onSubmit={(event) => void submitOrg(event)} className="p-6 lg:p-10">
            <h4 className="mb-2 text-2xl font-semibold text-gray-800 dark:text-white/90">
              {orgForm.id === undefined ? t("organizations.newOrganization") : orgForm.acronym}
            </h4>
            {orgForm.id === undefined && <p className={`${helpClass} mb-6`}>{t("organizations.newOrganizationDesc")}</p>}
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <label className={labelClass}>
                {t("organizations.fields.acronym")}
                <input required value={orgForm.acronym} onChange={(e) => setOrgForm({ ...orgForm, acronym: e.target.value })} className={inputClass} />
              </label>
              <label className={labelClass}>
                {t("organizations.fields.kind")}
                <select
                  value={orgForm.kind}
                  onChange={(e) => setOrgForm({ ...orgForm, kind: e.target.value as OrganizationInput["kind"] })}
                  className={inputClass}
                >
                  <option value="FEDERAL">{t("organizations.kinds.FEDERAL")}</option>
                  <option value="STATE">{t("organizations.kinds.STATE")}</option>
                </select>
              </label>
              <label className={`${labelClass} sm:col-span-2`}>
                {t("organizations.fields.name")}
                <input required value={orgForm.name} onChange={(e) => setOrgForm({ ...orgForm, name: e.target.value })} className={inputClass} />
              </label>
              {orgForm.kind === "STATE" && (
                <label className={labelClass}>
                  {t("organizations.fields.uf")}
                  <input
                    required
                    maxLength={2}
                    value={orgForm.uf}
                    onChange={(e) => setOrgForm({ ...orgForm, uf: e.target.value.toUpperCase() })}
                    className={inputClass}
                  />
                </label>
              )}
            </div>
            {orgForm.id !== undefined && (
              <div className="mt-5">
                <Checkbox label={t("organizations.fields.active")} checked={orgForm.active} onChange={(active) => setOrgForm({ ...orgForm, active })} />
              </div>
            )}
            <div className="mt-6 flex justify-end gap-3">
              <Button type="button" size="sm" variant="outline" onClick={() => setOrgForm(null)}>
                {t("organizations.cancel")}
              </Button>
              <Button type="submit" size="sm" disabled={saving}>
                {t("organizations.save")}
              </Button>
            </div>
          </form>
        )}
      </Modal>

      <Modal isOpen={!!unitForm} onClose={() => setUnitForm(null)} className="max-w-[760px] m-4">
        {unitForm && (
          <form onSubmit={(event) => void submitUnit(event)} className="p-6 lg:p-10">
            <h4 className="mb-6 text-2xl font-semibold text-gray-800 dark:text-white/90">
              {unitForm.id === undefined ? t("organizations.newUnit") : unitForm.acronym}
            </h4>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              {!unitForm.root && (
                <label className={`${labelClass} sm:col-span-2`}>
                  {t("organizations.fields.parent")}
                  <select
                    value={unitForm.parent_id ?? ""}
                    onChange={(e) => setUnitForm({ ...unitForm, parent_id: Number(e.target.value) })}
                    className={inputClass}
                  >
                    {parentChoices(unitForm).map((u) => (
                      <option key={u.id} value={u.id}>
                        {u.acronym} — {u.name}
                      </option>
                    ))}
                  </select>
                </label>
              )}
              <label className={labelClass}>
                {t("organizations.fields.acronym")}
                <input required value={unitForm.acronym} onChange={(e) => setUnitForm({ ...unitForm, acronym: e.target.value })} className={inputClass} />
              </label>
              <label className={labelClass}>
                {t("organizations.fields.name")}
                <input required value={unitForm.name} onChange={(e) => setUnitForm({ ...unitForm, name: e.target.value })} className={inputClass} />
              </label>
              <label className={labelClass}>
                {t("organizations.fields.uf")}
                <input
                  maxLength={2}
                  value={unitForm.uf}
                  onChange={(e) => setUnitForm({ ...unitForm, uf: e.target.value.toUpperCase() })}
                  className={inputClass}
                />
              </label>
              <label className={labelClass}>
                {t("organizations.fields.municipality")}
                <input
                  maxLength={7}
                  inputMode="numeric"
                  value={unitForm.municipality_code}
                  onChange={(e) => setUnitForm({ ...unitForm, municipality_code: e.target.value })}
                  className={inputClass}
                />
              </label>
            </div>

            <h5 className="mb-1 mt-6 text-base font-semibold text-gray-800 dark:text-white/90">{t("organizations.laudo")}</h5>
            <p className={`${helpClass} mb-3`}>{t("organizations.laudoDesc")}</p>
            <div className="grid grid-cols-1 gap-4">
              <label className={labelClass}>
                {t("organizations.fields.laudoHeader")}
                <textarea
                  rows={3}
                  value={unitForm.header}
                  placeholder={formHeaderPlaceholder(unitForm)}
                  onChange={(e) => setUnitForm({ ...unitForm, header: e.target.value })}
                  className={textareaClass}
                />
                <span className={helpClass}>{t("organizations.laudoHeaderHelp")}</span>
              </label>
              <label className={labelClass}>
                {t("organizations.fields.laudoName")}
                <input
                  value={unitForm.laudo_name}
                  placeholder={unitForm.name}
                  onChange={(e) => setUnitForm({ ...unitForm, laudo_name: e.target.value })}
                  className={inputClass}
                />
                <span className={helpClass}>{t("organizations.laudoNameHelp")}</span>
              </label>
            </div>

            <div className="mt-5 grid grid-cols-1 gap-3 sm:grid-cols-2">
              <Checkbox
                label={t("organizations.fields.active")}
                checked={unitForm.active}
                disabled={unitForm.receives_new_cases && unitForm.id !== undefined && byId.get(unitForm.id)?.receives_new_cases}
                onChange={(active) => setUnitForm({ ...unitForm, active })}
              />
              <Checkbox
                label={t("organizations.receivesNewCases")}
                checked={unitForm.receives_new_cases}
                disabled={unitForm.id !== undefined && byId.get(unitForm.id)?.receives_new_cases}
                onChange={(receives_new_cases) => setUnitForm({ ...unitForm, receives_new_cases })}
              />
            </div>
            <p className={`${helpClass} mt-2`}>{t("organizations.receivesNewCasesHelp")}</p>
            <div className="mt-6 flex justify-end gap-3">
              <Button type="button" size="sm" variant="outline" onClick={() => setUnitForm(null)}>
                {t("organizations.cancel")}
              </Button>
              <Button type="submit" size="sm" disabled={saving}>
                {t("organizations.save")}
              </Button>
            </div>
          </form>
        )}
      </Modal>
    </>
  );
}
