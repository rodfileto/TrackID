import { useTranslation } from "react-i18next";
import { comingSoonTasks, offeredTasks, taskModality, type AnalysisType } from "../../services/analyses";
import { typeKey } from "./labels";

/** The task a request asks of a case: the offered examinations, limited to modality when the case
 * is known, and the ones coming soon, listed but not selectable. */
export default function TaskSelect({ id, value, onChange, modality, className }: {
  id?: string;
  value: string;
  onChange: (task: AnalysisType) => void;
  modality?: string;
  className?: string;
}) {
  const { t } = useTranslation();
  const fits = (task: AnalysisType) => !modality || taskModality(task) === modality;
  return (
    <select id={id} required className={className} value={value} onChange={(e) => onChange(e.target.value as AnalysisType)}>
      <option value="" disabled>
        {t("requests.chooseTask")}
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
