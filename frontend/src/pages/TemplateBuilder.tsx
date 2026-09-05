import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ReactFlow,
  Background,
  BaseEdge,
  Controls,
  EdgeLabelRenderer,
  Handle,
  Position,
  getStraightPath,
  useInternalNode,
  useNodesState,
  useEdgesState,
  type Edge,
  type EdgeProps,
  type FinalConnectionState,
  type InternalNode,
  type Node,
  type NodeProps,
  type ReactFlowInstance,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";

import PageBreadcrumb from "../components/common/PageBreadCrumb";
import PageMeta from "../components/common/PageMeta";
import ComponentCard from "../components/common/ComponentCard";
import Label from "../components/form/Label";
import Input from "../components/form/input/InputField";
import TextArea from "../components/form/input/TextArea";
import Button from "../components/ui/button/Button";
import Badge from "../components/ui/badge/Badge";
import Alert from "../components/ui/alert/Alert";
import {
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableRow,
} from "../components/ui/table";
import {
  createTemplate,
  fetchTemplate,
  fetchTemplateList,
  updateTemplate,
  type ConceptDefinition,
  type ConceptRelation,
  type TemplateSummary,
} from "../types/ontology";

type ConceptNodeData = {
  label: string;
  description?: string;
  isEditing?: boolean;
  onCommitName?: (id: string, name: string) => void;
  onStartEdit?: (id: string) => void;
  onDelete?: (id: string) => void;
  onSetDescription?: (id: string, description: string) => void;
};

function ConceptNode({ id, data }: NodeProps) {
  const {
    label,
    description,
    isEditing,
    onCommitName,
    onStartEdit,
    onDelete,
    onSetDescription,
  } = data as ConceptNodeData;

  const [draftName, setDraftName] = useState(label);
  const [editingDescription, setEditingDescription] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (isEditing) {
      setDraftName(label);
      requestAnimationFrame(() => {
        inputRef.current?.focus();
        inputRef.current?.select();
      });
    }
  }, [isEditing, label]);

  const commit = () => onCommitName?.(id, draftName);

  return (
    <div className="group relative max-w-[220px] rounded-xl border border-gray-200 bg-white px-4 py-3 shadow-theme-xs dark:border-gray-800 dark:bg-white/[0.03]">
      <Handle type="target" position={Position.Left} />

      {onDelete && (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            onDelete(id);
          }}
          className="nodrag absolute -right-2 -top-2 hidden h-5 w-5 items-center justify-center rounded-full border border-gray-300 bg-white text-xs leading-none text-gray-500 hover:border-error-300 hover:text-error-500 group-hover:flex dark:border-gray-700 dark:bg-gray-900"
          title="Delete concept"
        >
          ×
        </button>
      )}

      {isEditing ? (
        <input
          ref={inputRef}
          value={draftName}
          onChange={(e) => setDraftName(e.target.value)}
          onBlur={commit}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              commit();
            }
          }}
          onClick={(e) => e.stopPropagation()}
          className="nodrag w-full rounded border border-brand-300 bg-transparent px-1 text-sm font-medium text-gray-800 outline-none dark:text-white/90"
        />
      ) : (
        <p
          className="font-medium text-gray-800 dark:text-white/90"
          onDoubleClick={(e) => {
            e.stopPropagation();
            onStartEdit?.(id);
          }}
        >
          {label || <span className="italic text-gray-400">Untitled</span>}
        </p>
      )}

      {editingDescription && onSetDescription ? (
        <textarea
          autoFocus
          defaultValue={description ?? ""}
          rows={2}
          onClick={(e) => e.stopPropagation()}
          onBlur={(e) => {
            onSetDescription(id, e.target.value.trim());
            setEditingDescription(false);
          }}
          className="nodrag mt-1 w-full rounded border border-gray-200 p-1 text-xs text-gray-600 outline-none dark:border-gray-700 dark:bg-gray-900 dark:text-gray-300"
        />
      ) : description ? (
        <p
          className="mt-1 text-xs text-gray-500 dark:text-gray-400"
          onDoubleClick={(e) => {
            e.stopPropagation();
            setEditingDescription(true);
          }}
        >
          {description}
        </p>
      ) : (
        onSetDescription && (
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation();
              setEditingDescription(true);
            }}
            className="nodrag mt-1 text-xs text-brand-500 hover:underline"
          >
            + description
          </button>
        )
      )}

      <Handle type="source" position={Position.Right} />
    </div>
  );
}

