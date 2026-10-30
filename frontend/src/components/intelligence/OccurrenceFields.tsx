import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import ComponentCard from "../common/ComponentCard";
import Label from "../form/Label";
import LocationPicker from "./LocationPicker";
import { GROUP_COLORS } from "./groupColors";
import type { OccurrenceType } from "../../services/occurrences";
import { occurrenceNature, type OccurrenceDraft } from "./occurrenceDraft";

const GROUPS = Object.keys(GROUP_COLORS);
const STATUSES = ["REGISTERED", "UNDER_INVESTIGATION", "INQUIRY_OPENED", "CLOSED"];
const PLACE_TYPES = ["PUBLIC_ROAD", "RESIDENCE", "COMMERCE", "BANK", "GOVERNMENT_OFFICE", "TRANSPORT", "RURAL", "OTHER"];

const inputClass =
  "h-11 w-full rounded-lg border border-gray-300 bg-transparent px-4 text-sm text-gray-800 shadow-theme-xs placeholder:text-gray-400 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90";

function toggle(list: string[], code: string): string[] {
  return list.includes(code) ? list.filter((c) => c !== code) : [...list, code];
}

/** The occurrence form's cards -- what, when, where, details -- shared by the occurrence page and
 * request registration. fixedNature limits the types to one nature (a case's). */
export default function OccurrenceFields({
  value,
  onChange,
  types,
  fixedNature,
}: {
  value: OccurrenceDraft;
  onChange: (next: OccurrenceDraft) => void;
  types: OccurrenceType[];
  fixedNature?: string;
}) {
  const { t } = useTranslation();
  const set = (patch: Partial<OccurrenceDraft>) => onChange({ ...value, ...patch });
  const nature = useMemo(() => occurrenceNature(value, types, fixedNature), [value, types, fixedNature]);
  const civil = nature === "CIVIL";
  const place = value.place;

  return (
    <>
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
                          checked={value.chosen.includes(x.code)}
                          disabled={disabled}
                          onChange={() => set({ chosen: toggle(value.chosen, x.code) })}
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
              <input id="occurred-from" type="datetime-local" className={inputClass} value={value.occurredFrom}
                onChange={(e) => set({ occurredFrom: e.target.value })} required />
            </div>
            <div>
              <label className="mb-1.5 flex items-center gap-2 text-sm font-medium text-gray-700 dark:text-gray-400">
                <input type="checkbox" checked={value.foundLater} onChange={(e) => set({ foundLater: e.target.checked })} />
                {t("occurrenceForm.foundLater")}
              </label>
              {value.foundLater && (
                <input type="datetime-local" className={inputClass} value={value.occurredTo}
                  onChange={(e) => set({ occurredTo: e.target.value })} aria-label={t("occurrenceForm.occurredTo")} />
              )}
            </div>
          </div>
        </ComponentCard>

        <ComponentCard title={t("occurrenceForm.whereTitle")} desc={t("occurrenceForm.whereDesc")}>
          <LocationPicker value={place} onChange={(p) => set({ place: p })} />
          <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
            <div>
              <Label htmlFor="place-address">{t("occurrenceForm.address")}</Label>
              <input id="place-address" className={inputClass} value={place.address ?? ""} disabled={place.existingId !== undefined}
                onChange={(e) => set({ place: { ...place, address: e.target.value } })} />
            </div>
            <div>
              <Label htmlFor="place-neighbourhood">{t("occurrenceForm.neighbourhood")}</Label>
              <input id="place-neighbourhood" className={inputClass} value={place.neighbourhood ?? ""} disabled={place.existingId !== undefined}
                onChange={(e) => set({ place: { ...place, neighbourhood: e.target.value } })} />
            </div>
            <div>
              <Label htmlFor="place-type">{t("occurrenceForm.placeType")}</Label>
              <select id="place-type" className={inputClass} value={place.placeType ?? ""} disabled={place.existingId !== undefined}
                onChange={(e) => set({ place: { ...place, placeType: e.target.value || undefined } })}>
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
                onChange={(e) => set({ place: { ...place, name: e.target.value } })} />
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
              <input id="agency" className={inputClass} value={value.agency} placeholder="DP-005"
                onChange={(e) => set({ agency: e.target.value })} />
            </div>
            <div>
              <Label htmlFor="agency-number">{t("occurrenceForm.agencyNumber")}</Label>
              <input id="agency-number" className={inputClass} value={value.agencyNumber} placeholder="005-01234/2026"
                onChange={(e) => set({ agencyNumber: e.target.value })} />
            </div>
            <div>
              <Label htmlFor="inquiry-number">{t("occurrenceForm.inquiryNumber")}</Label>
              <input id="inquiry-number" className={inputClass} value={value.inquiryNumber} placeholder="IPL 005-00123/2026"
                onChange={(e) => set({ inquiryNumber: e.target.value })} />
            </div>
            <div>
              <Label htmlFor="status">{t("occurrenceForm.status")}</Label>
              <select id="status" className={inputClass} value={value.status} onChange={(e) => set({ status: e.target.value })}>
                {STATUSES.map((s) => (
                  <option key={s} value={s}>{t(`occurrenceStatus.${s}`)}</option>
                ))}
              </select>
            </div>
            <div>
              <Label htmlFor="victims">{t("occurrenceForm.victimCount")}</Label>
              <input id="victims" type="number" min={0} className={inputClass} value={value.victimCount}
                onChange={(e) => set({ victimCount: e.target.value })} />
            </div>
            {!civil && (
              <>
                <div>
                  <Label htmlFor="offenders">{t("occurrenceForm.offenderCount")}</Label>
                  <input id="offenders" type="number" min={0} className={inputClass} value={value.offenderCount}
                    onChange={(e) => set({ offenderCount: e.target.value })} />
                </div>
                <label className="flex items-center gap-2 self-end pb-3 text-sm text-gray-700 dark:text-gray-400">
                  <input type="checkbox" checked={value.attempted} onChange={(e) => set({ attempted: e.target.checked })} />
                  {t("occurrenceForm.attempted")}
                </label>
              </>
            )}
          </div>
          <div>
            <Label htmlFor="description">{t("occurrenceForm.description")}</Label>
            <textarea id="description" rows={4} required value={value.description} onChange={(e) => set({ description: e.target.value })}
              className="w-full rounded-lg border border-gray-300 bg-transparent px-4 py-2.5 text-sm text-gray-800 shadow-theme-xs focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90" />
          </div>
        </ComponentCard>

    </>
  );
}
