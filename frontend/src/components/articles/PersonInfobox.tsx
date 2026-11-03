import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { getIdentityFileObjectUrl, type PersonProfile } from "../../services/persons";
import type { LinkCounts } from "../../services/articles";

/** The enrolled photo, at infobox size. */
function InfoboxPhoto({ personId, identityFileId, alt }: { personId: string; identityFileId: number; alt: string }) {
  const [url, setUrl] = useState<string | null>(null);
  useEffect(() => {
    let cancelled = false;
    let created: string | null = null;
    getIdentityFileObjectUrl(personId, identityFileId)
      .then((u) => {
        created = u;
        if (cancelled) URL.revokeObjectURL(u);
        else setUrl(u);
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
      if (created) URL.revokeObjectURL(created);
    };
  }, [personId, identityFileId]);
  if (!url) return <div className="aspect-[3/4] w-full rounded-lg bg-gray-100 dark:bg-white/5" />;
  return <img src={url} alt={alt} className="aspect-[3/4] w-full rounded-lg object-cover" />;
}

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="grid grid-cols-[7rem_1fr] gap-2 border-t border-gray-100 px-4 py-2 text-sm first:border-t-0 dark:border-gray-800">
      <dt className="font-medium text-gray-500 dark:text-gray-400">{label}</dt>
      <dd className="min-w-0 break-words text-gray-800 dark:text-white/90">{children}</dd>
    </div>
  );
}

/** The infobox: what the data says about the person, generated and never typed, so the article
 * beside it cannot contradict the records (docs/person-article-plan.md §1). */
/** Counts by certainty, most certain first; "not reviewed" stands out, as a lead to follow. */
function Certainty({ parts }: { parts: { count: number; label: string; open?: boolean; always?: boolean }[] }) {
  const shown = parts.filter((p) => p.count > 0 || p.always);
  if (shown.length === 0) return <>—</>;
  return (
    <span className="flex flex-col">
      {shown.map((p) => (
        <span key={p.label} className={p.open && p.count > 0 ? "text-warning-700 dark:text-warning-400" : ""}>
          {p.count} {p.label}
        </span>
      ))}
    </span>
  );
}

export default function PersonInfobox({
  profile,
  name,
  counts,
}: {
  profile: PersonProfile;
  name: string;
  /** Links by certainty (the matcher's unreviewed ones included); the reviewed profile alone until loaded. */
  counts?: LinkCounts | null;
}) {
  const { t } = useTranslation();
  // The API sends null rather than [] for an empty list (Go nil slices).
  const { identity } = profile;
  const clusters = profile.clusters ?? [];
  const cases = profile.cases ?? [];
  const documents = identity.documents ?? [];
  const registers = documents.flatMap((d) => d.registers ?? []);
  const photo = registers
    .flatMap((r) => r.files ?? [])
    .find((f) => f.contentType?.startsWith("image/"));
  const names = Array.from(new Set(registers.map((r) => r.name).filter(Boolean)));
  const born = registers.find((r) => r.birthDate)?.birthDate;
  const byModality = (list: { modality: string }[]) => {
    const facial = list.filter((x) => x.modality === "FACIAL").length;
    const fingerprint = list.length - facial;
    return [
      facial > 0 ? `${facial} ${t("modality.facial").toLowerCase()}` : "",
      fingerprint > 0 ? `${fingerprint} ${t("modality.fingerprint").toLowerCase()}` : "",
    ]
      .filter(Boolean)
      .join(", ");
  };

  return (
    <aside className="overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-gray-800 dark:bg-white/[0.03]">
      <p className="bg-gray-50 px-4 py-2.5 text-center font-semibold text-gray-800 dark:bg-white/[0.04] dark:text-white/90">
        {name}
      </p>
      {photo && (
        <div className="px-4 pt-3">
          <InfoboxPhoto personId={identity.personId} identityFileId={photo.identityFileId} alt={name} />
        </div>
      )}
      <dl className="py-2">
        <Row label={t("articles.infobox.personId")}>{identity.personId}</Row>
        {names.length > 1 && <Row label={t("articles.infobox.names")}>{names.join("; ")}</Row>}
        {born && <Row label={t("articles.infobox.born")}>{born}</Row>}
        {documents.length > 0 && (
          <Row label={t("articles.infobox.documents")}>
            {documents.map((d) => `${d.documentType} ${d.documentNumber}`).join("; ")}
          </Row>
        )}
        {counts ? (
          <>
            <Row label={t("articles.infobox.cases")}>
              <Certainty
                parts={[
                  { count: counts.cases.confirmed, label: t("articles.certainty.confirmed"), always: true },
                  { count: counts.cases.unreviewed, label: t("articles.certainty.unreviewed"), open: true },
                  { count: counts.cases.uncertain, label: t("articles.certainty.uncertain"), open: true },
                  { count: counts.cases.excluded, label: t("articles.certainty.excluded") },
                ]}
              />
            </Row>
            <Row label={t("articles.infobox.clusters")}>
              <Certainty
                parts={[
                  { count: counts.clusters.reviewed, label: t("articles.certainty.reviewedClusters"), always: true },
                  { count: counts.clusters.automatic, label: t("articles.certainty.automaticClusters"), open: true },
                ]}
              />
            </Row>
            {counts.otherIdentities.confirmed + counts.otherIdentities.unreviewed > 0 && (
              <Row label={t("articles.infobox.otherIdentities")}>
                <Certainty
                  parts={[
                    { count: counts.otherIdentities.confirmed, label: t("articles.certainty.confirmed") },
                    { count: counts.otherIdentities.unreviewed, label: t("articles.certainty.unreviewed"), open: true },
                  ]}
                />
              </Row>
            )}
            {counts.associates.confirmed + counts.associates.unreviewed > 0 && (
              <Row label={t("articles.infobox.associates")}>
                <Certainty
                  parts={[
                    { count: counts.associates.confirmed, label: t("articles.certainty.confirmed") },
                    { count: counts.associates.unreviewed, label: t("articles.certainty.unreviewed"), open: true },
                  ]}
                />
              </Row>
            )}
          </>
        ) : (
          <>
            <Row label={t("articles.infobox.clusters")}>
              {clusters.length === 0 ? "—" : `${clusters.length} (${byModality(clusters)})`}
            </Row>
            <Row label={t("articles.infobox.cases")}>{cases.length === 0 ? "—" : `${cases.length} (${byModality(cases)})`}</Row>
          </>
        )}
      </dl>
      <p className="border-t border-gray-100 px-4 py-2 text-[11px] text-gray-400 dark:border-gray-800">
        {t("articles.infobox.generated")}
        {counts && counts.cases.unreviewed + counts.cases.uncertain + counts.clusters.automatic > 0 && (
          <span className="mt-1 block">{t("articles.certainty.note")}</span>
        )}
      </p>
    </aside>
  );
}