/**
 * Where a straight line between two node centers crosses `intersectionNode`'s
 * boundary - standard React Flow "floating edge" geometry. Needed because
 * ConceptNode always renders its target handle on the left and source
 * handle on the right: an edge whose source node ends up laid out to the
 * *right* of its target (e.g. the user dragged the connection in the
 * opposite direction from left-to-right reading order) would otherwise
 * have to travel from the source's right-side handle, loop around both
 * nodes' outer edges, and arrive at the target's left-side handle -
 * rendering as a stray stub past each node rather than a direct line
 * between them. Computing the path from actual node positions instead of
 * fixed handle coordinates makes the edge direction irrelevant to how it
 * looks.
 */
function getNodeIntersection(intersectionNode: InternalNode, targetNode: InternalNode) {
  const w = (intersectionNode.measured.width ?? 0) / 2;
  const h = (intersectionNode.measured.height ?? 0) / 2;
  const x2 = intersectionNode.internals.positionAbsolute.x + w;
  const y2 = intersectionNode.internals.positionAbsolute.y + h;
  const x1 = targetNode.internals.positionAbsolute.x + (targetNode.measured.width ?? 0) / 2;
  const y1 = targetNode.internals.positionAbsolute.y + (targetNode.measured.height ?? 0) / 2;

  const xx1 = (x1 - x2) / (2 * w) - (y1 - y2) / (2 * h);
  const yy1 = (x1 - x2) / (2 * w) + (y1 - y2) / (2 * h);
  const a = 1 / (Math.abs(xx1) + Math.abs(yy1) || 1);
  const xx3 = a * xx1;
  const yy3 = a * yy1;

  return { x: w * (xx3 + yy3) + x2, y: h * (-xx3 + yy3) + y2 };
}

function getFloatingEdgeParams(source: InternalNode, target: InternalNode) {
  const sourcePoint = getNodeIntersection(source, target);
  const targetPoint = getNodeIntersection(target, source);
  return { sx: sourcePoint.x, sy: sourcePoint.y, tx: targetPoint.x, ty: targetPoint.y };
}

