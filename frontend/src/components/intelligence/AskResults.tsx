import { lazy, Suspense, type ReactNode } from "react";
import { Link } from "react-router";
import { useTranslation } from "react-i18next";

import Badge from "../ui/badge/Badge";
import { IdentityConflictList } from "../clusters/IdentityConflicts";
import type { AskPoint } from "./AskPointsMap";
import type {
  AskCaseDetail,
  AskCluster,
  AskFoundPerson,
  AskIdentityConflicts,
  AskInquiry,
  AskLinkedPerson,
  AskNear,
  AskOccurrence,
  AskOccurrenceSearch,
  AskPersonCases,
  AskPersonClusters,
  AskPersonIdentities,
  AskPersonSearch,
} from "../../services/ask";

// The results of the AI mode's walk tools, drawn from what the database returned and not from the
// model's prose: this is what an analyst checks an answer against, and where they can leave the
// conversation for the full page.

// MapLibre is about 1 MB: loaded only when an answer has points.
const AskPointsMap = lazy(() => import("./AskPointsMap"));

function Points({ points }: { points: AskPoint[] }) {
  if (points.length === 0) return null;
  return (
    <Suspense fallback={null}>
      <AskPointsMap points={points} />
    </Suspense>
  );
}

const linkClass = "text-brand-500 hover:underline";

function occurrenceLabel(o: { id: number; trackidNumber?: string; agency?: string; number?: string; inquiryNumber?: string }) {
  if (o.trackidNumber) return o.trackidNumber;
  if (o.number) return [o.agency, o.number].filter(Boolean).join(" ");
  if (o.inquiryNumber) return `IPL ${o.inquiryNumber}`;
  return `#${o.id}`;
}

function pointOf(o: AskOccurrence): AskPoint | null {
  // A point known only to the municipality is its centre, not a place to mark.
  if (o.lat == null || o.lon == null || o.precision === "MUNICIPALITY") return null;
  return { lat: o.lat, lon: o.lon, label: occurrenceLabel(o) };
}

function pointsOf(list: (AskOccurrence | undefined)[]): AskPoint[] {
  const seen = new Set<number>();
  const out: AskPoint[] = [];
  for (const o of list) {
    if (!o || seen.has(o.id)) continue;
    seen.add(o.id);
    const p = pointOf(o);
    if (p) out.push(p);
  }
  return out;
}

function LinkBadge({ link }: { link: string }) {
  const { t } = useTranslation();
  return (
    <span title={t(`ask.linkHint.${link}`)}>
      <Badge size="sm" color={link === "REVIEWED" ? "success" : "warning"}>
        {t(`ask.link.${link}`)}
      </Badge>
    </span>
  );
}

function PersonLink({ number, names }: { number: number; names: string[] }) {
  return (
    <Link to={`/persons/${number}`} className={linkClass}>
      {names.join(" / ") || `#${number}`}
    </Link>
  );
}

function CaseLink({ id }: { id: string }) {
  return (
    <Link to={`/cases/${encodeURIComponent(id)}`} className={`${linkClass} whitespace-nowrap`}>
      {id}
    </Link>
  );
}

function ClusterLink({ id }: { id: number }) {
  return (
    <Link to={`/clusters/${id}`} className={`${linkClass} whitespace-nowrap`}>
      #{id}
    </Link>
  );
}

function ClusterLinks({ ids }: { ids?: number[] }) {
  if (!ids?.length) return null;
  return (
    <span className="ml-1 inline-flex flex-wrap gap-1 text-xs text-gray-400">
      {ids.map((id) => (
        <ClusterLink key={id} id={id} />
      ))}
    </span>
  );
}

