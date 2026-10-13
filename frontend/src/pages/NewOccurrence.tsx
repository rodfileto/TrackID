import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import PageMeta from "../components/common/PageMeta";
import PageBreadcrumb from "../components/common/PageBreadCrumb";
import ComponentCard from "../components/common/ComponentCard";
import Label from "../components/form/Label";
import LocationPicker from "../components/intelligence/LocationPicker";
import { GROUP_COLORS } from "../components/intelligence/groupColors";
import {
  createOccurrence,
  getCaseOccurrenceLink,
  listOccurrenceTypes,
  type OccurrenceType,
  type PlaceInput,
} from "../services/occurrences";

const GROUPS = Object.keys(GROUP_COLORS);
const STATUSES = ["REGISTERED", "UNDER_INVESTIGATION", "INQUIRY_OPENED", "CLOSED"];
const PLACE_TYPES = ["PUBLIC_ROAD", "RESIDENCE", "COMMERCE", "BANK", "GOVERNMENT_OFFICE", "TRANSPORT", "RURAL", "OTHER"];

const inputClass =
  "h-11 w-full rounded-lg border border-gray-300 bg-transparent px-4 text-sm text-gray-800 shadow-theme-xs placeholder:text-gray-400 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90";

/** A datetime-local value (the user's local time) as an ISO instant. */
function toISO(local: string): string | undefined {
  return local ? new Date(local).toISOString() : undefined;
}

