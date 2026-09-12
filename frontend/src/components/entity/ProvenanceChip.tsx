import Badge from "../ui/badge/Badge";

type ProvenanceChipProps = {
  status?: string;
  confidence?: number;
  title?: string;
};

// Colours the review state of an assertion. Anything unrecognised stays neutral
// rather than borrowing the confidence of a state it has not earned.
function colorForStatus(status?: string) {
  switch (status) {
    case "CONFIRMED":
    case "VERIFIED":
      return "success" as const;
    case "PROPOSED":
      return "warning" as const;
    case "RETRACTED":
      return "error" as const;
    default:
      return "light" as const;
  }
}

const statusLabels: Record<string, string> = {
  CONFIRMED: "Confirmado",
  VERIFIED: "Verificado",
  PROPOSED: "Proposto",
  RETRACTED: "Retratado",
};

export default function ProvenanceChip({ status, confidence, title }: ProvenanceChipProps) {
  const label = status ? (statusLabels[status] ?? status) : "Sem revisão";
  const confidenceLabel =
    typeof confidence === "number" ? ` · ${Math.round(confidence * 100)}%` : "";

  return (
    <span title={title} aria-label={title}>
      <Badge variant="light" size="sm" color={colorForStatus(status)}>
        {`${label}${confidenceLabel}`}
      </Badge>
    </span>
  );
}
