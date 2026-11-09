import { Link } from "react-router";
import { useTranslation } from "react-i18next";

import Badge from "../ui/badge/Badge";
import { personPath } from "../../services/persons";
import type { ConflictRegister, ConflictStrength, IdentityConflict } from "../../services/clusters";

// Identity conflicts (cluster.ListConflicts), shown the same on the cluster page, the person page and
// in the AI mode: one biometric under different names, or one identity with different biometrics. They
// are flags for an examiner, so the wording stays on what was found and says nothing of who is who.

const linkClass = "font-medium text-brand-500 hover:text-brand-600 dark:text-brand-400 dark:hover:text-brand-300";

function strengthColor(strength: ConflictStrength): "success" | "warning" | "light" | "error" {
  switch (strength) {
    case "REVIEWED":
      return "success";
    case "TWO_MODALITIES":
      return "warning";
    case "HUMAN_NEGATIVE":
      return "error";
    default:
      return "light";
  }
}

/** A conflict's kind and strength, as the clusters listing shows it. */
export function ConflictBadges({ kind, strength }: Pick<IdentityConflict, "kind" | "strength">) {
  const { t } = useTranslation();
  return (
    <span className="inline-flex flex-wrap items-center gap-1" title={t(`identityConflict.strengthHint.${strength}`)}>
      <Badge size="sm" color="warning">
        {t(`identityConflict.kind.${kind}`)}
      </Badge>
      <Badge size="sm" color={strengthColor(strength)}>
        {t(`identityConflict.strength.${strength}`)}
      </Badge>
    </span>
  );
}

function fold(value: string | undefined): string {
  return (value ?? "").trim().toLowerCase();
}

/** The registers side by side. A value that differs from the first register's is marked, parents in
 * either order: what to compare is whether these records describe one person. */
