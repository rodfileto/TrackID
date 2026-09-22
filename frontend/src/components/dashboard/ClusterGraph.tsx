import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import ForceGraph2D, {
  type ForceGraphMethods,
  type LinkObject,
  type NodeObject,
} from "react-force-graph-2d";
import Badge from "../ui/badge/Badge";
import {
  getClusterGraph,
  type ClusterGraph as ClusterGraphData,
  type ClusterGraphNode,
  type GraphNodeKind,
} from "../../services/clusters";

type GNode = NodeObject<ClusterGraphNode>;
type GLink = LinkObject<ClusterGraphNode>;

// Neo4j-Browser-like palette: one colour per node label.
const KIND_COLOR: Record<GraphNodeKind, string> = {
  person: "#f79767",
  cluster: "#57c7e3",
  trace: "#8dcc93",
};
const KIND_LABEL_KEY: Record<GraphNodeKind, string> = {
  person: "graph.kind.person",
  cluster: "graph.kind.cluster",
  trace: "graph.kind.trace",
};
const KIND_RADIUS: Record<GraphNodeKind, number> = {
  person: 11,
  cluster: 8,
  trace: 6,
};

function nodeRadius(node: ClusterGraphNode): number {
  const base = KIND_RADIUS[node.kind];
  return node.kind === "cluster"
    ? base + Math.min(6, (node.size ?? 0) / 4)
    : base;
}

function endId(end: GLink["source"]): string {
  return typeof end === "object" ? String((end as GNode).id) : String(end);
}

