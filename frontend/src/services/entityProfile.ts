import { getToken } from "./auth";

const apiBaseUrl = import.meta.env.VITE_API_URL ?? "/api";

export type SourceState = "OK" | "UNAVAILABLE";

export type Claim = {
  value: string;
  sourceType: "INFOBIO_ENROLLMENT" | "GRAPH_NODE" | "GRAPH_CACHE";
  sourceId: string;
  linkStatus?: string;
  confidence?: number;
  assertedAt?: string;
  containerNumber?: string;
};

export type AttributeSet = {
  field: string;
  claims: Claim[];
  divergent: boolean;
  distinctValues: number;
  missingIn?: string[];
};

export type Identity = {
  attributes: AttributeSet[];
  divergentFields: string[];
};

export type EnrollmentLink = {
  nif: string;
  documentType?: string;
  status?: string;
  confidence?: number;
  source?: string;
  updatedAt?: string;
  containerNumber?: string;
  graphNome?: string;
};

export type BiometricFeature = {
  featureId: string;
  modality: string;
  finger?: string;
  templateRef?: string;
  algorithm?: string;
  algorithmVersion?: string;
  quality?: number;
  createdAt?: string;
};

export type EvidenceTrait = {
  featureId: string;
  role?: string;
  modality?: string;
  finger?: string;
  documentId?: string;
  documentKind?: string;
  occurrenceId?: string;
  occurredAt?: string;
  caseId?: string;
  locationId?: string;
};

export type EvidenceClaim = {
  evidenceId: string;
  claim?: string;
  method?: string;
  status?: string;
  confidence?: number;
  assertedBy?: string;
  assertedAt?: string;
  viaNif?: string;
  traits: EvidenceTrait[];
};

export type CaseInvolvement = {
  caseId: string;
  caseNumber?: string;
  caseType?: string;
  status?: string;
  title?: string;
  role?: string;
  confidence?: number;
  updatedAt?: string;
};

export type MergeRecord = {
  eventId?: string;
  reason?: string;
  confidence?: number;
  createdAt?: string;
  sourcePersonId?: string;
  mergedIntoPersonId?: string;
};

export type PersonProfile = {
  subject: { type: "PERSON"; id: string; displayName?: string; status?: string };
  sources: { graph: SourceState; enrollments: SourceState };
  identity: Identity;
  mergedInto?: string;
  sections: {
    enrollments: EnrollmentLink[];
    biometrics: BiometricFeature[];
    evidence: EvidenceClaim[];
    cases: CaseInvolvement[];
    merges: MergeRecord[];
  };
};

export type EntityProfileError = Error & { status?: number };

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const token = getToken();
  const response = await fetch(`${apiBaseUrl}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...options.headers,
    },
  });

  const body = (await response.json().catch(() => ({}))) as Record<string, unknown>;
  if (!response.ok) {
    const error = new Error(
      typeof body.detail === "string"
        ? body.detail
        : typeof body.error === "string"
          ? body.error
          : "A solicitação não foi concluída",
    ) as EntityProfileError;
    error.status = response.status;
    throw error;
  }
  return body as T;
}

export async function getPersonProfile(personId: string): Promise<PersonProfile> {
  return request<PersonProfile>(`/graph/persons/${encodeURIComponent(personId)}/profile`);
}

export type EnrollmentPersonLink = {
  personId: string;
  status: string;
  confidence: number;
  source: string;
};

export type EnrollmentRecord = {
  nif: { nif: string; containerNumber: string };
  personLinks: EnrollmentPersonLink[];
};

// resolvePersonIdForEnrollment looks up which known identity (if any) an enrollment document is
// currently linked to. An enrollment is never itself a person — see docs/graph-schema.md's
// "Enrollment documents" — so any page that only has a document number (an InfoBio NIF today)
// must resolve it to a personId before it can open that subject's profile; there is no shortcut
// id scheme to construct locally any more.
export async function resolvePersonIdForEnrollment(nif: string): Promise<string | undefined> {
  const record = await request<EnrollmentRecord>(`/graph/infobio/nifs/${encodeURIComponent(nif)}`);
  return record.personLinks[0]?.personId;
}
