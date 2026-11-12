import { Fragment, useEffect, useMemo, useRef, useState } from "react";
import { Link, useParams } from "react-router";
import BackLink from "../components/common/BackLink";
import FollowButton from "../components/common/FollowButton";
import { useTranslation } from "react-i18next";
import ForceGraph2D, {
  type ForceGraphMethods,
  type LinkObject,
  type NodeObject,
} from "react-force-graph-2d";
import PageMeta from "../components/common/PageMeta";
import PageBreadcrumb from "../components/common/PageBreadCrumb";
import ComponentCard from "../components/common/ComponentCard";
import PresenceMapCard from "../components/intelligence/PresenceMapCard";
import { getClusterOccurrences } from "../services/occurrences";
import {
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableRow,
} from "../components/ui/table";
import Badge from "../components/ui/badge/Badge";
import IdentityFileThumbnail from "../components/persons/IdentityFileThumbnail";
import RegisterFacts from "../components/persons/RegisterFacts";
import { IdentityConflictList } from "../components/clusters/IdentityConflicts";
import CaseTraceThumbnail from "../components/cases/CaseTraceThumbnail";
import IntelligencePanels from "../components/common/IntelligencePanels";
import { extension } from "../extensions";
import {
  getCluster,
  type ClusterDetail,
  type ClusterPair,
  type ClusterPersonSample,
  type EdgeStatus,
} from "../services/clusters";
import { personPath, type PersonClusterMember } from "../services/persons";

const PAIRS_PAGE = 50;
const GRAPH_HEIGHT = 420;
const linkClass =
  "font-medium text-brand-500 hover:text-brand-600 dark:text-brand-400 dark:hover:text-brand-300";

function modalityLabelKey(modality: string): string {
  return modality === "FACIAL" ? "modality.facial" : "modality.fingerprint";
}

function statusColor(status: EdgeStatus): "success" | "error" | "warning" | "light" {
  switch (status) {
    case "CONFIRMED":
      return "success";
    case "REJECTED":
      return "error";
    case "DISPUTED":
      return "warning";
    default:
      return "light";
  }
}

function score(value?: number): string {
  return value === undefined ? "—" : value.toFixed(3);
}

/** A member's short label: the person and register of an enrollment, the case and trace of
 * case evidence; the bare feature id of a sample outside the cluster. */
function memberLabel(member: PersonClusterMember | undefined, featureId: string): string {
  if (!member) return featureId;
  if (member.kind === "KNOWN") return `${member.name ?? member.personId} · ${member.registerNumber ?? ""}`;
  return `${member.caseId} · #${member.traceId}`;
}

function MemberThumbnail({ member }: { member: PersonClusterMember }) {
  const { t } = useTranslation();
  if (member.kind === "KNOWN") {
    return (
      <IdentityFileThumbnail
        personId={member.personId ?? ""}
        identityFileId={member.identityFileId ?? 0}
        contentType={member.contentType}
        alt={member.name ?? ""}
      />
    );
  }
  return (
    <CaseTraceThumbnail
      caseId={member.caseId ?? ""}
      thumbnailFileId={member.thumbnailFileId}
      thumbnailBox={member.thumbnailBox}
      alt={t("personLookup.traceAlt", { traceId: member.traceId, caseId: member.caseId })}
    />
  );
}

function MemberLink({ member, featureId }: { member?: PersonClusterMember; featureId: string }) {
  if (!member) return <span className="font-mono text-xs">{featureId}</span>;
  if (member.kind === "KNOWN") {
    return (
      <Link to={personPath(member.personNumber)} className={linkClass}>
        {memberLabel(member, featureId)}
      </Link>
    );
  }
  return (
    <Link to={`/cases/${encodeURIComponent(member.caseId ?? "")}`} className={linkClass}>
      {memberLabel(member, featureId)}
    </Link>
  );
}

function ClusterLink({ id, current }: { id?: number; current?: number }) {
  const { t } = useTranslation();
  if (!id) return <span className="text-gray-400 dark:text-white/30">—</span>;
  if (id === current) {
    return (
      <Badge size="sm" color="primary">
        {t("clusterDetail.thisCluster")}
      </Badge>
    );
  }
  return (
    <Link to={`/clusters/${id}`} className={linkClass}>
      #{id}
    </Link>
  );
}

