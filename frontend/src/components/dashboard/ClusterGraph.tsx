import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import ForceGraph2D, {
  type ForceGraphMethods,
  type LinkObject,
  type NodeObject,
} from "react-force-graph-2d";
import {
  getOverviewGraph,
  type ClusterGraph as ClusterGraphData,
  type ClusterGraphNode,
  type GraphNodeKind,
} from "../../services/clusters";

type GNode = NodeObject<ClusterGraphNode>;
type GLink = LinkObject<ClusterGraphNode>;

// One colour per kind of node: identities, biometric clusters, case traces.
const KIND_COLOR: Record<GraphNodeKind, string> = {
  person: "#f79767",
  cluster: "#57c7e3",
  trace: "#8dcc93",
};
const KIND_RADIUS: Record<GraphNodeKind, number> = {
  person: 11,
  cluster: 8,
  trace: 6,
};
const HEIGHT = 520;

function nodeRadius(node: ClusterGraphNode): number {
  const base = KIND_RADIUS[node.kind];
  return node.kind === "cluster" ? base + Math.min(6, (node.size ?? 0) / 4) : base;
}

function endId(end: GLink["source"]): string {
  return typeof end === "object" ? String((end as GNode).id) : String(end);
}

// The home page's illustration of how identities connect to case traces through shared biometric
// clusters: nodes and edges only. The server sends it anonymous (no names, no ids), so every
// signed-in user sees it.
export default function ClusterGraph() {
  const { t } = useTranslation();
  const wrapRef = useRef<HTMLDivElement>(null);
  const fgRef = useRef<ForceGraphMethods<GNode, GLink> | undefined>(undefined);
  const [width, setWidth] = useState(800);
  const [data, setData] = useState<ClusterGraphData | null>(null);
  const [failed, setFailed] = useState(false);
  const [hover, setHover] = useState<GNode | null>(null);

  useEffect(() => {
    getOverviewGraph()
      .then(setData)
      .catch(() => setFailed(true));
  }, []);

  useEffect(() => {
    const el = wrapRef.current;
    if (!el) return;
    const ro = new ResizeObserver(([entry]) => setWidth(entry.contentRect.width));
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const graphData = useMemo(
    () => (data ? { nodes: data.nodes.map((n) => ({ ...n })), links: data.edges.map((e) => ({ ...e })) } : null),
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
  const dim = (id: string) => hover != null && !lit.has(id);

  useEffect(() => {
    const fg = fgRef.current;
    if (!fg) return;
    fg.d3Force("charge")?.strength(-120);
    fg.d3Force("link")?.distance(40);
  }, [graphData]);

  // Nothing to illustrate (no clusters yet, or no database): the card stays out of the way.
  if (failed || (data && data.nodes.length === 0)) return null;

  return (
    <div className="overflow-hidden rounded-2xl border border-gray-200 bg-white dark:border-gray-800 dark:bg-white/[0.03]">
      <div className="px-5 py-4 sm:px-6">
        <h3 className="text-lg font-semibold text-gray-800 dark:text-white/90">{t("graph.title")}</h3>
        <p className="text-theme-sm text-gray-500 dark:text-gray-400">{t("graph.subtitle")}</p>
      </div>
      <div ref={wrapRef} className="relative bg-[#1b2030]" style={{ height: HEIGHT }}>
        {graphData && (
          <ForceGraph2D<ClusterGraphNode>
            ref={fgRef}
            width={width}
            height={HEIGHT}
            graphData={graphData}
            backgroundColor="#1b2030"
            cooldownTicks={120}
            onEngineStop={() => fgRef.current?.zoomToFit(400, 40)}
            nodeRelSize={1}
            nodeVal={(n) => nodeRadius(n as ClusterGraphNode) ** 2}
            nodeLabel={() => ""}
            onNodeHover={(n) => setHover(n)}
            linkColor={(l) =>
              dim(endId(l.source)) || dim(endId(l.target)) ? "rgba(165,171,182,0.12)" : "rgba(165,171,182,0.55)"
            }
            linkWidth={1.2}
            linkDirectionalParticles={2}
            linkDirectionalParticleWidth={(l) =>
              hover && (endId(l.source) === hover.id || endId(l.target) === hover.id) ? 3 : 0
            }
            linkDirectionalParticleColor={() => "#ffffff"}
            nodeCanvasObject={(node, ctx) => {
              const r = nodeRadius(node);
              const color = KIND_COLOR[node.kind];
              ctx.globalAlpha = dim(node.id as string) ? 0.2 : 1;
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
              ctx.globalAlpha = 1;
            }}
            nodePointerAreaPaint={(node, color, ctx) => {
              ctx.fillStyle = color;
              ctx.beginPath();
              ctx.arc(node.x ?? 0, node.y ?? 0, nodeRadius(node) + 2, 0, 2 * Math.PI);
              ctx.fill();
            }}
          />
        )}
      </div>
    </div>
  );
}
