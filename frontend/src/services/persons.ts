import { apiBaseUrl, requestApi } from "./api";
import { getToken } from "./auth";

/** One identity_file row -- a raw file produced by an enrollment event (a
 * photo, a NIST record, ...) -- plus the KNOWN biometric feature it yielded.
 * featureId/featureType are unset when the file type yields no feature
 * (e.g. a pdf). */
export interface PersonIdentityFile {
  identityFileId: number;
  fileType: string;
  sequence: number;
  sourcePath: string;
  storageRef: string;
  contentType?: string;
  sizeBytes?: number;
  featureId?: number;
  featureType?: string;
}

/** One enrollment event (identity_register) plus the files it produced. */
export interface PersonIdentityRegister {
  registerId: number;
  registerNumber: string;
  name: string;
  parent1Name: string;
  parent1Gender: string;
  parent2Name: string;
  parent2Gender: string;
  birthDate?: string;
  files: PersonIdentityFile[];
}

/** One identity_document row plus the registers filed under it. */
export interface PersonIdentityDocument {
  documentId: number;
  documentNumber: string;
  documentType: string;
  fiscalNumber?: string;
  registers: PersonIdentityRegister[];
}

/** A person's full enrollment chain. */
export interface PersonIdentity {
  personId: string;
  documents: PersonIdentityDocument[];
}

/** One biometric sample a cluster groups together -- an enrollment photo
 * (kind "KNOWN") or a crime-scene trace (kind "QUESTIONED"). Only the fields
 * for that kind are set: personId..contentType for KNOWN, caseId..
 * thumbnailBox for QUESTIONED (see person.ClusterMember). */
export interface PersonClusterMember {
  kind: "KNOWN" | "QUESTIONED";

  personId?: string;
  name?: string;
  registerNumber?: string;
  documentType?: string;
  documentNumber?: string;
  identityFileId?: number;
  contentType?: string;

  caseId?: string;
  modality?: string;
  description?: string;
  traceId?: number;
  thumbnailFileId?: number;
  thumbnailBox?: ThumbnailBox;
}

/** One biometric cluster a person's enrolled features have resolved into
 * (see MODEL.md section 4). hasCaseEvidence is true when at least one member
 * is QUESTIONED -- this person's biometric has been matched to real
 * crime-scene evidence, not just to another enrollment record. */
export interface PersonCluster {
  clusterId: number;
  modality: string;
  createdAt: string;
  memberCount: number;
  hasCaseEvidence: boolean;
  members: PersonClusterMember[];
}

/** One criminal case linked to a person through a resolved biometric
 * cluster. clusterIds names which of the person's clusters supplied the
 * link -- a case can carry more than one (e.g. face and fingerprint
 * evidence both resolving to this person). */
export interface PersonRelatedCase {
  caseId: string;
  modality: string;
  description: string;
  clusterIds: number[];
}

/** A person's full intelligence profile: identity, resolved biometric
 * clusters, and the criminal cases linked through them. */
export interface PersonProfile {
  identity: PersonIdentity;
  clusters: PersonCluster[];
  cases: PersonRelatedCase[];
}

/** How many distinct criminal cases a search result is linked to for one
 * modality ("FACIAL" or "FINGERPRINT") -- see person.ModalityCount. */
export interface ModalityCount {
  modality: string;
  count: number;
}

/** One identity_register whose name matched a name search -- a candidate to
 * disambiguate before opening a person's full profile. The same personId
 * can appear more than once when the search term matches several of a
 * person's registers (an alias, or a name spelled differently across
 * enrollments).
 *
 * caseCounts is the "N facial / N fingerprint" badge to show alongside this
 * candidate -- empty (or null) when the person has no linked criminal
 * cases. */
export interface PersonSearchResult {
  personId: string;
  name: string;
  registerNumber: string;
  documentType: string;
  documentNumber: string;
  caseCounts: ModalityCount[] | null;
}

/** One identity_register whose enrolled face matched a face search --
 * a candidate to disambiguate before opening a person's full profile,
 * ranked by similarity (cosine similarity, 1 = identical direction).
 * identityFileId is the enrollment photo the match was made against, for a
 * thumbnail (see IdentityFileThumbnail) -- the whole file is already just
 * the face, no crop needed. */
