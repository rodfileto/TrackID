import { useEffect, useState } from "react";
import { Link, useParams } from "react-router";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import PageMeta from "../components/common/PageMeta";
import PageBreadcrumb from "../components/common/PageBreadCrumb";
import ComponentCard from "../components/common/ComponentCard";
import Badge from "../components/ui/badge/Badge";
import Button from "../components/ui/button/Button";
import { extension } from "../extensions";
import { getOperation, updateOperation, type Operation } from "../services/operations";
import { operationInputClass } from "./Operations";

/** One operation, with the panel an organization attached to it, if any
 * (Extension.operationPanels, by the operation's slug). */
export default function OperationDetail() {
  const { t } = useTranslation();
  const { slug = "" } = useParams();
  const [operation, setOperation] = useState<Operation | null>(null);
  const [error, setError] = useState("");
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setOperation(null);
    setError("");
    getOperation(slug)
      .then((loaded) => {
        if (!cancelled) setOperation(loaded);
      })
      .catch((err) => {
        if (!cancelled) setError(err instanceof Error ? err.message : t("operations.loadError"));
      });
    return () => {
      cancelled = true;
    };
  }, [slug, t]);

  async function save(changes: Partial<Pick<Operation, "name" | "description" | "status">>) {
    if (!operation) return;
    setBusy(true);
    try {
      const updated = await updateOperation(operation.slug, {
        name: operation.name,
        description: operation.description,
        status: operation.status,
        ...changes,
      });
      setOperation(updated);
      setEditing(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("operations.saveError"));
    } finally {
      setBusy(false);
    }
  }

  const panel = extension.operationPanels?.[slug];

  return (
    <>
      <PageMeta title={`${operation?.name ?? t("operations.title")} | TrackID`} description={t("operations.desc")} />
      <PageBreadcrumb pageTitle={operation?.name ?? t("operations.title")} />
      <p className="mb-4 text-sm">
        <Link to="/intelligence/operations" className="text-brand-500 hover:underline">
          {t("operations.back")}
        </Link>
      </p>
      {error && <p className="mb-4 text-sm text-error-500">{error}</p>}
      {operation && (
        <div className="space-y-6">
          <ComponentCard title={operation.name} desc={operation.slug}>
            {editing ? (
              <div className="space-y-4">
                <label className="block text-sm text-gray-700 dark:text-gray-300">
                  {t("operations.name")}
                  <input className={`mt-1 ${operationInputClass}`} value={name} onChange={(e) => setName(e.target.value)} maxLength={200} />
                </label>
                <label className="block text-sm text-gray-700 dark:text-gray-300">
                  {t("operations.description")}
                  <textarea className={`mt-1 ${operationInputClass}`} rows={3} value={description} onChange={(e) => setDescription(e.target.value)} />
                </label>
                <div className="flex gap-2">
                  <Button size="sm" disabled={busy || !name.trim()} onClick={() => save({ name, description })}>
                    {t("operations.save")}
                  </Button>
                  <Button size="sm" variant="outline" disabled={busy} onClick={() => setEditing(false)}>
                    {t("operations.cancel")}
                  </Button>
                </div>
              </div>
            ) : (
              <div className="space-y-4">
                <Badge size="sm" color={operation.status === "ACTIVE" ? "success" : "light"}>
                  {t(`operations.status.${operation.status}`)}
                </Badge>
                {operation.description && (
                  <p className="whitespace-pre-wrap text-sm text-gray-700 dark:text-gray-300">{operation.description}</p>
                )}
                <div className="flex gap-2">
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={busy}
                    onClick={() => {
                      setName(operation.name);
                      setDescription(operation.description);
                      setEditing(true);
                    }}
                  >
                    {t("operations.edit")}
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={busy}
                    onClick={() => save({ status: operation.status === "ACTIVE" ? "CLOSED" : "ACTIVE" })}
                  >
                    {operation.status === "ACTIVE" ? t("operations.close") : t("operations.reopen")}
                  </Button>
                </div>
              </div>
            )}
          </ComponentCard>
          {panel}
        </div>
      )}
    </>
  );
}