function ConceptEdge({ id, source, target, style, data }: EdgeProps) {
  const sourceNode = useInternalNode(source);
  const targetNode = useInternalNode(target);

  const onDelete = (data as { onDelete?: (id: string) => void } | undefined)
    ?.onDelete;

  if (!sourceNode || !targetNode) return null;

  const { sx, sy, tx, ty } = getFloatingEdgeParams(sourceNode, targetNode);
  const [edgePath, labelX, labelY] = getStraightPath({
    sourceX: sx,
    sourceY: sy,
    targetX: tx,
    targetY: ty,
  });

  return (
    <>
      <BaseEdge id={id} path={edgePath} style={style} />
      {onDelete && (
        <EdgeLabelRenderer>
          <div
            style={{
              position: "absolute",
              pointerEvents: "all",
              transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)`,
            }}
            className="nodrag nopan"
          >
            <button
              type="button"
              onClick={() => onDelete(id)}
              className="flex h-5 w-5 items-center justify-center rounded-full border border-gray-300 bg-white text-xs leading-none text-gray-500 hover:border-error-300 hover:text-error-500 dark:border-gray-700 dark:bg-gray-900"
              title="Delete relation"
            >
              ×
            </button>
          </div>
        </EdgeLabelRenderer>
      )}
    </>
  );
}

const nodeTypes = { concept: ConceptNode };
const edgeTypes = { concept: ConceptEdge };

const EDGE_STYLE = { stroke: "#667085" };

function circularLayout(concepts: ConceptDefinition[]): Node[] {
  const radius = Math.max(180, concepts.length * 40);
  const center = { x: radius + 40, y: radius + 40 };
  return concepts.map((c, i) => {
    const angle = (2 * Math.PI * i) / Math.max(1, concepts.length);
    return {
      id: c.name,
      type: "concept",
      position: {
        x: center.x + radius * Math.cos(angle),
        y: center.y + radius * Math.sin(angle),
      },
      data: { label: c.name, description: c.description },
    };
  });
}

function relationsToEdges(relations: ConceptRelation[]): Edge[] {
  return relations.map((r) => ({
    id: `${r.source}->${r.target}`,
    source: r.source,
    target: r.target,
    type: "concept",
    animated: false,
    style: EDGE_STYLE,
  }));
}

/**
 * Same layout as circularLayout, but with synthetic ids decoupled from the
 * concept name - required so renaming a concept in edit mode never has to
 * rewrite the node's id (and therefore every edge referencing it). Only
 * used to seed the editable canvas; read-only view mode keeps
 * circularLayout's id === name as-is, since it never renames anything.
 */
function seedEditableGraph(
  concepts: ConceptDefinition[],
  relations: ConceptRelation[],
): { nodes: Node[]; edges: Edge[] } {
  const nameToId = new Map<string, string>();
  const nodes: Node[] = circularLayout(concepts).map((n) => {
    const newId = crypto.randomUUID();
    nameToId.set(n.id, newId);
    return { ...n, id: newId };
  });
  const edges: Edge[] = relations.map((r) => {
    const source = nameToId.get(r.source) ?? r.source;
    const target = nameToId.get(r.target) ?? r.target;
    return { id: `${source}->${target}`, source, target, style: EDGE_STYLE };
  });
  return { nodes, edges };
}

export default function TemplateBuilder() {
  const [mode, setMode] = useState<"view" | "edit">("view");

  const [templateList, setTemplateList] = useState<TemplateSummary[]>([]);
  const [listError, setListError] = useState<string | null>(null);
  const [selectedName, setSelectedName] = useState<string | null>(null);
  const selectedSummary = templateList.find((t) => t.name === selectedName);

  const [viewNodes, setViewNodes] = useState<Node[]>([]);
  const [viewEdges, setViewEdges] = useState<Edge[]>([]);
  const [loadingDetail, setLoadingDetail] = useState(false);
  const [detailError, setDetailError] = useState<string | null>(null);

  const refreshList = async () => {
    try {
      const res = await fetchTemplateList();
      setTemplateList(res.items);
      setListError(null);
    } catch (err) {
      setListError(
        err instanceof Error ? err.message : "Failed to load templates.",
      );
    }
  };

  useEffect(() => {
    refreshList();
  }, []);

  const handleSelectTemplate = async (name: string) => {
    setMode("view");
    setSelectedName(name);
    setLoadingDetail(true);
    setDetailError(null);
    try {
      const template = await fetchTemplate(name);
      setViewNodes(circularLayout(template.concepts));
      setViewEdges(relationsToEdges(template.relations));
    } catch (err) {
      setDetailError(
        err instanceof Error ? err.message : "Failed to load template.",
      );
      setViewNodes([]);
      setViewEdges([]);
    } finally {
      setLoadingDetail(false);
    }
  };

  // --- Edit mode: shared by "create new" (editingExistingName === null)
  // and "edit an existing user-defined template" (editingExistingName set
  // to that template's name - builtins never reach this, see the "Edit"
  // button below, which only renders for source === "user-defined"). ---
  const [editingExistingName, setEditingExistingName] = useState<string | null>(null);
  const [newName, setNewName] = useState("");
  const [newDescription, setNewDescription] = useState("");
  const [editNodes, setEditNodes, onEditNodesChange] = useNodesState<Node>([]);
  const [editEdges, setEditEdges, onEditEdgesChange] = useEdgesState<Edge>([]);
  const [editingNodeId, setEditingNodeId] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const rfInstanceRef = useRef<ReactFlowInstance | null>(null);

  const startCreating = () => {
    setMode("edit");
    setEditingExistingName(null);
    setSelectedName(null);
    setNewName("");
    setNewDescription("");
    setEditNodes([]);
    setEditEdges([]);
    setEditingNodeId(null);
    setSaveError(null);
  };

  const startEditingExisting = async (name: string) => {
    setLoadingDetail(true);
    setDetailError(null);
    try {
      const template = await fetchTemplate(name);
      const { nodes, edges } = seedEditableGraph(template.concepts, template.relations);
      setMode("edit");
      setEditingExistingName(name);
      setSelectedName(name);
      setNewName(template.name);
      setNewDescription(template.description);
      setEditNodes(nodes);
      setEditEdges(edges);
      setEditingNodeId(null);
      setSaveError(null);
    } catch (err) {
      setDetailError(
        err instanceof Error ? err.message : "Failed to load template.",
      );
    } finally {
      setLoadingDetail(false);
    }
  };

  // Stable callbacks (id-keyed, functional state updates) passed into every
  // node/edge's data - see ConceptNode/ConceptEdge above.
  const commitNodeName = useCallback((id: string, name: string) => {
    const trimmed = name.trim();
    setEditNodes((nodes) =>
      nodes.map((n) =>
        n.id === id
          ? {
              ...n,
              data: {
                ...n.data,
                // Empty commit reverts to the previous name rather than
                // leaving a blank node - avoids losing an existing
                // concept's name to an accidental empty blur.
                label: trimmed || (n.data as ConceptNodeData).label,
              },
            }
          : n,
      ),
    );
    setEditingNodeId((current) => (current === id ? null : current));
  }, [setEditNodes]);

  const startEditingNode = useCallback((id: string) => {
    setEditingNodeId(id);
  }, []);

  const deleteNode = useCallback((id: string) => {
    rfInstanceRef.current?.deleteElements({ nodes: [{ id }] });
  }, []);

  const deleteEdge = useCallback((id: string) => {
    rfInstanceRef.current?.deleteElements({ edges: [{ id }] });
  }, []);

  const setNodeDescription = useCallback((id: string, description: string) => {
    setEditNodes((nodes) =>
      nodes.map((n) =>
        n.id === id ? { ...n, data: { ...n.data, description } } : n,
      ),
    );
  }, [setEditNodes]);

  const renderedEditNodes = useMemo(
    () =>
      editNodes.map((n) => ({
        ...n,
        data: {
          ...n.data,
          isEditing: n.id === editingNodeId,
          onCommitName: commitNodeName,
          onStartEdit: startEditingNode,
          onDelete: deleteNode,
          onSetDescription: setNodeDescription,
        },
      })),
    [editNodes, editingNodeId, commitNodeName, startEditingNode, deleteNode, setNodeDescription],
  );

  const renderedEditEdges = useMemo(
    () =>
      editEdges.map((e) => ({ ...e, type: "concept", data: { onDelete: deleteEdge } })),
    [editEdges, deleteEdge],
  );

  // Drag from a node's handle out to empty canvas -> new node, connected.
  // Fires regardless of outcome; a *valid* connection (node to node) is
  // handled by onConnect below, so this only acts when it landed on
  // nothing (connectionState.toNode === null).
  const onConnectEnd = useCallback(
    (event: MouseEvent | TouchEvent, connectionState: FinalConnectionState) => {
      if (connectionState.isValid) return;
      const fromNode = connectionState.fromNode;
      if (!fromNode || !rfInstanceRef.current) return;

      const point = "changedTouches" in event ? event.changedTouches[0] : event;
      if (!point) return;
      const position = rfInstanceRef.current.screenToFlowPosition({
        x: point.clientX,
        y: point.clientY,
      });

      const newId = crypto.randomUUID();
      const newNode: Node = {
        id: newId,
        type: "concept",
        position,
        data: { label: "New Concept" },
      };

      const fromIsSource = connectionState.fromHandle?.type === "source";
      const newEdge: Edge = fromIsSource
        ? { id: `${fromNode.id}->${newId}`, source: fromNode.id, target: newId, style: EDGE_STYLE }
        : { id: `${newId}->${fromNode.id}`, source: newId, target: fromNode.id, style: EDGE_STYLE };

      setEditNodes((nodes) => [...nodes, newNode]);
      setEditEdges((edges) => [...edges, newEdge]);
      setEditingNodeId(newId);
    },
    [setEditNodes, setEditEdges],
  );

  const onConnect = useCallback(
    (connection: import("@xyflow/react").Connection) => {
      setEditEdges((edges) => [
        ...edges,
        {
          id: `${connection.source}->${connection.target}`,
          source: connection.source,
          target: connection.target,
          style: EDGE_STYLE,
        },
      ]);
    },
    [setEditEdges],
  );

  // Double-click empty canvas -> new, unconnected node. Node-level
  // double-click (rename, see ConceptNode) stops propagation so this
  // pane-level handler only fires for the canvas background itself.
  const onPaneDoubleClick = useCallback(
    (event: React.MouseEvent) => {
      if (mode !== "edit" || !rfInstanceRef.current) return;
      const position = rfInstanceRef.current.screenToFlowPosition({
        x: event.clientX,
        y: event.clientY,
      });
      const newId = crypto.randomUUID();
      setEditNodes((nodes) => [
        ...nodes,
        { id: newId, type: "concept", position, data: { label: "New Concept" } },
      ]);
      setEditingNodeId(newId);
    },
    [mode, setEditNodes],
  );

  const handleSave = async () => {
    if (!newName.trim()) {
      setSaveError("Give the template a name.");
      return;
    }
    setSaving(true);
    setSaveError(null);
    try {
      const idToName = new Map(
        editNodes.map((n) => [n.id, (n.data as ConceptNodeData).label]),
      );
      const payload = {
        name: newName.trim(),
        description: newDescription.trim(),
        concepts: editNodes.map((n) => ({
          name: (n.data as ConceptNodeData).label,
          description: (n.data as ConceptNodeData).description ?? "",
        })),
        relations: editEdges.map((e) => ({
          source: idToName.get(e.source) ?? e.source,
          target: idToName.get(e.target) ?? e.target,
        })),
      };
      const saved = editingExistingName
        ? await updateTemplate(editingExistingName, payload)
        : await createTemplate(payload);
      await refreshList();
      await handleSelectTemplate(saved.name);
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : "Failed to save template.");
    } finally {
      setSaving(false);
    }
  };

  return (
    <div>
      <PageMeta
        title="Template Builder | TrackID"
        description="Visualize built-in Target-Type Templates or create new ones"
      />
      <PageBreadcrumb pageTitle="Template Builder" />

      <div className="grid grid-cols-1 gap-6 xl:grid-cols-3">
        <div className="xl:col-span-1">
          <ComponentCard
            title="Target-Type Templates"
            desc="A template is a network of Concepts describing how a kind of Target System typically operates."
          >
            {listError && (
              <Alert variant="error" title="Failed to load templates" message={listError} />
            )}
            <div className="overflow-hidden rounded-xl border border-gray-200 dark:border-white/[0.05]">
              <div className="max-w-full overflow-x-auto">
                <Table>
                  <TableHeader className="border-b border-gray-100 dark:border-white/[0.05]">
                    <TableRow>
                      <TableCell isHeader className="px-4 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400">
                        Name
                      </TableCell>
                      <TableCell isHeader className="px-4 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400">
                        Concepts
                      </TableCell>
                      <TableCell isHeader className="px-4 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400">
                        Source
                      </TableCell>
                    </TableRow>
                  </TableHeader>
                  <TableBody className="divide-y divide-gray-100 dark:divide-white/[0.05]">
                    {templateList.map((t) => (
                      <TableRow
                        key={t.name}
                        onClick={() => handleSelectTemplate(t.name)}
                        className={`cursor-pointer hover:bg-gray-50 dark:hover:bg-white/[0.03] ${
                          selectedName === t.name && mode === "view"
                            ? "bg-brand-50 dark:bg-brand-500/10"
                            : ""
                        }`}
                      >
                        <TableCell className="px-4 py-3 text-gray-700 text-start text-theme-sm dark:text-gray-300">
                          {t.name}
                        </TableCell>
                        <TableCell className="px-4 py-3 text-gray-500 text-start text-theme-sm dark:text-gray-400">
                          {t.concept_count}
                        </TableCell>
                        <TableCell className="px-4 py-3 text-start">
                          <Badge color={t.source === "builtin" ? "info" : "success"} size="sm">
                            {t.source}
                          </Badge>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            </div>

            <div className="flex gap-2">
              <Button onClick={startCreating} variant={mode === "edit" && !editingExistingName ? "primary" : "outline"}>
                + Create new template
              </Button>
              {mode === "view" && selectedName && selectedSummary?.source === "user-defined" && (
                <Button variant="outline" onClick={() => startEditingExisting(selectedName)}>
                  Edit
                </Button>
              )}
            </div>
          </ComponentCard>

          {mode === "edit" && (
            <ComponentCard
              title={editingExistingName ? `Editing: ${editingExistingName}` : "New template"}
              desc="Drag from a concept's edge into empty space for a new connected concept, or double-click empty canvas for an unconnected one. Double-click a concept to rename it."
            >
              <div>
                <Label>Template name</Label>
                <Input
                  value={newName}
                  onChange={(e) => setNewName(e.target.value)}
                  placeholder="e.g. Drug Trafficking Network"
                  disabled={!!editingExistingName}
                />
              </div>
              <div>
                <Label>Description</Label>
                <TextArea value={newDescription} onChange={setNewDescription} rows={2} />
              </div>

              {saveError && <Alert variant="error" title="Cannot save" message={saveError} />}

              <Button onClick={handleSave} disabled={saving}>
                {saving ? "Saving..." : editingExistingName ? "Save changes" : "Save template"}
              </Button>
            </ComponentCard>
          )}
        </div>

        <div className="xl:col-span-2">
          <ComponentCard
            title={
              mode === "edit"
                ? "Building: drag from a handle to connect, double-click canvas to add, double-click a concept to rename"
                : selectedName
                  ? `${selectedName} — Concept network`
                  : "Select a template to view its Concept network"
            }
          >
            {detailError && (
              <Alert variant="error" title="Failed to load template" message={detailError} />
            )}
            {loadingDetail ? (
              <p className="text-sm text-gray-500 dark:text-gray-400">Loading…</p>
            ) : (
              <div
                style={{ height: 500 }}
                className="rounded-xl border border-gray-200 dark:border-gray-800"
                onDoubleClick={onPaneDoubleClick}
              >
                <ReactFlow
                  nodes={mode === "edit" ? renderedEditNodes : viewNodes}
                  edges={mode === "edit" ? renderedEditEdges : viewEdges}
                  onNodesChange={mode === "edit" ? onEditNodesChange : undefined}
                  onEdgesChange={mode === "edit" ? onEditEdgesChange : undefined}
                  onConnect={mode === "edit" ? onConnect : undefined}
                  onConnectEnd={mode === "edit" ? onConnectEnd : undefined}
                  onInit={(instance) => {
                    rfInstanceRef.current = instance;
                  }}
                  nodesDraggable={mode === "edit"}
                  nodesConnectable={mode === "edit"}
                  elementsSelectable={mode === "edit"}
                  zoomOnDoubleClick={mode !== "edit"}
                  deleteKeyCode={["Backspace", "Delete"]}
                  nodeTypes={nodeTypes}
                  edgeTypes={edgeTypes}
                  fitView
                >
                  <Background />
                  <Controls showInteractive={mode === "edit"} />
                </ReactFlow>
              </div>
            )}
          </ComponentCard>
        </div>
      </div>
    </div>
  );
}