function Table({ head, children }: { head: string[]; children: ReactNode }) {
  return (
    <div className="max-h-96 overflow-auto rounded-lg border border-gray-200 dark:border-gray-800">
      <table className="w-full text-sm">
        <thead className="sticky top-0 bg-gray-50 text-left text-xs text-gray-500 dark:bg-gray-900 dark:text-gray-400">
          <tr>
            {head.map((h) => (
              <th key={h} className="px-3 py-1.5 font-medium whitespace-nowrap">
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody className="text-gray-700 dark:text-gray-300">{children}</tbody>
      </table>
    </div>
  );
}

const rowClass = "border-t border-gray-100 align-top dark:border-gray-800";
const cellClass = "px-3 py-1.5";

function PersonsTable({ persons }: { persons: AskLinkedPerson[] }) {
  const { t } = useTranslation();
  if (persons.length === 0) return <p className="text-xs text-gray-500 dark:text-gray-400">{t("ask.noPersons")}</p>;
  return (
    <Table head={[t("ask.person"), t("ask.birthDate"), t("ask.linkLabel")]}>
      {persons.map((p) => (
        <tr key={p.personNumber} className={rowClass}>
          <td className={cellClass}>
            <PersonLink number={p.personNumber} names={p.names} />
            <ClusterLinks ids={p.clusterIds} />
            {p.sameTraceAs && p.sameTraceAs.length > 0 && (
              <div className="text-xs text-warning-600 dark:text-warning-400">{t("ask.sameTraceAs", { people: p.sameTraceAs.map((n) => `#${n}`).join(", ") })}</div>
            )}
          </td>
          <td className={`${cellClass} whitespace-nowrap`}>{p.birthDate ?? t("ask.notRecorded")}</td>
          <td className={cellClass}>
            <LinkBadge link={p.link} />
          </td>
        </tr>
      ))}
    </Table>
  );
}

export function PersonSearchResult({ result }: { result: AskPersonSearch }) {
  const { t } = useTranslation();
  if (result.rows.length === 0) return <p className="text-sm text-gray-500 dark:text-gray-400">{t("ask.noneFound")}</p>;
  const counts = (p: AskFoundPerson) => t("ask.personCases", { reviewed: p.reviewedCases, leads: p.cases - p.reviewedCases });
  return (
    <Table head={[t("ask.person"), t("ask.birthDate"), t("ask.documents"), t("ask.cases")]}>
      {result.rows.map((p) => (
        <tr key={p.personNumber} className={rowClass}>
          <td className={cellClass}>
            <PersonLink number={p.personNumber} names={p.names} />
            <span className="ml-1 text-xs text-gray-400">#{p.personNumber}</span>
          </td>
          <td className={`${cellClass} whitespace-nowrap`}>{p.birthDates.join(" / ") || t("ask.notRecorded")}</td>
          <td className={cellClass}>{p.documents.map((d) => `${d.type} ${d.number}`).join(" · ")}</td>
          <td className={`${cellClass} whitespace-nowrap`}>{counts(p)}</td>
        </tr>
      ))}
    </Table>
  );
}

export function PersonCasesResult({ result }: { result: AskPersonCases }) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-2">
      <p className="text-sm text-gray-700 dark:text-gray-300">
        <PersonLink number={result.person.personNumber} names={result.person.names} />
        {" · "}
        {t("ask.personCases", { reviewed: result.reviewedCount, leads: result.leadCount })}
      </p>
      {result.rows.length > 0 && (
        <Table head={[t("ask.case"), t("ask.linkLabel"), t("ask.occurrence"), t("ask.date"), t("ask.types"), t("ask.place"), "IPL"]}>
          {result.rows.map((c) => (
            <tr key={c.caseId} className={rowClass}>
              <td className={cellClass}>
                <CaseLink id={c.caseId} />
                <div className="text-xs text-gray-400">
                  {t(`ask.modality.${c.modality}`, c.modality)}
                  <ClusterLinks ids={c.clusterIds} />
                </div>
              </td>
              <td className={cellClass}>
                <LinkBadge link={c.link} />
              </td>
              <td className={`${cellClass} whitespace-nowrap`}>{c.occurrence ? occurrenceLabel(c.occurrence) : t("ask.notLinked")}</td>
              <td className={`${cellClass} whitespace-nowrap`}>{c.occurrence?.occurredFrom ?? ""}</td>
              <td className={cellClass}>{c.occurrence?.types.map((code) => t(`occurrenceType.${code}`, code)).join(" · ")}</td>
              <td className={cellClass}>{c.occurrence ? [c.occurrence.placeName, c.occurrence.neighbourhood, c.occurrence.uf].filter(Boolean).join(" · ") : ""}</td>
              <td className={`${cellClass} whitespace-nowrap`}>{c.occurrence?.inquiryNumber ?? ""}</td>
            </tr>
          ))}
        </Table>
      )}
      <Points points={pointsOf(result.rows.map((c) => c.occurrence))} />
    </div>
  );
}

const EVIDENCE_COLORS = { REVIEWED: "success", TWO_MODALITIES: "warning", ONE_MODALITY: "light", INDIRECT: "light", HUMAN_NEGATIVE: "error" } as const;

export function PersonIdentitiesResult({ result }: { result: AskPersonIdentities }) {
  const { t, i18n } = useTranslation();
  const coincide = (same: boolean) => (same ? t("ask.same") : t("ask.different"));
  return (
    <div className="flex flex-col gap-2">
      <p className="text-sm text-gray-700 dark:text-gray-300">
        <PersonLink number={result.person.personNumber} names={result.person.names} />
        {" · "}
        {result.person.documents.map((d) => `${d.type} ${d.number}`).join(" · ")}
      </p>
      {result.rows.length === 0 && <p className="text-sm text-gray-500 dark:text-gray-400">{t("ask.noIdentities")}</p>}
      {result.rows.map((row) => (
        <div key={row.person.personNumber} className="flex flex-col gap-1.5 rounded-lg border border-gray-200 p-3 dark:border-gray-800">
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <PersonLink number={row.person.personNumber} names={row.person.names} />
            <span className="text-xs text-gray-400">#{row.person.personNumber}</span>
            <span title={t(`ask.evidenceHint.${row.evidence}`)}>
              <Badge size="sm" color={EVIDENCE_COLORS[row.evidence]}>
                {t(`ask.evidence.${row.evidence}`)}
              </Badge>
            </span>
          </div>
          <p className="text-xs text-gray-500 dark:text-gray-400">{t(`ask.evidenceHint.${row.evidence}`)}</p>
          <p className="text-xs text-gray-600 dark:text-gray-400">
            {t("ask.nameIs", { value: coincide(row.sharesName) })} · {t("ask.birthDateIs", { value: coincide(row.sharesBirthDate) })} ·{" "}
            {t("ask.parentIs", { value: coincide(row.sharesParent) })} · {t("ask.sharedTraces", { count: row.sharedTraces })}
          </p>
          {row.comparisons.map((c) => (
            <div key={c.modality} className="text-sm text-gray-700 dark:text-gray-300">
              <span className="font-medium">{t(`ask.modality.${c.modality}`, c.modality)}</span>
              {c.score != null && (
                <span className="ml-2 tabular-nums">
                  {t("ask.score", { score: c.score.toLocaleString(i18n.language, { maximumFractionDigits: 3 }), threshold: c.threshold?.toLocaleString(i18n.language) })}
                </span>
              )}
              {c.decision && <span className="ml-2 text-xs text-gray-500 dark:text-gray-400">{t(`ask.decision.${c.decision}`, c.decision)}</span>}
              {c.examinerDecisions?.map((d) => (
                <div key={d} className="text-xs">
                  {d}
                </div>
              ))}
              {c.matches?.map((m) => (
                <div key={m} className="text-xs break-words text-gray-500 dark:text-gray-400">
                  {m}
                </div>
              ))}
            </div>
          ))}
        </div>
      ))}
      {result.weakCandidates > 0 && <p className="text-xs text-gray-500 dark:text-gray-400">{t("ask.weakCandidates", { count: result.weakCandidates })}</p>}
    </div>
  );
}

export function IdentityConflictsResult({ result }: { result: AskIdentityConflicts }) {
  const { t } = useTranslation();
  if (result.total === 0) return <p className="text-sm text-gray-500 dark:text-gray-400">{t("identityConflict.none")}</p>;
  return (
    <div className="flex flex-col gap-2">
      <p className="text-sm text-gray-700 dark:text-gray-300">
        {t("identityConflict.total", { count: result.total })}
        {": "}
        {Object.entries(result.byStrength)
          .map(([strength, count]) => `${count} ${t(`identityConflict.strength.${strength}`, strength)}`)
          .join(" · ")}
      </p>
      <IdentityConflictList conflicts={result.rows} />
    </div>
  );
}

export function ClusterResult({ result }: { result: AskCluster }) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-2">
      <p className="text-sm text-gray-700 dark:text-gray-300">
        <ClusterLink id={result.clusterId} /> · {t(`ask.modality.${result.modality}`, result.modality)} · {t(`ask.basis.${result.basis}`)} ·{" "}
        {t("ask.clusterMembers", { count: result.members })}
      </p>
      {result.conflicts.length > 0 && <IdentityConflictList conflicts={result.conflicts} currentCluster={result.clusterId} />}
      {result.persons.length > 0 && (
        <Table head={[t("ask.person"), t("ask.birthDate"), t("ask.samples")]}>
          {result.persons.map((p) => (
            <tr key={p.personNumber} className={rowClass}>
              <td className={cellClass}>
                <PersonLink number={p.personNumber} names={p.names} />
              </td>
              <td className={`${cellClass} whitespace-nowrap`}>{p.birthDate ?? t("ask.notRecorded")}</td>
              <td className={`${cellClass} tabular-nums`}>{p.samples}</td>
            </tr>
          ))}
        </Table>
      )}
      {result.cases.length > 0 && (
        <Table head={[t("ask.case"), t("ask.occurrence"), t("ask.date"), t("ask.types"), "IPL"]}>
          {result.cases.map((c) => (
            <tr key={c.caseId} className={rowClass}>
              <td className={cellClass}>
                <CaseLink id={c.caseId} />
              </td>
              <td className={`${cellClass} whitespace-nowrap`}>{c.occurrence ? occurrenceLabel(c.occurrence) : t("ask.notLinked")}</td>
              <td className={`${cellClass} whitespace-nowrap`}>{c.occurrence?.occurredFrom ?? ""}</td>
              <td className={cellClass}>{c.occurrence?.types.map((code) => t(`occurrenceType.${code}`, code)).join(" · ")}</td>
              <td className={`${cellClass} whitespace-nowrap`}>{c.occurrence?.inquiryNumber ?? ""}</td>
            </tr>
          ))}
        </Table>
      )}
      {result.linkedClusters.length > 0 && (
        <p className="flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
          <span>{t("ask.linkedClusters")}:</span>
          {result.linkedClusters.map((c) => (
            <span key={c.clusterId}>
              <ClusterLink id={c.clusterId} /> {t(`ask.modality.${c.modality}`, c.modality)}
            </span>
          ))}
        </p>
      )}
      <Points points={pointsOf(result.cases.map((c) => c.occurrence))} />
    </div>
  );
}

