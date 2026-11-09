import { type FormEvent, useEffect, useState } from "react";
import { Link } from "react-router";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import PageMeta from "../components/common/PageMeta";
import PageBreadcrumb from "../components/common/PageBreadCrumb";
import ComponentCard from "../components/common/ComponentCard";
import Badge from "../components/ui/badge/Badge";
import Button from "../components/ui/button/Button";
import { Table, TableBody, TableCell, TableHeader, TableRow } from "../components/ui/table";
import { createOperation, listOperations, type Operation } from "../services/operations";

const headerClass = "px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400";
const cellClass = "px-5 py-3 text-start text-theme-sm text-gray-700 dark:text-gray-300";
export const operationInputClass =
  "w-full rounded-lg border border-gray-200 bg-transparent px-4 py-2.5 text-sm text-gray-800 shadow-theme-xs focus:outline-hidden dark:border-gray-700 dark:text-white/90";

/** The intelligence operations, and registering a new one. */
export default function Operations() {
  const { t, i18n } = useTranslation();
  const [operations, setOperations] = useState<Operation[] | null>(null);
  const [error, setError] = useState("");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    listOperations()
      .then(setOperations)
      .catch((err) => setError(err instanceof Error ? err.message : t("operations.loadError")));
  }, [t]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    try {
      const created = await createOperation({ name, description });
      setOperations((current) => [created, ...(current ?? [])]);
      setName("");
      setDescription("");
      toast.success(t("operations.created"));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("operations.saveError"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <PageMeta title={`${t("operations.title")} | TrackID`} description={t("operations.desc")} />
      <PageBreadcrumb pageTitle={t("operations.title")} />
      {error && <p className="mb-4 text-sm text-error-500">{error}</p>}
      <div className="grid grid-cols-1 gap-6 xl:grid-cols-3">
        <div className="xl:col-span-2">
          <ComponentCard title={t("operations.title")} desc={t("operations.desc")}>
            {operations && operations.length === 0 && (
              <p className="text-sm text-gray-500 dark:text-gray-400">{t("operations.empty")}</p>
            )}
            {operations && operations.length > 0 && (
              <div className="overflow-x-auto">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableCell isHeader className={headerClass}>{t("operations.name")}</TableCell>
                      <TableCell isHeader className={headerClass}>{t("operations.statusLabel")}</TableCell>
                      <TableCell isHeader className={headerClass}>{t("operations.createdAt")}</TableCell>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {operations.map((operation) => (
                      <TableRow key={operation.id}>
                        <TableCell className={cellClass}>
                          <Link to={`/intelligence/operations/${operation.slug}`} className="text-brand-500 hover:underline">
                            {operation.name}
                          </Link>
                        </TableCell>
                        <TableCell className={cellClass}>
                          <Badge size="sm" color={operation.status === "ACTIVE" ? "success" : "light"}>
                            {t(`operations.status.${operation.status}`)}
                          </Badge>
                        </TableCell>
                        <TableCell className={cellClass}>
                          {new Date(operation.createdAt).toLocaleDateString(i18n.language)}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
          </ComponentCard>
        </div>
        <ComponentCard title={t("operations.create")}>
          <form onSubmit={submit} className="space-y-4">
            <label className="block text-sm text-gray-700 dark:text-gray-300">
              {t("operations.name")}
              <input className={`mt-1 ${operationInputClass}`} value={name} onChange={(e) => setName(e.target.value)} required maxLength={200} />
            </label>
            <label className="block text-sm text-gray-700 dark:text-gray-300">
              {t("operations.description")}
              <textarea className={`mt-1 ${operationInputClass}`} rows={3} value={description} onChange={(e) => setDescription(e.target.value)} />
            </label>
            <p className="text-theme-xs text-gray-500 dark:text-gray-400">{t("operations.sensitiveHint")}</p>
            <Button size="sm" type="submit" disabled={busy || !name.trim()}>
              {t("operations.create")}
            </Button>
          </form>
        </ComponentCard>
      </div>
    </>
  );
}
