import { useTranslation } from "react-i18next";

/** A register's biographic data as labelled fields, for comparing one record with another. */
export default function RegisterFacts({ birthDate, parent1Name, parent2Name }: { birthDate?: string; parent1Name: string; parent2Name: string }) {
  const { t } = useTranslation();
  const facts: [string, string | undefined][] = [
    [t("identityConflict.compare.birthDate"), birthDate],
    [t("identityConflict.compare.parent1"), parent1Name],
    [t("identityConflict.compare.parent2"), parent2Name],
  ];
  return (
    <dl className="grid gap-x-6 gap-y-2 sm:grid-cols-3">
      {facts.map(([label, value]) => (
        <div key={label}>
          <dt className="text-xs text-gray-500 dark:text-gray-400">{label}</dt>
          <dd className="text-sm text-gray-800 dark:text-white/90">{value || "—"}</dd>
        </div>
      ))}
    </dl>
  );
}