function Stat({ label, value, tone }: { label: string; value: number | string; tone?: "warn" }) {
  return (
    <div className="rounded-xl border border-gray-200 px-4 py-3 dark:border-white/[0.05]">
      <p className="text-xs text-gray-500 dark:text-gray-400">{label}</p>
      <p
        className={`mt-1 text-xl font-semibold ${
          tone === "warn" ? "text-warning-600 dark:text-warning-400" : "text-gray-800 dark:text-white/90"
        }`}
      >
        {value}
      </p>
    </div>
  );
}

function Flag({ tone, children }: { tone: "warning" | "error" | "info"; children: React.ReactNode }) {
  const tones = {
    warning: "border-warning-500 bg-warning-50 dark:border-warning-500/30 dark:bg-warning-500/15",
    error: "border-error-500 bg-error-50 dark:border-error-500/30 dark:bg-error-500/15",
    info: "border-blue-light-500 bg-blue-light-50 dark:border-blue-light-500/30 dark:bg-blue-light-500/15",
  };
  return (
    <div className={`rounded-xl border p-4 text-sm text-gray-700 dark:text-gray-300 ${tones[tone]}`}>
      {children}
    </div>
  );
}

/** The persons the cluster resolves to, each with every enrollment sample, of any modality, and
 * the cluster each one is in: a person whose samples of the cluster's modality sit in different
 * clusters holds biometrics that did not match each other. */
function PersonsCard({ detail }: { detail: ClusterDetail }) {
  const { t } = useTranslation();
  const byPerson = useMemo(() => {
    const out = new Map<string, ClusterPersonSample[]>();
    for (const s of detail.personSamples) {
      out.set(s.personId, [...(out.get(s.personId) ?? []), s]);
    }
    return [...out.entries()];
  }, [detail.personSamples]);

  return (
    <ComponentCard title={t("clusterDetail.persons")} desc={t("clusterDetail.personsHint")}>
      {byPerson.length === 0 ? (
        <p className="text-sm text-gray-400 dark:text-white/30">{t("clusterDetail.noPersons")}</p>
      ) : (
        <div className="space-y-4">
          {byPerson.map(([personId, samples]) => {
            const elsewhere = samples.some(
              (s) => s.modality === detail.modality && s.clusterId !== detail.clusterId,
            );
            return (
              <div key={personId} className="rounded-xl border border-gray-200 p-4 dark:border-white/[0.05]">
                <div className="mb-3 flex flex-wrap items-center gap-2">
                  <Link to={personPath(samples[0].personNumber)} className={linkClass}>
                    {samples[0].name}
                  </Link>
                  <span className="text-xs text-gray-500 dark:text-gray-400">{personId}</span>
                  {/* On the reviewed layer an unreviewed sample sits alone: that says nothing yet. */}
                  {elsewhere && detail.basis === "AUTOMATIC" && (
                    <Badge size="sm" color="warning">
                      {t("clusterDetail.samplesElsewhere")}
                    </Badge>
                  )}
                </div>
                <div className="mb-3 flex flex-col gap-3">
                  {[...new Map(samples.map((s) => [s.registerNumber, s])).values()].map((s) => (
                    <div key={s.registerNumber}>
                      <p className="mb-1 text-xs text-gray-500 dark:text-gray-400">
                        {s.registerNumber} · <span className="font-medium text-gray-800 dark:text-white/90">{s.name}</span>
                      </p>
                      <RegisterFacts birthDate={s.birthDate} parent1Name={s.parent1Name} parent2Name={s.parent2Name} />
                    </div>
                  ))}
                </div>
                <ul className="flex flex-wrap gap-3">
                  {samples.map((s) => (
                    <li
                      key={s.identityFileId}
                      className="flex items-center gap-2 rounded-lg border border-gray-100 py-1.5 pl-1.5 pr-3 dark:border-white/[0.05]"
                    >
                      <IdentityFileThumbnail
                        personId={s.personId}
                        identityFileId={s.identityFileId}
                        contentType={s.contentType}
                        alt={`${s.name} · ${s.registerNumber}`}
                      />
                      <div className="text-xs">
                        <p className="text-gray-700 dark:text-gray-300">{s.registerNumber}</p>
                        <p className="mt-0.5 text-gray-500 dark:text-gray-400">
                          {s.fileType} · {t(modalityLabelKey(s.modality))} ·{" "}
                          <ClusterLink id={s.clusterId} current={detail.clusterId} />
                        </p>
                      </div>
                    </li>
                  ))}
                </ul>
              </div>
            );
          })}
        </div>
      )}
    </ComponentCard>
  );
}

