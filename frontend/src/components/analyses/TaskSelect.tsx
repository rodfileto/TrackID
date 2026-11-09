import { useTranslation } from "react-i18next";
import { comingSoonTasks, offeredTasks, taskModality, type AnalysisType } from "../../services/analyses";
import { typeKey } from "./labels";

/** The task a request asks of a case: the offered examinations, limited to modality when the case
 * is known, and the ones coming soon, listed but not selectable. optional allows none, for a request
 * that formalizes a scene before anything follows from it. */
export default function TaskSelect({ id, value, onChange, modality, className, optional }: {
  id?: string;
  value: string;
  onChange: (task: AnalysisType) => void;
  modality?: string;
  className?: string;
  optional?: boolean;
}) {
  const { t } = useTranslation();
  const fits = (task: AnalysisType) => !modality || taskModality(task) === modality;
  return (
    <select id={id} required={!optional} className={className} value={value} onChange={(e) => onChange(e.target.value as AnalysisType)}>
      <option value="" disabled={!optional}>
        {t(optional ? "requests.noTask" : "requests.chooseTask")}
      </option>
      {offeredTasks.filter(fits).map((task) => (
        <option key={task} value={task}>
          {t(typeKey(task))}
        </option>
      ))}
      {comingSoonTasks.filter(fits).map((task) => (
        <option key={task} value={task} disabled>
          {t(typeKey(task))} ({t("comingSoon.label")})
        </option>
      ))}
    </select>
  );
}