// Illustrative home-page view of how enrolled identities connect to case
// traces through shared biometric clusters, drawn like the Neo4j browser.
export default function ClusterGraph() {
  const { t } = useTranslation();
  const wrapRef = useRef<HTMLDivElement>(null);
  const fgRef = useRef<ForceGraphMethods<GNode, GLink> | undefined>(undefined);
  const [data, setData] = useState<ClusterGraphData | null>(null);
  const [error, setError] = useState("");
  const [width, setWidth] = useState(800);
  const [hover, setHover] = useState<GNode | null>(null);
  const [selected, setSelected] = useState<ClusterGraphNode | null>(null);
  const [limit, setLimit] = useState(40);
  const [minMembers, setMinMembers] = useState(2);

  useEffect(() => {
    let stale = false;
    getClusterGraph(limit, minMembers)
      .then((g) => {
        if (stale) return;
        setError("");
        setData(g);
      })
      .catch((e: Error) => !stale && setError(e.message));
    return () => {
      stale = true;
    };
  }, [limit, minMembers]);

  useEffect(() => {
    const el = wrapRef.current;
    if (!el) return;
    const observer = new ResizeObserver(() => setWidth(el.clientWidth));
    observer.observe(el);
    setWidth(el.clientWidth);
    return () => observer.disconnect();
  }, []);

  const graphData = useMemo(
    () =>
      data && {
        nodes: data.nodes.map((n) => ({ ...n })),
        links: data.edges.map((e) => ({ ...e })),
      },
    [data],
  );

  // Nodes adjacent to the hovered one stay lit; the rest fade back.
  const lit = useMemo(() => {
    const ids = new Set<string>();
    if (hover && data) {
      ids.add(hover.id as string);
      for (const e of data.edges) {
        if (e.source === hover.id) ids.add(e.target);
        if (e.target === hover.id) ids.add(e.source);
      }
    }
    return ids;
  }, [hover, data]);

  const neighbours = useMemo(() => {
    if (!selected || !data) return [];
    const byId = new Map(data.nodes.map((n) => [n.id, n]));
    const out: ClusterGraphNode[] = [];
    for (const e of data.edges) {
      const other =
        e.source === selected.id
          ? e.target
          : e.target === selected.id
            ? e.source
            : null;
      const node = other && byId.get(other);
      if (node) out.push(node);
    }
    return out;
  }, [selected, data]);

  const dim = (id: string) => hover != null && !lit.has(id);

  useEffect(() => {
    const fg = fgRef.current;
    if (!fg) return;
    fg.d3Force("charge")?.strength(-120);
    fg.d3Force("link")?.distance(40);
  }, [graphData]);

  return (
    <div className="grid grid-cols-12 gap-4 md:gap-6">
      <div className="col-span-12 overflow-hidden rounded-2xl border border-gray-200 bg-white dark:border-gray-800 dark:bg-white/[0.03] xl:col-span-8">
        <div className="flex flex-wrap items-center justify-between gap-3 px-5 py-4 sm:px-6">
          <div>
            <h3 className="text-lg font-semibold text-gray-800 dark:text-white/90">
              {t("graph.title")}
            </h3>
            <p className="text-theme-sm text-gray-500 dark:text-gray-400">
              {t("graph.subtitle")}
            </p>
          </div>
          <div className="flex items-center gap-4 text-theme-xs text-gray-600 dark:text-gray-300">
            <label className="flex items-center gap-2">
              {t("graph.clusters")}
              <select
                value={limit}
                onChange={(e) => setLimit(Number(e.target.value))}
                className="rounded-lg border border-gray-300 bg-transparent px-2 py-1 dark:border-gray-700 dark:bg-gray-900"
              >
                {[20, 40, 80, 150, 300].map((n) => (
                  <option key={n} value={n}>
                    {n}
                  </option>
                ))}
              </select>
            </label>
            <label className="flex items-center gap-2">
              {t("graph.minElements")}
              <input
                type="number"
                min={1}
                value={minMembers}
                onChange={(e) =>
                  setMinMembers(Math.max(1, Number(e.target.value) || 1))
                }
                className="w-16 rounded-lg border border-gray-300 bg-transparent px-2 py-1 dark:border-gray-700"
              />
            </label>
          </div>
          <ul className="flex items-center gap-4">
            {(Object.keys(KIND_COLOR) as GraphNodeKind[]).map((kind) => (
              <li
                key={kind}
                className="flex items-center gap-1.5 text-theme-xs text-gray-600 dark:text-gray-300"
              >
                <span
                  className="inline-block size-2.5 rounded-full"
                  style={{ background: KIND_COLOR[kind] }}
                />
                {t(KIND_LABEL_KEY[kind])}
              </li>
            ))}
          </ul>
        </div>

        <div ref={wrapRef} className="relative h-[520px] bg-[#1b2030]">
          {error && (
            <p className="absolute inset-0 flex items-center justify-center text-sm text-error-400">
              {error}
            </p>
          )}
          {!error && !data && (
            <p className="absolute inset-0 flex items-center justify-center text-sm text-gray-400">
              {t("graph.loading")}
            </p>
          )}
          {data && data.nodes.length === 0 && (
            <p className="absolute inset-0 flex items-center justify-center text-sm text-gray-400">
              {t("clusters.emptyShort")}
            </p>
          )}
          {graphData && graphData.nodes.length > 0 && (
            <ForceGraph2D<ClusterGraphNode>
              ref={fgRef}
              width={width}
              height={520}
              graphData={graphData}
              backgroundColor="#1b2030"
              cooldownTicks={120}
              onEngineStop={() => fgRef.current?.zoomToFit(400, 40)}
              nodeRelSize={1}
              nodeVal={(n) => nodeRadius(n as ClusterGraphNode) ** 2}
              onNodeHover={(n) => {
                setHover(n);
                if (n) setSelected(n as ClusterGraphNode);
              }}
              linkColor={(l) =>
                dim(endId(l.source)) || dim(endId(l.target))
                  ? "rgba(165,171,182,0.12)"
                  : "rgba(165,171,182,0.55)"
              }
              linkWidth={1.2}
              linkDirectionalParticles={2}
              linkDirectionalParticleWidth={(l) =>
                hover &&
                (endId(l.source) === hover.id || endId(l.target) === hover.id)
                  ? 3
                  : 0
              }
              linkDirectionalParticleColor={() => "#ffffff"}
              nodeCanvasObject={(node, ctx, scale) => {
                const r = nodeRadius(node);
                const faded = dim(node.id as string);
                const color = KIND_COLOR[node.kind];
                ctx.globalAlpha = faded ? 0.2 : 1;

                ctx.beginPath();
                ctx.arc(node.x ?? 0, node.y ?? 0, r, 0, 2 * Math.PI);
                ctx.fillStyle = color;
                ctx.shadowColor = color;
                ctx.shadowBlur = hover?.id === node.id ? 18 : 6;
                ctx.fill();
                ctx.shadowBlur = 0;
                ctx.lineWidth = 1.5;
                ctx.strokeStyle = "rgba(255,255,255,0.75)";
                ctx.stroke();

                // Labels for identities always, others when zoomed in or hovered.
                if (node.kind === "person" || scale > 2) {
                  const fontSize = Math.max(10 / scale, 3);
                  ctx.font = `${fontSize}px sans-serif`;
                  ctx.textAlign = "center";
                  ctx.textBaseline = "top";
                  ctx.fillStyle = "#e6e8ee";
                  ctx.fillText(node.label, node.x ?? 0, (node.y ?? 0) + r + 2);
                }
                ctx.globalAlpha = 1;
              }}
              nodePointerAreaPaint={(node, color, ctx) => {
                ctx.fillStyle = color;
                ctx.beginPath();
                ctx.arc(
                  node.x ?? 0,
                  node.y ?? 0,
                  nodeRadius(node) + 2,
                  0,
                  2 * Math.PI,
                );
                ctx.fill();
              }}
            />
          )}
        </div>
      </div>

      <div className="col-span-12 xl:col-span-4">
        <NodeDetailCard node={selected} neighbours={neighbours} />
      </div>
    </div>
  );
}