/** The other modality's clusters holding the same persons' enrollment samples: a reference, not a
 * merge -- the link is the enrollment, and each cluster stands on its own decisions. */
function OtherModalitiesCard({ detail }: { detail: ClusterDetail }) {
  const { t } = useTranslation();
  const names = useMemo(
    () => new Map(detail.personSamples.map((s) => [s.personId, s.name])),
    [detail.personSamples],
  );
  if (detail.linkedClusters.length === 0) return null;
  return (
    <ComponentCard title={t("clusterDetail.otherModalities")} desc={t("clusterDetail.otherModalitiesHint")}>
      <ul className="space-y-2 text-sm text-gray-700 dark:text-gray-300">
        {detail.linkedClusters.map((c) => (
          <li key={c.clusterId} className="flex flex-wrap items-center gap-2">
            <ClusterLink id={c.clusterId} />
            <Badge size="sm" color={c.modality === "FACIAL" ? "info" : "warning"}>
              {t(modalityLabelKey(c.modality))}
            </Badge>
            <span className="text-xs text-gray-500 dark:text-gray-400">
              {t("clusterDetail.linkedCluster.members", { count: c.memberCount })}
              {c.questionedCount > 0 && ` · ${t("clusterDetail.linkedCluster.traces", { count: c.questionedCount })}`}
              {" · "}
              {t("clusterDetail.linkedCluster.via", {
                names: c.personIds.map((id) => names.get(id) ?? id).join(", "),
              })}
            </span>
          </li>
        ))}
      </ul>
    </ComponentCard>
  );
}

type GNode = NodeObject<{ id: string; kind: string; label: string; cut: boolean }>;
type GLink = LinkObject<GNode> & { bridge: boolean; score?: number };

function endId(end: GLink["source"]): string {
  return typeof end === "object" ? String((end as GNode).id) : String(end);
}

/** The cluster's members joined by its edges: enrollments orange, case evidence green, bridges
 * red. Clicking a member filters the pairs below to it. */