function RegisterComparison({ registers }: { registers: ConflictRegister[] }) {
  const { t } = useTranslation();
  const first = registers[0];
  const firstParents = [fold(first.parent1Name), fold(first.parent2Name)];
  const cell = (value: string | undefined, differs: boolean) => (
    <td className={`px-2 py-1 ${differs ? "font-medium text-warning-700 dark:text-warning-400" : "text-gray-700 dark:text-gray-300"}`}>{value || "—"}</td>
  );
  return (
    <div className="flex flex-col gap-1">
      <div className="overflow-x-auto rounded-lg border border-gray-200 bg-white dark:border-gray-800 dark:bg-white/[0.02]">
        <table className="w-full text-xs">
          <thead className="text-left text-gray-500 dark:text-gray-400">
            <tr>
              <th className="px-2 py-1 font-medium whitespace-nowrap">{t("identityConflict.compare.person")}</th>
              <th className="px-2 py-1 font-medium whitespace-nowrap">{t("identityConflict.compare.document")}</th>
              <th className="px-2 py-1 font-medium whitespace-nowrap">{t("identityConflict.compare.fiscal")}</th>
              <th className="px-2 py-1 font-medium whitespace-nowrap">{t("identityConflict.compare.name")}</th>
              <th className="px-2 py-1 font-medium whitespace-nowrap">{t("identityConflict.compare.birthDate")}</th>
              <th className="px-2 py-1 font-medium whitespace-nowrap">{t("identityConflict.compare.parent1")}</th>
              <th className="px-2 py-1 font-medium whitespace-nowrap">{t("identityConflict.compare.parent2")}</th>
            </tr>
          </thead>
          <tbody>
            {registers.map((r) => (
              <tr key={r.registerNumber} className="border-t border-gray-100 align-top dark:border-gray-800">
                <td className="px-2 py-1 whitespace-nowrap">
                  <Link to={personPath(r.personNumber)} className={linkClass}>
                    #{r.personNumber}
                  </Link>
                </td>
                <td className="px-2 py-1 whitespace-nowrap text-gray-700 dark:text-gray-300">
                  {r.documentType} {r.documentNumber}
                </td>
                {cell(r.fiscalNumber, fold(r.fiscalNumber) !== fold(first.fiscalNumber))}
                {cell(r.name, fold(r.name) !== fold(first.name))}
                {cell(r.birthDate, fold(r.birthDate) !== fold(first.birthDate))}
                {cell(r.parent1Name, !firstParents.includes(fold(r.parent1Name)))}
                {cell(r.parent2Name, !firstParents.includes(fold(r.parent2Name)))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <p className="text-xs text-gray-500 dark:text-gray-400">{t("identityConflict.compare.hint")}</p>
    </div>
  );
}

function ConflictRow({ conflict, currentCluster }: { conflict: IdentityConflict; currentCluster?: number }) {
  const { t, i18n } = useTranslation();
  const number = (value: number) => value.toLocaleString(i18n.language, { maximumFractionDigits: 3 });
  const clusters = [...conflict.clusterIds, ...(conflict.reviewedClusterIds ?? [])];
  return (
    <div className="flex flex-col gap-2 rounded-lg border border-warning-500/40 bg-warning-50/40 p-3 dark:bg-warning-500/5">
      <ConflictBadges kind={conflict.kind} strength={conflict.strength} />
      <p className="text-xs text-gray-600 dark:text-gray-400">
        {t(`identityConflict.kindHint.${conflict.kind}`)} {t(`identityConflict.strengthHint.${conflict.strength}`)}
        {conflict.identityBasis && ` ${t(`identityConflict.identityBasis.${conflict.identityBasis}`)}`}
      </p>
      {conflict.persons.length > 2 && <p className="text-xs text-warning-700 dark:text-warning-400">{t("identityConflict.chainNote", { count: conflict.persons.length })}</p>}

      <div className="flex flex-col gap-1 text-sm text-gray-700 dark:text-gray-300">
        {conflict.persons.map((person) => (
          <div key={person.personNumber} className="flex flex-wrap items-baseline gap-x-2">
            <Link to={personPath(person.personNumber)} className={linkClass}>
              {person.names.join(" / ")}
            </Link>
            <span className="text-xs text-gray-500 dark:text-gray-400">
              #{person.personNumber} · {person.birthDates.join(" / ") || "—"} · {person.documents.map((d) => `${d.type} ${d.number}`).join(" · ")}
            </span>
            <span className="text-xs text-gray-500 dark:text-gray-400">
              {person.status
                ? person.status.identified
                  ? t(`identityConflict.status.${person.status.basis}`)
                  : t("identityConflict.status.BELOW_LEVEL")
                : t("identityConflict.status.NONE")}
            </span>
          </div>
        ))}
      </div>

      <p className="flex flex-wrap items-center gap-x-2 text-xs text-gray-500 dark:text-gray-400">
        <span>{t("identityConflict.clusters")}:</span>
        {clusters.map((id) =>
          id === currentCluster ? (
            <span key={id} className="font-medium">
              #{id}
            </span>
          ) : (
            <Link key={id} to={`/clusters/${id}`} className={linkClass}>
              #{id}
            </Link>
          ),
        )}
        <span>· {t("identityConflict.traces", { count: conflict.traces })}</span>
      </p>

      {conflict.registers && conflict.registers.length > 0 && <RegisterComparison registers={conflict.registers} />}

      {conflict.comparisons.length > 0 && (
        <div className="flex flex-col gap-0.5">
          {conflict.comparisons.map((c, i) => (
            <p key={i} className="text-xs break-words text-gray-600 dark:text-gray-400">
              <span className="font-medium">{t(`identityConflict.modality.${c.modality}`, c.modality)}</span>
              {": "}
              {c.a.documentType} {c.a.documentNumber} ({c.a.name}) ↔ {c.b.documentType} {c.b.documentNumber} ({c.b.name}){" — "}
              {c.score != null
                ? c.measured
                  ? t("identityConflict.measured", { score: number(c.score) })
                  : t("identityConflict.score", { score: number(c.score), threshold: c.threshold != null ? number(c.threshold) : "—" })
                : t("identityConflict.noComparison")}
              {c.settledBy && c.settledBy !== "SYSTEM" && c.status !== "NO_DECISION" && ` · ${t("identityConflict.settledBy", { status: t(`identityConflict.edge.${c.status}`, c.status ?? "") })}`}
            </p>
          ))}
        </div>
      )}
    </div>
  );
}

/** The conflicts, with nothing around them. */
export function IdentityConflictList({ conflicts, currentCluster }: { conflicts: IdentityConflict[]; currentCluster?: number }) {
  return (
    <div className="flex flex-col gap-2">
      {conflicts.map((conflict, i) => (
        <ConflictRow key={i} conflict={conflict} currentCluster={currentCluster} />
      ))}
    </div>
  );
}
