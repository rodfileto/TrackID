import { useContext } from "react";
import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import { NodeViewWrapper, type NodeViewProps } from "@tiptap/react";
import type { MentionKind, MentionTarget } from "../../services/articles";
import { KIND_CLASS, KIND_CODE, targetHref } from "./mentionKinds";
import { MentionContext, mentionKey, pickedTargets } from "./mentionContext";

function formatDate(iso: string | undefined, locale: string): string {
  return iso ? new Date(iso).toLocaleDateString(locale) : "";
}

/** The small card shown when hovering a chip: what the target is, now. */
function MentionCard({ target, mergedFrom }: { target: MentionTarget; mergedFrom?: string }) {
  const { t, i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? "en";
  const facts: string[] = [];
  if (target.modality) facts.push(t(target.modality === "FACIAL" ? "modality.facial" : "modality.fingerprint"));
  if (target.status) facts.push(t(`occurrenceStatus.${target.status}`, { defaultValue: target.status }));
  if (target.reportType) facts.push(t(`analyses.reportType.${target.reportType}`, { defaultValue: target.reportType }));
  if (target.basis) facts.push(t(`articles.basis.${target.basis}`, { defaultValue: target.basis }));
  if (target.count !== undefined && target.count > 0) {
    facts.push(t(`articles.count.${target.kind}`, { count: target.count, defaultValue: String(target.count) }));
  }
  if (target.date) facts.push(formatDate(target.date, locale));
  return (
    <span
      role="tooltip"
      className="pointer-events-none invisible absolute left-0 top-full z-50 mt-1 block w-64 rounded-lg border border-gray-200 bg-white p-3 text-left text-xs font-normal normal-case text-gray-700 opacity-0 shadow-theme-lg transition-opacity group-hover:visible group-hover:opacity-100 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-300"
    >
      <span className="block text-[10px] uppercase tracking-wide text-gray-400">
        {t(`articles.kind.${target.kind}`)}
      </span>
      <span className="block font-medium text-gray-800 dark:text-white/90">{target.label}</span>
      {mergedFrom && (
        <span className="block text-warning-600 dark:text-warning-400">
          {t("articles.mergedFrom", { from: mergedFrom, to: target.ref })}
        </span>
      )}
      {target.description && target.description !== target.label && (
        <span className="mt-1 line-clamp-3 block">{target.description}</span>
      )}
      {target.caseIds && target.caseIds.length > 0 && target.kind !== "CASE" && (
        <span className="mt-1 block">{t("articles.inCases", { cases: target.caseIds.join(", ") })}</span>
      )}
      {facts.length > 0 && <span className="mt-1 block text-gray-500 dark:text-gray-400">{facts.join(" · ")}</span>}
    </span>
  );
}

/** One mention, as a chip: the kind's code, the target's label as it is now, a card on hover
 * and, when reading, a link to the target. A removed target is struck through with the label it
 * had when mentioned; a merged cluster shows the cluster that holds its members now. */
export function MentionChipView({ kind, refKey, label }: { kind: MentionKind; refKey: string; label: string }) {
  const { t } = useTranslation();
  const { resolved, editing } = useContext(MentionContext);
  const key = mentionKey(kind, refKey);
  const r = resolved.get(key);
  const target = r?.target ?? pickedTargets.get(key);
  const removed = r !== undefined && !r.found && !r.mergedInto;

  const shown = removed ? label : target?.label ?? label;
  const chip = (
    <span
      className={`group relative inline-flex items-baseline gap-1 rounded-md px-1.5 py-px text-[0.92em] font-medium ${KIND_CLASS[kind]} ${removed ? "line-through opacity-70" : ""}`}
      title={removed ? t("articles.removedTarget") : undefined}
    >
      <span className="text-[0.7em] font-semibold opacity-70">{KIND_CODE[kind]}</span>
      <span>{shown}</span>
      {r?.mergedInto && <span className="text-[0.75em] opacity-80">→ #{r.mergedInto}</span>}
      {target && !removed && <MentionCard target={target} mergedFrom={r?.mergedInto ? refKey : undefined} />}
    </span>
  );

  const href = !editing && target && !removed ? targetHref(target) : null;
  if (href) {
    return (
      <Link to={href} className="no-underline hover:opacity-90">
        {chip}
      </Link>
    );
  }
  return chip;
}

/** The TipTap node view for a mention node. */
export function MentionNodeView({ node }: NodeViewProps) {
  const { kind, ref, label } = node.attrs as { kind: MentionKind; ref: string; label: string };
  return (
    <NodeViewWrapper as="span" className="mention-node">
      <MentionChipView kind={kind} refKey={ref} label={label} />
    </NodeViewWrapper>
  );
}