function StructureGraph({
  detail,
  resolved,
  selected,
  onSelect,
}: {
  detail: ClusterDetail;
  resolved: Map<string, PersonClusterMember>;
  selected: string | null;
  onSelect: (featureId: string | null) => void;
}) {
  const { t } = useTranslation();
  const wrapRef = useRef<HTMLDivElement>(null);
  const fgRef = useRef<ForceGraphMethods<GNode, GLink> | undefined>(undefined);
  const [width, setWidth] = useState(800);

  useEffect(() => {
    const el = wrapRef.current;
    if (!el) return;
    const ro = new ResizeObserver(([entry]) => setWidth(entry.contentRect.width));
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const data = useMemo(
    () => ({
      nodes: detail.members.map<GNode>((m) => {
        const r = resolved.get(m.featureId);
        return { id: m.featureId, kind: r?.kind ?? "QUESTIONED", label: memberLabel(r, m.featureId), cut: m.cutVertex };
      }),
      links: detail.pairs
        .filter((p) => p.edge)
        .map<GLink>((p) => ({ source: p.featureA, target: p.featureB, bridge: p.bridge, score: p.score })),
    }),
    [detail, resolved],
  );

  return (
    <ComponentCard title={t("clusterDetail.structure")} desc={t("clusterDetail.structureHint")}>
      <div ref={wrapRef} className="relative overflow-hidden rounded-xl bg-[#1b2030]" style={{ height: GRAPH_HEIGHT }}>
        <ForceGraph2D<GNode, GLink>
          ref={fgRef}
          width={width}
          height={GRAPH_HEIGHT}
          graphData={data}
          backgroundColor="#1b2030"
          cooldownTicks={120}
          onEngineStop={() => fgRef.current?.zoomToFit(400, 30)}
          nodeLabel={(n) => n.label}
          linkLabel={(l) => score(l.score)}
          linkColor={(l) => {
            if (l.bridge) return "#f04438";
            if (selected && (endId(l.source) === selected || endId(l.target) === selected)) return "#ffffff";
            return "rgba(165,171,182,0.35)";
          }}
          linkWidth={(l) => (l.bridge ? 3 : 1)}
          onNodeClick={(n) => onSelect(selected === n.id ? null : String(n.id))}
          onBackgroundClick={() => onSelect(null)}
          nodeCanvasObject={(node, ctx) => {
            const r = node.kind === "KNOWN" ? 7 : 5;
            ctx.beginPath();
            ctx.arc(node.x ?? 0, node.y ?? 0, r, 0, 2 * Math.PI);
            ctx.fillStyle = node.kind === "KNOWN" ? "#f79767" : "#8dcc93";
            ctx.fill();
            ctx.lineWidth = node.id === selected ? 3 : 1.5;
            ctx.strokeStyle = node.cut ? "#f04438" : node.id === selected ? "#ffffff" : "rgba(255,255,255,0.6)";
            ctx.stroke();
          }}
          nodePointerAreaPaint={(node, color, ctx) => {
            ctx.fillStyle = color;
            ctx.beginPath();
            ctx.arc(node.x ?? 0, node.y ?? 0, 9, 0, 2 * Math.PI);
            ctx.fill();
          }}
        />
        <div className="pointer-events-none absolute bottom-2 left-3 flex flex-wrap gap-3 text-xs text-gray-300">
          <span><span className="mr-1 inline-block size-2.5 rounded-full bg-[#f79767]" />{t("clusterDetail.legendKnown")}</span>
          <span><span className="mr-1 inline-block size-2.5 rounded-full bg-[#8dcc93]" />{t("clusterDetail.legendQuestioned")}</span>
          <span><span className="mr-1 inline-block h-0.5 w-4 bg-[#f04438] align-middle" />{t("clusterDetail.legendBridge")}</span>
        </div>
      </div>
    </ComponentCard>
  );
}

function MembersCard({
  detail,
  resolved,
  selected,
  onSelect,
}: {
  detail: ClusterDetail;
  resolved: Map<string, PersonClusterMember>;
  selected: string | null;
  onSelect: (featureId: string | null) => void;
}) {
  const { t } = useTranslation();
  const other = detail.basis === "AUTOMATIC" ? t("clusterDetail.basis.REVIEWED") : t("clusterDetail.basis.AUTOMATIC");
  const members = [...detail.members].sort((a, b) => {
    const ka = resolved.get(a.featureId)?.kind === "KNOWN" ? 0 : 1;
    const kb = resolved.get(b.featureId)?.kind === "KNOWN" ? 0 : 1;
    return ka - kb || a.degree - b.degree;
  });
  const head = "px-4 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400";
  return (
    <ComponentCard title={t("clusterDetail.members")} desc={t("clusterDetail.membersHint")}>
      <div className="overflow-hidden rounded-xl border border-gray-200 dark:border-white/[0.05]">
        <div className="max-w-full overflow-x-auto">
          <Table>
            <TableHeader className="border-b border-gray-100 dark:border-white/[0.05]">
              <TableRow>
                <TableCell isHeader className={head}>{""}</TableCell>
                <TableCell isHeader className={head}>{t("clusterDetail.columns.sample")}</TableCell>
                <TableCell isHeader className={head}>{t("clusterDetail.columns.kind")}</TableCell>
                <TableCell isHeader className={head}>{t("clusterDetail.columns.degree")}</TableCell>
                <TableCell isHeader className={head}>{t("clusterDetail.columns.otherLayer", { layer: other })}</TableCell>
                <TableCell isHeader className={head}>{""}</TableCell>
              </TableRow>
            </TableHeader>
            <TableBody className="divide-y divide-gray-100 dark:divide-white/[0.05]">
              {members.map((m) => {
                const r = resolved.get(m.featureId);
                return (
                  <TableRow
                    key={m.featureId}
                    className={selected === m.featureId ? "bg-brand-50 dark:bg-brand-500/10" : ""}
                  >
                    <TableCell className="px-4 py-2">{r && <MemberThumbnail member={r} />}</TableCell>
                    <TableCell className="px-4 py-2 text-theme-sm">
                      <MemberLink member={r} featureId={m.featureId} />
                      {r?.kind === "QUESTIONED" && r.description && (
                        <p className="mt-0.5 max-w-xs truncate text-xs text-gray-500 dark:text-gray-400">{r.description}</p>
                      )}
                    </TableCell>
                    <TableCell className="px-4 py-2">
                      <Badge size="sm" color={r?.kind === "KNOWN" ? "warning" : "success"}>
                        {r?.kind === "KNOWN" ? t("clusterDetail.known") : t("clusterDetail.questioned")}
                      </Badge>
                    </TableCell>
                    <TableCell className="px-4 py-2 text-theme-sm text-gray-600 dark:text-gray-300">
                      {m.degree}
                      {m.cutVertex && (
                        <span className="ml-2">
                          <Badge size="sm" color="error">{t("clusterDetail.cutVertex")}</Badge>
                        </span>
                      )}
                    </TableCell>
                    <TableCell className="px-4 py-2 text-theme-sm">
                      <ClusterLink id={m.otherClusterId} />
                    </TableCell>
                    <TableCell className="px-4 py-2 text-end">
                      <button
                        type="button"
                        onClick={() => onSelect(selected === m.featureId ? null : m.featureId)}
                        className="text-xs text-brand-500 hover:text-brand-600 dark:text-brand-400"
                      >
                        {selected === m.featureId ? t("clusterDetail.clearFilter") : t("clusterDetail.showPairs")}
                      </button>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </div>
      </div>
    </ComponentCard>
  );
}

type PairFilter = "edges" | "insideOther" | "boundary" | "all";

function PairChain({ pair, resolved }: { pair: ClusterPair; resolved: Map<string, PersonClusterMember> }) {
  const { t, i18n } = useTranslation();
  const a = resolved.get(pair.featureA);
  const b = resolved.get(pair.featureB);
  return (
    <div className="space-y-3 bg-gray-50 px-4 py-3 dark:bg-white/[0.02]">
      <div className="flex flex-wrap items-center gap-6">
        {[pair.featureA, pair.featureB].map((f, i) => {
          const r = i === 0 ? a : b;
          return (
            <div key={f} className="flex items-center gap-2">
              {r ? <MemberThumbnail member={r} /> : null}
              <div className="text-xs">
                <MemberLink member={r} featureId={f} />
                <p className="font-mono text-gray-400">{f}</p>
              </div>
            </div>
          );
        })}
      </div>
      <table className="w-full text-xs">
        <thead>
          <tr className="text-left text-gray-500 dark:text-gray-400">
            <th className="py-1 pr-3 font-medium">{t("clusterDetail.columns.role")}</th>
            <th className="py-1 pr-3 font-medium">{t("clusterDetail.columns.decision")}</th>
            <th className="py-1 pr-3 font-medium">{t("clusterDetail.columns.decidedBy")}</th>
            <th className="py-1 pr-3 font-medium">{t("clusterDetail.columns.score")}</th>
            <th className="py-1 pr-3 font-medium">{t("clusterDetail.columns.analysis")}</th>
            <th className="py-1 pr-3 font-medium">{t("clusterDetail.columns.conclusion")}</th>
            <th className="py-1 font-medium">{t("clusterDetail.columns.decidedAt")}</th>
          </tr>
        </thead>
        <tbody className="text-gray-700 dark:text-gray-300">
          {pair.chain.map((d, i) => (
            <tr key={i} className="border-t border-gray-200 dark:border-white/[0.05]">
              <td className="py-1 pr-3">{d.role}</td>
              <td className="py-1 pr-3">{d.decision}</td>
              <td className="py-1 pr-3">{d.decidedBy}</td>
              <td className="py-1 pr-3">
                {d.confidence !== undefined ? `${score(d.confidence)} / ${score(d.threshold)}` : "—"}
              </td>
              <td className="py-1 pr-3">{d.analysisId ? `#${d.analysisId} ${d.analysisType ?? ""}` : "—"}</td>
              <td className="py-1 pr-3">{d.conclusion ? t(`analyses.conclusion.${d.conclusion}`) : "—"}</td>
              <td className="py-1">{new Date(d.decidedAt).toLocaleString(i18n.resolvedLanguage ?? "en")}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/** Every decided pair touching the cluster, weakest score first: its edges, the pairs inside it
 * that are not edges (inconclusive, disputed, rejected, or confirmed by SYSTEM alone in the
 * reviewed layer), and the boundary pairs to samples outside it. */
function PairsCard({
  detail,
  resolved,
  selected,
  onSelect,
}: {
  detail: ClusterDetail;
  resolved: Map<string, PersonClusterMember>;
  selected: string | null;
  onSelect: (featureId: string | null) => void;
}) {
  const { t } = useTranslation();
  const [filter, setFilter] = useState<PairFilter>("edges");
  const [bridgesOnly, setBridgesOnly] = useState(false);
  const [open, setOpen] = useState<string | null>(null);
  const [shown, setShown] = useState(PAIRS_PAGE);

  const counts = useMemo(
    () => ({
      edges: detail.pairs.filter((p) => p.edge).length,
      insideOther: detail.pairs.filter((p) => p.inside && !p.edge).length,
      boundary: detail.pairs.filter((p) => !p.inside).length,
      all: detail.pairs.length,
    }),
    [detail.pairs],
  );

  const memberSet = useMemo(() => new Set(detail.members.map((m) => m.featureId)), [detail.members]);
  const pairs = detail.pairs.filter((p) => {
    if (selected && p.featureA !== selected && p.featureB !== selected) return false;
    if (bridgesOnly && !p.bridge) return false;
    switch (filter) {
      case "edges":
        return p.edge;
      case "insideOther":
        return p.inside && !p.edge;
      case "boundary":
        return !p.inside;
      default:
        return true;
    }
  });

  const head = "px-4 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400";
  const tab = (active: boolean) =>
    `inline-flex h-9 items-center rounded-lg border px-3 text-sm font-medium transition-colors ${
      active
        ? "border-brand-500 bg-brand-500 text-white"
        : "border-gray-200 bg-white text-gray-700 hover:bg-gray-50 dark:border-gray-800 dark:bg-gray-900 dark:text-gray-400 dark:hover:bg-white/[0.03]"
    }`;

  return (
    <ComponentCard title={t("clusterDetail.pairs")} desc={t("clusterDetail.pairsHint")}>
      <div className="flex flex-wrap items-center gap-2">
        {(["edges", "insideOther", "boundary", "all"] as PairFilter[]).map((f) => (
          <button
            key={f}
            type="button"
            className={tab(filter === f)}
            aria-pressed={filter === f}
            onClick={() => {
              setFilter(f);
              setShown(PAIRS_PAGE);
            }}
          >
            {t(`clusterDetail.filter.${f}`)} ({counts[f]})
          </button>
        ))}
        <button type="button" className={tab(bridgesOnly)} aria-pressed={bridgesOnly} onClick={() => setBridgesOnly(!bridgesOnly)}>
          {t("clusterDetail.filter.bridges")}
        </button>
        {selected && (
          <button type="button" className={tab(true)} onClick={() => onSelect(null)}>
            {memberLabel(resolved.get(selected), selected)} ✕
          </button>
        )}
      </div>

      <div className="overflow-hidden rounded-xl border border-gray-200 dark:border-white/[0.05]">
        <div className="max-w-full overflow-x-auto">
          <Table>
            <TableHeader className="border-b border-gray-100 dark:border-white/[0.05]">
              <TableRow>
                <TableCell isHeader className={head}>{t("clusterDetail.columns.sampleA")}</TableCell>
                <TableCell isHeader className={head}>{t("clusterDetail.columns.sampleB")}</TableCell>
                <TableCell isHeader className={head}>{t("clusterDetail.columns.score")}</TableCell>
                <TableCell isHeader className={head}>{t("clusterDetail.columns.status")}</TableCell>
                <TableCell isHeader className={head}>{t("clusterDetail.columns.settledBy")}</TableCell>
                <TableCell isHeader className={head}>{""}</TableCell>
              </TableRow>
            </TableHeader>
            <TableBody className="divide-y divide-gray-100 dark:divide-white/[0.05]">
              {pairs.slice(0, shown).map((p) => {
                const key = `${p.featureA}|${p.featureB}`;
                const outside = !p.inside ? (memberSet.has(p.featureA) ? p.featureB : p.featureA) : null;
                return (
                  <Fragment key={key}>
                    <tr
                      className="cursor-pointer hover:bg-gray-50 dark:hover:bg-white/[0.02]"
                      onClick={() => setOpen(open === key ? null : key)}
                    >
                      {[p.featureA, p.featureB].map((f) => (
                        <TableCell key={f} className="px-4 py-2 text-theme-sm text-gray-700 dark:text-gray-300">
                          {f === outside ? (
                            <span>
                              <span className="font-mono text-xs">{f}</span>{" "}
                              <span className="text-xs text-gray-500">
                                ({t("clusterDetail.outside")} <ClusterLink id={p.outsideClusterId} />)
                              </span>
                            </span>
                          ) : (
                            memberLabel(resolved.get(f), f)
                          )}
                        </TableCell>
                      ))}
                      <TableCell className="px-4 py-2 font-mono text-theme-sm text-gray-700 dark:text-gray-300">
                        {score(p.score)}
                        <span className="text-gray-400"> / {score(p.threshold)}</span>
                      </TableCell>
                      <TableCell className="px-4 py-2">
                        <span className="inline-flex flex-wrap gap-1">
                          <Badge size="sm" color={statusColor(p.status)}>
                            {t(`clusterDetail.status.${p.status}`)}
                          </Badge>
                          {p.bridge && <Badge size="sm" color="error">{t("clusterDetail.bridge")}</Badge>}
                          {p.reviewed && <Badge size="sm" color="info">{t("clusterDetail.reviewed")}</Badge>}
                        </span>
                      </TableCell>
                      <TableCell className="px-4 py-2 text-theme-sm text-gray-500 dark:text-gray-400">
                        {p.settledBy ?? "—"}
                      </TableCell>
                      <TableCell className="px-4 py-2 text-end text-xs text-gray-400">
                        {open === key ? "▲" : "▼"}
                      </TableCell>
                    </tr>
                    {open === key && (
                      <TableRow>
                        <TableCell colSpan={6} className="p-0">
                          <PairChain pair={p} resolved={resolved} />
                        </TableCell>
                      </TableRow>
                    )}
                  </Fragment>
                );
              })}
              {pairs.length === 0 && (
                <TableRow>
                  <TableCell colSpan={6} className="px-5 py-8 text-center text-gray-500 dark:text-gray-400">
                    {t("clusterDetail.noPairs")}
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </div>
      </div>
      {pairs.length > shown && (
        <button
          type="button"
          onClick={() => setShown(shown + PAIRS_PAGE)}
          className="text-sm text-brand-500 hover:text-brand-600 dark:text-brand-400"
        >
          {t("clusterDetail.showMore", { count: pairs.length - shown })}
        </button>
      )}
    </ComponentCard>
  );
}

function MergesCard({ detail }: { detail: ClusterDetail }) {
  const { t, i18n } = useTranslation();
  if (detail.merges.length === 0) return null;
  return (
    <ComponentCard title={t("clusterDetail.merges")} desc={t("clusterDetail.mergesHint")} collapsible defaultCollapsed>
      <ul className="space-y-1 text-sm text-gray-700 dark:text-gray-300">
        {detail.merges.map((m, i) => (
          <li key={i}>
            <ClusterLink id={m.fromClusterId} current={detail.clusterId} /> →{" "}
            <ClusterLink id={m.toClusterId} current={detail.clusterId} />{" "}
            <span className="text-xs text-gray-500 dark:text-gray-400">
              {new Date(m.mergedAt).toLocaleString(i18n.resolvedLanguage ?? "en")}
              {m.reason ? ` · ${m.reason}` : ""}
            </span>
          </li>
        ))}
      </ul>
    </ComponentCard>
  );
}

// One cluster, of either layer, in depth: who it resolves to (and where those persons' other
// samples are), its structure (edges, bridges, cut vertices), every decision touching it and its
// merges -- for checking why a cluster holds what it holds.
export default function ClusterDetailPage() {
  const { t, i18n } = useTranslation();
  const { clusterId } = useParams();
  const id = Number(clusterId);
  const [detail, setDetail] = useState<ClusterDetail | null>(null);
  const [error, setError] = useState("");
  const [selected, setSelected] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setDetail(null);
    setError("");
    setSelected(null);
    getCluster(id)
      .then((d) => {
        if (!cancelled) setDetail(d);
      })
      .catch((err) => {
        if (!cancelled) setError(err instanceof Error ? err.message : t("clusterDetail.loadError"));
      });
    return () => {
      cancelled = true;
    };
  }, [id, t]);

  const resolved = useMemo(
    () => new Map((detail?.resolvedMembers ?? []).map((m) => [m.featureId, m])),
    [detail],
  );

  const title = t("clusterDetail.title", { id: clusterId ?? "" });
  const header = (
    <>
      <PageMeta title={`${title} | TrackID`} description={t("clusterDetail.metaDescription")} />
      <PageBreadcrumb pageTitle={title} />
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <BackLink fallback="/clusters" label={t("clusterDetail.back")} />
        {id > 0 && <FollowButton kind="CLUSTER" id={id} />}
      </div>
    </>
  );

  if (error || !detail) {
    return (
      <>
        {header}
        <p className={`py-12 text-center text-sm ${error ? "text-error-500" : "text-gray-500 dark:text-gray-400"}`}>
          {error || t("common.loading")}
        </p>
      </>
    );
  }

  const known = detail.resolvedMembers.filter((m) => m.kind === "KNOWN").length;
  const cases = new Set(detail.resolvedMembers.filter((m) => m.kind === "QUESTIONED").map((m) => m.caseId)).size;
  const persons = [...new Map(detail.personSamples.map((s) => [s.personId, s.name])).entries()];
  const edges = detail.pairs.filter((p) => p.edge);
  const bridges = edges.filter((p) => p.bridge).length;
  const scores = edges.map((p) => p.score).filter((s): s is number => s !== undefined);
  const weakest = scores.length ? Math.min(...scores) : undefined;
  const reviewedEdges = edges.filter((p) => p.reviewed).length;

  return (
    <>
      {header}
      <div className="space-y-6">
        <ComponentCard
          title={title}
          desc={t(`clusterDetail.basisHint.${detail.basis}`)}
        >
          <div className="flex flex-wrap items-center gap-2 text-sm text-gray-500 dark:text-gray-400">
            <Badge size="sm" color={detail.modality === "FACIAL" ? "info" : "warning"}>
              {t(modalityLabelKey(detail.modality))}
            </Badge>
            <Badge size="sm" color={detail.basis === "REVIEWED" ? "success" : "light"}>
              {t(`clusterDetail.basis.${detail.basis}`)}
            </Badge>
            <span>
              {t("clusterDetail.created", {
                date: new Date(detail.createdAt).toLocaleString(i18n.resolvedLanguage ?? "en"),
              })}
            </span>
          </div>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4 lg:grid-cols-8">
            <Stat label={t("clusterDetail.stats.members")} value={detail.members.length} />
            <Stat label={t("clusterDetail.stats.known")} value={known} />
            <Stat label={t("clusterDetail.stats.questioned")} value={detail.members.length - known} />
            <Stat label={t("clusterDetail.stats.cases")} value={cases} />
            <Stat label={t("clusterDetail.stats.persons")} value={persons.length} tone={detail.conflicts.length > 0 ? "warn" : undefined} />
            <Stat label={t("clusterDetail.stats.edges")} value={`${edges.length} (${reviewedEdges})`} />
            <Stat label={t("clusterDetail.stats.bridges")} value={bridges} tone={bridges > 0 ? "warn" : undefined} />
            <Stat label={t("clusterDetail.stats.weakest")} value={score(weakest)} />
          </div>
          <p className="text-xs text-gray-500 dark:text-gray-400">{t("clusterDetail.edgesNote")}</p>
        </ComponentCard>

        {(bridges > 0 || detail.components > 1) && (
          <div className="space-y-3">
            {detail.components > 1 && (
              <Flag tone="error">{t("clusterDetail.flags.components", { count: detail.components })}</Flag>
            )}
            {bridges > 0 && <Flag tone="info">{t("clusterDetail.flags.bridges", { count: bridges })}</Flag>}
          </div>
        )}

        {detail.conflicts.length > 0 && (
          <ComponentCard title={t("identityConflict.title")} desc={t("identityConflict.desc")}>
            <IdentityConflictList conflicts={detail.conflicts} currentCluster={detail.clusterId} />
          </ComponentCard>
        )}
        <PersonsCard detail={detail} />
        <OtherModalitiesCard detail={detail} />
        <PresenceMapCard subject="cluster" load={() => getClusterOccurrences(detail.clusterId)} />
        <IntelligencePanels panels={extension.clusterPanels} props={{ clusterId: detail.clusterId, modality: detail.modality }} />
        {detail.members.length > 1 && (
          <StructureGraph detail={detail} resolved={resolved} selected={selected} onSelect={setSelected} />
        )}
        <MembersCard detail={detail} resolved={resolved} selected={selected} onSelect={setSelected} />
        <PairsCard detail={detail} resolved={resolved} selected={selected} onSelect={setSelected} />
        <MergesCard detail={detail} />
      </div>
    </>
  );
}