export interface PersonFaceSearchResult extends PersonSearchResult {
  similarity: number;
  identityFileId: number;
  contentType?: string;
}

/** A face's bounding box within a CaseFaceSearchResult's thumbnailFileId
 * image, in that image's own pixel coordinates. */
export interface ThumbnailBox {
  x1: number;
  y1: number;
  x2: number;
  y2: number;
}

/** One criminal case whose evidence holds a QUESTIONED face trace matching a
 * face search -- traceId names the best-scoring trace that put this case in
 * the results, ranked by similarity. Not directly comparable to a
 * PersonFaceSearchResult's similarity (KNOWN vs QUESTIONED embeddings).
 *
 * thumbnailFileId/thumbnailBox say how to render that trace as a thumbnail:
 * download thumbnailFileId via getEvidenceObjectUrl, and if thumbnailBox is
 * set, crop it down to that box (see utils/cropImage) -- otherwise the file
 * is already just the face. thumbnailFileId is unset when neither the
 * trace's own crop nor its evidence file is available anymore. */
export interface CaseFaceSearchResult {
  caseId: string;
  modality: string;
  description: string;
  traceId: number;
  similarity: number;
  thumbnailFileId?: number;
  thumbnailBox?: ThumbnailBox;
}

/** searchPersonsByFace's result: enrolled persons whose KNOWN face matched,
 * and criminal cases whose evidence holds a matching QUESTIONED face
 * trace -- kept as two separate ranked lists (see person.FaceSearchResults). */
export interface FaceSearchResults {
  persons: PersonFaceSearchResult[];
  cases: CaseFaceSearchResult[];
}

function authHeaders(): Record<string, string> {
  const token = getToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

export async function searchPersonsByName(
  name: string,
): Promise<PersonSearchResult[]> {
  const { results } = await requestApi<{ results: PersonSearchResult[] }>(
    `/persons/search?name=${encodeURIComponent(name)}`,
    { headers: authHeaders() },
  );
  return results;
}

export async function searchPersonsByFace(
  image: File,
): Promise<FaceSearchResults> {
  const form = new FormData();
  form.append("image", image);

  const token = getToken();
  const response = await fetch(`${apiBaseUrl}/persons/search-by-face`, {
    method: "POST",
    headers: token ? { Authorization: `Bearer ${token}` } : {},
    body: form,
  });

  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as {
      error?: string;
    };
    throw new Error(body.error ?? "Could not search persons by face");
  }

  return (await response.json()) as FaceSearchResults;
}

export async function getPersonProfile(
  personId: string,
): Promise<PersonProfile> {
  return requestApi<PersonProfile>(
    `/persons/${encodeURIComponent(personId)}`,
    { headers: authHeaders() },
  );
}

export async function getIdentityFileObjectUrl(
  personId: string,
  identityFileId: number,
): Promise<string> {
  const token = getToken();
  const response = await fetch(
    `${apiBaseUrl}/persons/${encodeURIComponent(personId)}/identity-files/${identityFileId}/download`,
    { headers: token ? { Authorization: `Bearer ${token}` } : {} },
  );

  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as {
      error?: string;
    };
    throw new Error(body.error ?? "Could not load identity file");
  }

  const blob = await response.blob();
  return URL.createObjectURL(blob);
}

export async function getPersonIdentity(
  personId: string,
): Promise<PersonIdentity> {
  return requestApi<PersonIdentity>(
    `/persons/${encodeURIComponent(personId)}/identity`,
    { headers: authHeaders() },
  );
}

export async function listPersonClusters(
  personId: string,
): Promise<PersonCluster[]> {
  const { clusters } = await requestApi<{ clusters: PersonCluster[] }>(
    `/persons/${encodeURIComponent(personId)}/clusters`,
    { headers: authHeaders() },
  );
  return clusters;
}

export async function listPersonCases(
  personId: string,
): Promise<PersonRelatedCase[]> {
  const { cases } = await requestApi<{ cases: PersonRelatedCase[] }>(
    `/persons/${encodeURIComponent(personId)}/cases`,
    { headers: authHeaders() },
  );
  return cases;
}