export function PersonClustersResult({ result }: { result: AskPersonClusters }) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-2">
      <p className="text-sm text-gray-700 dark:text-gray-300">
        <PersonLink number={result.person.personNumber} names={result.person.names} />
      </p>
      {result.conflicts.length > 0 && <IdentityConflictList conflicts={result.conflicts} />}
      {result.rows.length > 0 ? (
        <Table head={[t("ask.cluster"), t("ask.modalityLabel"), t("ask.layer"), t("ask.samples"), t("ask.traces")]}>
          {result.rows.map((c) => (
            <tr key={c.clusterId} className={rowClass}>
              <td className={cellClass}>
                <ClusterLink id={c.clusterId} />
              </td>
              <td className={cellClass}>{t(`ask.modality.${c.modality}`, c.modality)}</td>
              <td className={cellClass}>{t(`ask.basis.${c.basis}`)}</td>
              <td className={`${cellClass} tabular-nums`}>{c.members}</td>
              <td className={`${cellClass} tabular-nums`}>{c.traces}</td>
            </tr>
          ))}
        </Table>
      ) : (
        <p className="text-sm text-gray-500 dark:text-gray-400">{t("ask.noneFound")}</p>
      )}
    </div>
  );
}

/** One occurrence's when, where and what. */
function OccurrenceFacts({ occurrence: o }: { occurrence: AskOccurrence }) {
  const { t } = useTranslation();
  const facts: [string, ReactNode][] = [];
  const add = (label: string, value: ReactNode) => {
    if (value !== undefined && value !== null && value !== "") facts.push([label, value]);
  };
  add(t("ask.types"), o.types.map((code) => t(`occurrenceType.${code}`, code)).join(" · ") + (o.attempted ? ` (${t("ask.attempted")})` : ""));
  add(t("ask.occurredFrom"), o.occurredFrom ?? t("ask.notRecorded"));
  add(t("ask.occurredTo"), o.occurredTo);
  add(t("ask.reportedAt"), o.reportedAt);
  add(t("ask.status"), t(`occurrenceStatus.${o.status}`, o.status));
  add(t("ask.place"), [o.placeName, o.address, o.neighbourhood, o.municipality, o.uf].filter(Boolean).join(" · ") || t("ask.notRecorded"));
  add(t("ask.placeType"), o.placeType && t(`placeType.${o.placeType}`, o.placeType));
  add(t("ask.precision"), o.precision && t(`precision.${o.precision}`, o.precision));
  add("IPL", o.inquiryNumber);
  add(t("ask.agency"), o.agency);
  add(t("ask.victims"), o.victimCount);
  add(t("ask.offenders"), o.offenderCount);
  add(t("ask.description"), o.description);
  return (
    <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-sm">
      {facts.map(([label, value]) => (
        <div key={label} className="contents">
          <dt className="text-xs text-gray-500 dark:text-gray-400">{label}</dt>
          <dd className="break-words text-gray-700 dark:text-gray-300">{value}</dd>
        </div>
      ))}
    </dl>
  );
}

