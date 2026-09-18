import { requestApi } from "./api";
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

/** One biometric cluster a person's enrolled features have resolved into
 * (see MODEL.md section 4). */
export interface PersonCluster {
  clusterId: number;
  caseType: string;
  createdAt: string;
  memberCount: number;
}

/** One criminal case linked to a person through a resolved biometric
 * cluster. clusterIds names which of the person's clusters supplied the
 * link -- a case can carry more than one (e.g. face and fingerprint
 * evidence both resolving to this person). */
export interface PersonRelatedCase {
  caseId: string;
  caseType: string;
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

/** One identity_register whose name matched a name search -- a candidate to
 * disambiguate before opening a person's full profile. The same personId
 * can appear more than once when the search term matches several of a
 * person's registers (an alias, or a name spelled differently across
 * enrollments). */
export interface PersonSearchResult {
  personId: string;
  name: string;
  registerNumber: string;
  documentType: string;
  documentNumber: string;
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

export async function getPersonProfile(
  personId: string,
): Promise<PersonProfile> {
  return requestApi<PersonProfile>(
    `/persons/${encodeURIComponent(personId)}`,
    { headers: authHeaders() },
  );
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
