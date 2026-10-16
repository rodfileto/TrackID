import type { AnalysisStage, AnalysisType, DecisionValue } from "../../services/analyses";

type BadgeColor = "primary" | "success" | "error" | "warning" | "info" | "light" | "dark";

export function stageColor(stage: AnalysisStage, status: string): BadgeColor {
  if (status === "CLOSED") return "light";
  switch (stage) {
    case "EXAMINATION":
      return "info";
    case "VERIFICATION":
      return "primary";
    case "INCONSISTENCE":
      return "warning";
    default:
      return "success";
  }
}

export function decisionColor(decision?: string): BadgeColor {
  if (decision === "POSITIVE") return "success";
  if (decision === "NEGATIVE") return "error";
  return "warning";
}

export const decisionValues: DecisionValue[] = ["POSITIVE", "NEGATIVE", "INCONCLUSIVE"];

export function typeKey(type: AnalysisType): string {
  return `analyses.type.${type}`;
}