function OccurrenceCases({ occurrence: o }: { occurrence: AskOccurrence }) {
  const { t } = useTranslation();
  if (!o.cases?.length) return <p className="text-xs text-gray-500 dark:text-gray-400">{t("ask.noCases")}</p>;
  return (
    <>
      {o.cases.map((c) => (
        <div key={c.caseId} className="flex flex-col gap-1">
          <p className="text-sm text-gray-700 dark:text-gray-300">
            <CaseLink id={c.caseId} /> · {t(`ask.modality.${c.modality}`, c.modality)}
          </p>
          <PersonsTable persons={c.persons} />
        </div>
      ))}
    </>
  );
}

export function OccurrenceResult({ occurrence }: { occurrence: AskOccurrence }) {
  return (
    <div className="flex flex-col gap-2">
      <p className="text-sm font-medium text-gray-800 dark:text-white/90">{occurrenceLabel(occurrence)}</p>
      <OccurrenceFacts occurrence={occurrence} />
      <Points points={pointsOf([occurrence])} />
      <OccurrenceCases occurrence={occurrence} />
    </div>
  );
}

export function InquiryResult({ result }: { result: AskInquiry }) {
  return (
    <div className="flex flex-col gap-4">
      {result.rows.map((o) => (
        <div key={o.id} className="flex flex-col gap-2">
          <p className="text-sm font-medium text-gray-800 dark:text-white/90">{occurrenceLabel(o)}</p>
          <OccurrenceFacts occurrence={o} />
          <OccurrenceCases occurrence={o} />
        </div>
      ))}
      <Points points={pointsOf(result.rows)} />
    </div>
  );
}

