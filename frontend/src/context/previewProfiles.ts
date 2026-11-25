import type { PreviewProfile } from "../extensions/types";

/** Core's profiles an admin can preview `/` and the menu as (docs/access-profiles-plan.md). */
export const corePreviewProfiles: PreviewProfile[] = [
  { name: "preview.profiles.fingerprintExaminer", access: { examines_fingerprint: true } },
  { name: "preview.profiles.facialExaminer", access: { examines_facial: true } },
  { name: "preview.profiles.analyst", access: { intelligence_analyst: true } },
  { name: "preview.profiles.coordinator", access: { coordinator: true } },
  { name: "preview.profiles.none" },
];
