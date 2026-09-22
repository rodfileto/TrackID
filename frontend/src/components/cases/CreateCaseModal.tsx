import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { Modal } from "../ui/modal";
import Button from "../ui/button/Button";
import Label from "../form/Label";
import Input from "../form/input/InputField";
import { createCase, type Case } from "../../services/cases";

const CASE_TYPES = [
  { value: "", labelKey: "createCase.selectType" },
  { value: "CRIMINAL", labelKey: "caseType.criminal" },
  { value: "CIVIL", labelKey: "caseType.civil" },
];

const MODALITIES = [
  { value: "", labelKey: "createCase.selectModality" },
  { value: "FACIAL", labelKey: "modality.facial" },
  { value: "FINGERPRINT", labelKey: "modality.fingerprint" },
];

interface CreateCaseModalProps {
  isOpen: boolean;
  onClose: () => void;
  onCreated: (created: Case) => void;
}

export default function CreateCaseModal({
  isOpen,
  onClose,
  onCreated,
}: CreateCaseModalProps) {
  const { t } = useTranslation();
  const [caseType, setCaseType] = useState("");
  const [modality, setModality] = useState("");
  const [description, setDescription] = useState("");
  const [creating, setCreating] = useState(false);

  function reset() {
    setCaseType("");
    setModality("");
    setDescription("");
  }

  function handleClose() {
    if (creating) return;
    reset();
    onClose();
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setCreating(true);
    try {
      const created = await createCase({ caseType, modality, description });
      reset();
      onCreated(created);
      onClose();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("createCase.error"));
    } finally {
      setCreating(false);
    }
  }

  return (
    <Modal isOpen={isOpen} onClose={handleClose} className="max-w-[500px] m-4">
      <div className="relative w-full p-4 overflow-y-auto bg-white no-scrollbar rounded-3xl dark:bg-gray-900 lg:p-8">
        <div className="px-2 pr-14">
          <h4 className="mb-2 text-2xl font-semibold text-gray-800 dark:text-white/90">
            {t("cases.newCase")}
          </h4>
          <p className="mb-6 text-sm text-gray-500 dark:text-gray-400">
            {t("createCase.autoNumber")}
          </p>
        </div>

        <form onSubmit={handleSubmit} className="flex flex-col">
          <div className="px-2 space-y-5">
            <div>
              <Label>{t("cases.columns.type")}</Label>
              <select
                value={caseType}
                onChange={(event) => setCaseType(event.target.value)}
                required
                className="h-11 w-full rounded-lg border border-gray-300 bg-transparent px-4 py-2.5 text-sm text-gray-800 shadow-theme-xs focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90 dark:focus:border-brand-800"
              >
                {CASE_TYPES.map((option) => (
                  <option key={option.value} value={option.value}>
                    {t(option.labelKey)}
                  </option>
                ))}
              </select>
            </div>

            <div>
              <Label>{t("cases.columns.modality")}</Label>
              <select
                value={modality}
                onChange={(event) => setModality(event.target.value)}
                required
                className="h-11 w-full rounded-lg border border-gray-300 bg-transparent px-4 py-2.5 text-sm text-gray-800 shadow-theme-xs focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90 dark:focus:border-brand-800"
              >
                {MODALITIES.map((option) => (
                  <option key={option.value} value={option.value}>
                    {t(option.labelKey)}
                  </option>
                ))}
              </select>
            </div>

            <div>
              <Label>{t("cases.columns.description")}</Label>
              <Input
                name="description"
                placeholder={t("createCase.descriptionPlaceholder")}
                value={description}
                onChange={(event) => setDescription(event.target.value)}
                required
              />
            </div>
          </div>

          <div className="flex items-center gap-3 px-2 mt-6 lg:justify-end">
            <button
              type="button"
              onClick={handleClose}
              className="inline-flex items-center justify-center gap-2 rounded-lg transition px-4 py-3 text-sm bg-white text-gray-700 ring-1 ring-inset ring-gray-300 hover:bg-gray-50 dark:bg-gray-800 dark:text-gray-400 dark:ring-gray-700 dark:hover:bg-white/[0.03] dark:hover:text-gray-300"
            >
              {t("common.cancel")}
            </button>
            <Button size="sm" disabled={creating}>
              {creating ? t("createCase.creating") : t("createCase.create")}
            </Button>
          </div>
        </form>
      </div>
    </Modal>
  );
}