export function CaseDetailResult({ result }: { result: AskCaseDetail }) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-2">
      <p className="text-sm text-gray-700 dark:text-gray-300">
        <CaseLink id={result.caseId} /> · {t(`ask.modality.${result.modality}`, result.modality)} · {t(`ask.caseType.${result.caseType}`, result.caseType)}
      </p>
      <PersonsTable persons={result.persons} />
      {result.occurrence ? (
        <>
          <p className="text-sm font-medium text-gray-800 dark:text-white/90">{occurrenceLabel(result.occurrence)}</p>
          <OccurrenceFacts occurrence={result.occurrence} />
          <Points points={pointsOf([result.occurrence])} />
        </>
      ) : (
        <p className="text-xs text-gray-500 dark:text-gray-400">{t("ask.notLinked")}</p>
      )}
      {result.requests.length > 0 && (
        <Table head={[t("ask.request"), t("ask.authority"), t("ask.receivedOn"), t("ask.dueOn")]}>
          {result.requests.map((r) => (
            <tr key={r.sequence} className={rowClass}>
              <td className={cellClass}>{r.reference}</td>
              <td className={cellClass}>{r.authority}</td>
              <td className={`${cellClass} whitespace-nowrap`}>{r.receivedOn}</td>
              <td className={`${cellClass} whitespace-nowrap`}>{r.dueOn ?? ""}</td>
            </tr>
          ))}
        </Table>
      )}
      {result.reports.length > 0 && (
        <Table head={[t("ask.report"), t("ask.reportType"), t("ask.issuedOn")]}>
          {result.reports.map((r, i) => (
            <tr key={i} className={rowClass}>
              <td className={`${cellClass} whitespace-nowrap`}>{r.number ?? ""}</td>
              <td className={cellClass}>{r.type}</td>
              <td className={`${cellClass} whitespace-nowrap`}>{r.issuedOn ?? ""}</td>
            </tr>
          ))}
        </Table>
      )}
    </div>
  );
}

