import { useEffect, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import PageMeta from "../components/common/PageMeta";
import PageBreadcrumb from "../components/common/PageBreadCrumb";
import OccurrenceFields from "../components/intelligence/OccurrenceFields";
import {
  emptyOccurrence,
  occurrenceInput,
  occurrenceReady,
  type OccurrenceDraft,
} from "../components/intelligence/occurrenceDraft";
import { createOccurrence, getCaseOccurrenceLink, listOccurrenceTypes, type OccurrenceType } from "../services/occurrences";

// Register an occurrence: TrackID numbers it ("2026.0001"); the police agency's
// own report number is optional. The location is a point on the map, an address
// search, or an existing location nearby.
export default function NewOccurrence() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  // ?case= comes from the case-creation flow: the occurrence is linked to that
  // case when saved, so only types of the case's nature can be chosen.
  const [searchParams] = useSearchParams();
  const caseId = searchParams.get("case") ?? "";
  const [caseNature, setCaseNature] = useState<string | undefined>();
  const [caseLinked, setCaseLinked] = useState(false);

  const [types, setTypes] = useState<OccurrenceType[]>([]);
  const [draft, setDraft] = useState<OccurrenceDraft>(emptyOccurrence);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    listOccurrenceTypes().then(setTypes).catch(() => setError(t("occurrenceForm.typesError")));
  }, [t]);

  useEffect(() => {
    if (!caseId) return;
    getCaseOccurrenceLink(caseId)
      .then((link) => {
        setCaseNature(link.caseType);
        setCaseLinked(link.occurrence !== null);
      })
      .catch(() => setError(t("occurrenceForm.caseError")));
  }, [caseId, t]);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setSaving(true);
    try {
      const created = await createOccurrence({ ...occurrenceInput(draft, types), caseId: caseId || undefined });
      toast.success(t("occurrenceForm.created", { number: created.trackidNumber }));
      if (caseId) {
        navigate(`/cases/${encodeURIComponent(caseId)}`);
      } else {
        const year = new Date(draft.occurredFrom).getFullYear();
        navigate(`/intelligence/map?year=${year}&occurrence=${created.id}`);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : t("occurrenceForm.saveError"));
    } finally {
      setSaving(false);
    }
  }

  const ready = occurrenceReady(draft) && !caseLinked;

  return (
    <>
      <PageMeta title={`${t("occurrenceForm.title")} | TrackID`} description={t("occurrenceForm.metaDescription")} />
      <PageBreadcrumb pageTitle={t("occurrenceForm.title")} />

      <form onSubmit={submit} className="space-y-6">
        {caseId && (
          <div
            className={`rounded-xl border px-5 py-4 text-sm ${
              caseLinked
                ? "border-warning-300 bg-warning-50 text-warning-700 dark:border-warning-500/30 dark:bg-warning-500/10 dark:text-warning-400"
                : "border-brand-200 bg-brand-50 text-brand-700 dark:border-brand-500/30 dark:bg-brand-500/10 dark:text-brand-300"
            }`}
          >
            {t(caseLinked ? "occurrenceForm.caseAlreadyLinked" : "occurrenceForm.forCase", { caseId })}{" "}
            <Link to={`/cases/${encodeURIComponent(caseId)}`} className="font-medium underline">
              {t("occurrenceForm.backToCase")}
            </Link>
          </div>
        )}
        <OccurrenceFields value={draft} onChange={setDraft} types={types} fixedNature={caseNature} />

        {error && <p className="text-sm text-error-500" role="alert">{error}</p>}
        <div className="flex items-center justify-end gap-3">
          <Link to={caseId ? `/cases/${encodeURIComponent(caseId)}` : "/intelligence/map"} className="h-11 rounded-lg border border-gray-300 px-5 py-2.5 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-white/[0.03]">
            {t("common.cancel")}
          </Link>
          <button type="submit" disabled={!ready || saving}
            className="h-11 rounded-lg bg-brand-500 px-5 text-sm font-medium text-white hover:bg-brand-600 disabled:opacity-50">
            {saving ? t("occurrenceForm.saving") : t("occurrenceForm.save")}
          </button>
        </div>
      </form>
    </>
  );
}