function NodeDetailCard({
  node,
  neighbours,
}: {
  node: ClusterGraphNode | null;
  neighbours: ClusterGraphNode[];
}) {
  const { t } = useTranslation();
  // Same shell as the template's metric cards (EcommerceMetrics): icon tile,
  // muted label, bold value, Badge.
  const shell =
    "h-full rounded-2xl border border-gray-200 bg-white p-5 dark:border-gray-800 dark:bg-white/[0.03] md:p-6";

  if (!node) {
    return (
      <div
        className={`${shell} flex flex-col items-center justify-center text-center`}
      >
        <span className="flex size-12 items-center justify-center rounded-xl bg-gray-100 dark:bg-gray-800">
          <span className="size-3 rounded-full bg-gray-400" />
        </span>
        <p className="mt-4 text-sm text-gray-500 dark:text-gray-400">
          {t("graph.hint")}
        </p>
      </div>
    );
  }

  const color = KIND_COLOR[node.kind];
  const link =
    node.kind === "person" && node.personId
      ? {
          to: `/persons/${encodeURIComponent(node.personId)}`,
          text: t("graph.openPerson"),
        }
      : node.kind === "trace" && node.caseId
        ? { to: `/cases/${encodeURIComponent(node.caseId)}`, text: t("graph.openCase") }
        : null;

  const rows: [string, string][] = [];
  if (node.kind === "cluster") rows.push([t("clusters.columns.members"), String(node.size ?? 0)]);
  if (node.modality)
    rows.push([
      t("graph.modality"),
      node.modality === "FACIAL"
        ? t("modality.facial")
        : t("modality.fingerprint"),
    ]);
  if (node.caseId) rows.push([t("caseDetail.case"), node.caseId]);
  rows.push([t("graph.connections"), String(neighbours.length)]);

  return (
    <div className={shell}>
      <div className="flex items-start justify-between">
        <span
          className="flex size-12 items-center justify-center rounded-xl"
          style={{ background: `${color}33` }}
        >
          <span
            className="size-4 rounded-full"
            style={{ background: color, boxShadow: `0 0 10px ${color}` }}
          />
        </span>
        <Badge
          color={
            node.kind === "person"
              ? "warning"
              : node.kind === "cluster"
                ? "info"
                : "success"
          }
        >
          {t(KIND_LABEL_KEY[node.kind])}
        </Badge>
      </div>

      <h4 className="mt-5 break-words text-title-sm font-bold text-gray-800 dark:text-white/90">
        {node.label}
      </h4>

      <dl className="mt-4 divide-y divide-gray-100 text-sm dark:divide-gray-800">
        {rows.map(([k, v]) => (
          <div key={k} className="flex justify-between py-2">
            <dt className="text-gray-500 dark:text-gray-400">{k}</dt>
            <dd className="font-medium text-gray-800 dark:text-white/90">
              {v}
            </dd>
          </div>
        ))}
      </dl>

      {neighbours.length > 0 && (
        <div className="mt-4">
          <p className="mb-2 text-theme-xs uppercase tracking-wide text-gray-500 dark:text-gray-400">
            {t("graph.linkedTo")}
          </p>
          <ul className="max-h-40 space-y-1.5 overflow-auto">
            {neighbours.map((n) => (
              <li
                key={n.id}
                className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300"
              >
                <span
                  className="size-2 shrink-0 rounded-full"
                  style={{ background: KIND_COLOR[n.kind] }}
                />
                <span className="truncate">{n.label}</span>
              </li>
            ))}
          </ul>
        </div>
      )}

      {link && (
        <Link
          to={link.to}
          // window.open (unlike target=_blank, which implies noopener) makes the
          // browser copy this tab's sessionStorage into the new one, so a login
          // made without "keep me logged in" carries over. Same-origin only.
          onClick={(e) => {
            e.preventDefault();
            window.open(link.to, "_blank");
          }}
          className="mt-5 inline-flex w-full items-center justify-center rounded-lg bg-brand-500 px-4 py-2.5 text-sm font-medium text-white hover:bg-brand-600"
        >
          {link.text}
        </Link>
      )}
    </div>
  );
}