export function OccurrenceSearchResult({ result }: { result: AskOccurrenceSearch }) {
  const { t } = useTranslation();
  if (result.rows.length === 0) return <p className="text-sm text-gray-500 dark:text-gray-400">{t("ask.noneFound")}</p>;
  return (
    <Table head={[t("ask.occurrence"), "IPL", t("ask.date"), t("ask.types"), t("ask.place")]}>
      {result.rows.map((o) => (
        <tr key={o.id} className={rowClass}>
          <td className={`${cellClass} whitespace-nowrap`}>{occurrenceLabel(o)}</td>
          <td className={`${cellClass} whitespace-nowrap`}>{o.inquiryNumber ?? ""}</td>
          <td className={`${cellClass} whitespace-nowrap`}>{o.occurredFrom ?? t("ask.notRecorded")}</td>
          <td className={cellClass}>{o.types.map((code) => t(`occurrenceType.${code}`, code)).join(" · ")}</td>
          <td className={cellClass}>{[o.placeName, o.neighbourhood].filter(Boolean).join(" · ")}</td>
        </tr>
      ))}
    </Table>
  );
}

export function NearResult({ result }: { result: AskNear }) {
  const { t, i18n } = useTranslation();
  // The centre last, so it stays on top of an occurrence at the same point.
  const points: AskPoint[] = [
    ...result.rows.map((o) => ({ lat: o.lat, lon: o.lon, label: occurrenceLabel(o) })),
    { lat: result.centerLat, lon: result.centerLon, label: t("ask.searchCenter"), center: true },
  ];
  return (
    <div className="flex flex-col gap-2">
      {result.municipalityOnlyLeftOut > 0 && (
        <p className="text-xs text-warning-600 dark:text-warning-400">{t("ask.municipalityOnlyLeftOut", { count: result.municipalityOnlyLeftOut })}</p>
      )}
      {result.undatedLeftOut > 0 && <p className="text-xs text-warning-600 dark:text-warning-400">{t("ask.undatedExcluded", { count: result.undatedLeftOut })}</p>}
      {result.rows.length > 0 ? (
        <Table head={[t("ask.occurrence"), t("ask.distance"), t("ask.date"), t("ask.types"), t("ask.place"), t("ask.cases")]}>
          {result.rows.map((o) => (
            <tr key={o.id} className={rowClass}>
              <td className={`${cellClass} whitespace-nowrap`}>{occurrenceLabel(o)}</td>
              <td className={`${cellClass} whitespace-nowrap tabular-nums`}>{o.distanceM.toLocaleString(i18n.language)} m</td>
              <td className={`${cellClass} whitespace-nowrap`}>{o.occurredFrom ?? t("ask.notRecorded")}</td>
              <td className={cellClass}>{o.types.map((code) => t(`occurrenceType.${code}`, code)).join(" · ")}</td>
              <td className={cellClass}>{[o.placeName, o.neighbourhood].filter(Boolean).join(" · ")}</td>
              <td className={`${cellClass} tabular-nums`}>{o.linkedCases}</td>
            </tr>
          ))}
        </Table>
      ) : (
        <p className="text-sm text-gray-500 dark:text-gray-400">{t("ask.noneFound")}</p>
      )}
      <Points points={points} />
    </div>
  );
}
