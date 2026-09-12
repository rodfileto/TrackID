import type { Claim } from "../../services/entityProfile";
import ProvenanceChip from "./ProvenanceChip";

// Every value on an entity page renders through this component. There is
// deliberately no component that renders a bare value, so that no fact can
// reach the screen without the source that asserted it.
type ClaimValueProps = {
  claim: Claim;
};

function sourceLabel(claim: Claim): string {
  switch (claim.sourceType) {
    case "INFOBIO_ENROLLMENT":
      return `NIF ${claim.sourceId}`;
    case "GRAPH_CACHE":
      return `NIF ${claim.sourceId} · cache do grafo`;
    case "GRAPH_NODE":
      return "Registro da pessoa";
    default:
      return claim.sourceId;
  }
}

function formatDate(value?: string): string {
  if (!value) return "";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return parsed.toLocaleDateString("pt-BR");
}

export default function ClaimValue({ claim }: ClaimValueProps) {
  const asserted = formatDate(claim.assertedAt);
  const details = [sourceLabel(claim)];
  if (claim.containerNumber) details.push(`contêiner ${claim.containerNumber}`);
  if (asserted) details.push(asserted);

  const isWeak = claim.sourceType === "GRAPH_CACHE";

  return (
    <div className="flex flex-col gap-1">
      <div className="flex flex-wrap items-center gap-2">
        <span
          className={
            isWeak
              ? "text-sm text-gray-500 dark:text-gray-400"
              : "text-sm font-medium text-gray-800 dark:text-white/90"
          }
        >
          {claim.value}
        </span>
        <ProvenanceChip
          status={claim.linkStatus}
          confidence={claim.confidence}
          title={details.join(" · ")}
        />
      </div>
      <span className="text-theme-xs text-gray-500 dark:text-gray-400">
        {details.join(" · ")}
      </span>
    </div>
  );
}