function optionalInt(v: string): number | undefined {
  return v === "" ? undefined : Number(v);
}

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
  const [chosen, setChosen] = useState<string[]>([]);
  const [occurredFrom, setOccurredFrom] = useState("");
  const [foundLater, setFoundLater] = useState(false);
  const [occurredTo, setOccurredTo] = useState("");
  const [attempted, setAttempted] = useState(false);
  const [offenderCount, setOffenderCount] = useState("");
  const [victimCount, setVictimCount] = useState("");
  const [status, setStatus] = useState("REGISTERED");
  const [agency, setAgency] = useState("");
  const [agencyNumber, setAgencyNumber] = useState("");
  const [description, setDescription] = useState("");
  const [place, setPlace] = useState<PlaceInput>({});
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

  const natureOf = useMemo(() => Object.fromEntries(types.map((x) => [x.code, x.caseType])), [types]);
  const nature = chosen.length > 0 ? natureOf[chosen[0]] : caseNature;
  const civil = nature === "CIVIL";

  function toggle(list: string[], code: string): string[] {
    return list.includes(code) ? list.filter((c) => c !== code) : [...list, code];
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setSaving(true);
    try {
      const created = await createOccurrence({
        types: chosen,
        attempted: civil ? undefined : attempted || undefined,
        occurredFrom: toISO(occurredFrom) ?? "",
        occurredTo: foundLater ? toISO(occurredTo) : undefined,
        status,
        description,
        agency: agency || undefined,
        agencyNumber: agencyNumber || undefined,
        offenderCount: civil ? undefined : optionalInt(offenderCount),
        victimCount: optionalInt(victimCount),
        place,
        caseId: caseId || undefined,
      });
      toast.success(t("occurrenceForm.created", { number: created.trackidNumber }));
      if (caseId) {
        navigate(`/cases/${encodeURIComponent(caseId)}`);
      } else {
        const year = new Date(occurredFrom).getFullYear();
        navigate(`/intelligence/map?year=${year}&occurrence=${created.id}`);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : t("occurrenceForm.saveError"));
    } finally {
      setSaving(false);
    }
  }

  const hasPlace = place.existingId !== undefined || (place.lon !== undefined && place.lat !== undefined);
  const ready = chosen.length > 0 && occurredFrom !== "" && description.trim() !== "" && hasPlace && !caseLinked;

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
        <ComponentCard title={t("occurrenceForm.whatTitle")} desc={t("occurrenceForm.whatDesc")}>
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
            {GROUPS.map((g) => {
              const inGroup = types.filter((x) => x.group === g);
              if (inGroup.length === 0) return null;
              return (
                <fieldset key={g} className="flex flex-col gap-1.5">
                  <legend className="mb-1 flex items-center gap-2 text-sm font-medium text-gray-700 dark:text-gray-300">
                    <span className="h-3 w-3 rounded-full" style={{ backgroundColor: GROUP_COLORS[g] }} aria-hidden />
                    {t(`occurrenceGroup.${g}`)}
                  </legend>
                  {inGroup.map((x) => {
                    const disabled = nature !== undefined && x.caseType !== nature;
                    return (
                      <label
                        key={x.code}
                        className={`flex items-center gap-2 text-sm ${disabled ? "text-gray-300 dark:text-gray-600" : "text-gray-700 dark:text-gray-400"}`}
                      >
                        <input
                          type="checkbox"
                          checked={chosen.includes(x.code)}
                          disabled={disabled}
                          onChange={() => setChosen((c) => toggle(c, x.code))}
                        />
                        {t(`occurrenceType.${x.code}`)}
                      </label>
                    );
                  })}
                </fieldset>
              );
            })}
          </div>
          {nature && (
            <p className="text-theme-xs text-gray-500 dark:text-gray-400">
              {t(civil ? "occurrenceForm.natureCivil" : "occurrenceForm.natureCriminal")}
            </p>
          )}
        </ComponentCard>

        <ComponentCard title={t("occurrenceForm.whenTitle")}>
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
            <div>
              <Label htmlFor="occurred-from">{t("occurrenceForm.occurredFrom")}</Label>
              <input id="occurred-from" type="datetime-local" className={inputClass} value={occurredFrom}
                onChange={(e) => setOccurredFrom(e.target.value)} required />
            </div>
            <div>
              <label className="mb-1.5 flex items-center gap-2 text-sm font-medium text-gray-700 dark:text-gray-400">
                <input type="checkbox" checked={foundLater} onChange={(e) => setFoundLater(e.target.checked)} />
                {t("occurrenceForm.foundLater")}
              </label>
              {foundLater && (
                <input type="datetime-local" className={inputClass} value={occurredTo}
                  onChange={(e) => setOccurredTo(e.target.value)} aria-label={t("occurrenceForm.occurredTo")} />
              )}
            </div>
          </div>
        </ComponentCard>

        <ComponentCard title={t("occurrenceForm.whereTitle")} desc={t("occurrenceForm.whereDesc")}>
          <LocationPicker value={place} onChange={setPlace} />
          <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
            <div>
              <Label htmlFor="place-address">{t("occurrenceForm.address")}</Label>
              <input id="place-address" className={inputClass} value={place.address ?? ""} disabled={place.existingId !== undefined}
                onChange={(e) => setPlace({ ...place, address: e.target.value })} />
            </div>
            <div>
              <Label htmlFor="place-neighbourhood">{t("occurrenceForm.neighbourhood")}</Label>
              <input id="place-neighbourhood" className={inputClass} value={place.neighbourhood ?? ""} disabled={place.existingId !== undefined}
                onChange={(e) => setPlace({ ...place, neighbourhood: e.target.value })} />
            </div>
            <div>
              <Label htmlFor="place-type">{t("occurrenceForm.placeType")}</Label>
              <select id="place-type" className={inputClass} value={place.placeType ?? ""} disabled={place.existingId !== undefined}
                onChange={(e) => setPlace({ ...place, placeType: e.target.value || undefined })}>
                <option value="">—</option>
                {PLACE_TYPES.map((p) => (
                  <option key={p} value={p}>{t(`placeType.${p}`)}</option>
                ))}
              </select>
            </div>
            <div className="md:col-span-3">
              <Label htmlFor="place-name">{t("occurrenceForm.placeName")}</Label>
              <input id="place-name" className={inputClass} value={place.name ?? ""} disabled={place.existingId !== undefined}
                placeholder={t("occurrenceForm.placeNameHint")}
                onChange={(e) => setPlace({ ...place, name: e.target.value })} />
            </div>
          </div>
          {place.existingId !== undefined && (
            <p className="text-theme-xs text-brand-600 dark:text-brand-400">{t("occurrenceForm.reusing")}</p>
          )}
        </ComponentCard>

        <ComponentCard title={t("occurrenceForm.detailsTitle")}>
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
            <div>
              <Label htmlFor="agency">{t("occurrenceForm.agency")}</Label>
              <input id="agency" className={inputClass} value={agency} placeholder="DP-005"
                onChange={(e) => setAgency(e.target.value)} />
            </div>
            <div>
              <Label htmlFor="agency-number">{t("occurrenceForm.agencyNumber")}</Label>
              <input id="agency-number" className={inputClass} value={agencyNumber} placeholder="005-01234/2026"
                onChange={(e) => setAgencyNumber(e.target.value)} />
            </div>
            <div>
              <Label htmlFor="status">{t("occurrenceForm.status")}</Label>
              <select id="status" className={inputClass} value={status} onChange={(e) => setStatus(e.target.value)}>
                {STATUSES.map((s) => (
                  <option key={s} value={s}>{t(`occurrenceStatus.${s}`)}</option>
                ))}
              </select>
            </div>
            <div>
              <Label htmlFor="victims">{t("occurrenceForm.victimCount")}</Label>
              <input id="victims" type="number" min={0} className={inputClass} value={victimCount}
                onChange={(e) => setVictimCount(e.target.value)} />
            </div>
            {!civil && (
              <>
                <div>
                  <Label htmlFor="offenders">{t("occurrenceForm.offenderCount")}</Label>
                  <input id="offenders" type="number" min={0} className={inputClass} value={offenderCount}
                    onChange={(e) => setOffenderCount(e.target.value)} />
                </div>
                <label className="flex items-center gap-2 self-end pb-3 text-sm text-gray-700 dark:text-gray-400">
                  <input type="checkbox" checked={attempted} onChange={(e) => setAttempted(e.target.checked)} />
                  {t("occurrenceForm.attempted")}
                </label>
              </>
            )}
          </div>
          <div>
            <Label htmlFor="description">{t("occurrenceForm.description")}</Label>
            <textarea id="description" rows={4} required value={description} onChange={(e) => setDescription(e.target.value)}
              className="w-full rounded-lg border border-gray-300 bg-transparent px-4 py-2.5 text-sm text-gray-800 shadow-theme-xs focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90" />
          </div>
        </ComponentCard>

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
