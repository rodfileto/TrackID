import { FormEvent, useState } from "react";
import { Link, useNavigate } from "react-router";
import { useTranslation } from "react-i18next";
import { ChevronLeftIcon, EyeCloseIcon, EyeIcon } from "../../icons";
import Label from "../form/Label";
import Input from "../form/input/InputField";
import Button from "../ui/button/Button";
import { register } from "../../services/auth";

export default function SignUpForm() {
  const { t } = useTranslation();
  const [showPassword, setShowPassword] = useState(false);
  const [form, setForm] = useState({
    nome: "",
    ultimo_nome: "",
    matricula: "",
    cargo: "",
    username: "",
    password: "",
  });
  const [error, setError] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const navigate = useNavigate();

  function updateField(field: keyof typeof form, value: string) {
    setForm((current) => ({ ...current, [field]: value }));
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setIsSubmitting(true);

    try {
      await register(form);
      navigate("/signin", {
        replace: true,
        state: { message: t("auth.accountCreated") },
      });
    } catch (submitError) {
      setError(
        submitError instanceof Error
          ? submitError.message
          : t("auth.createError"),
      );
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <div className="flex flex-col flex-1 w-full overflow-y-auto lg:w-1/2 no-scrollbar">
      <div className="w-full max-w-md mx-auto mb-5 sm:pt-10">
        <Link
          to="/"
          className="inline-flex items-center text-sm text-gray-500 transition-colors hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300"
        >
          <ChevronLeftIcon className="size-5" />
          {t("auth.backToDashboard")}
        </Link>
      </div>
      <div className="flex flex-col justify-center flex-1 w-full max-w-md mx-auto">
        <div className="mb-5 sm:mb-8">
          <h1 className="mb-2 font-semibold text-gray-800 text-title-sm dark:text-white/90 sm:text-title-md">
            {t("auth.createTitle")}
          </h1>
          <p className="text-sm text-gray-500 dark:text-gray-400">
            {t("auth.createSubtitle")}
          </p>
        </div>

        <form onSubmit={handleSubmit}>
          <div className="space-y-5">
            <div className="grid grid-cols-1 gap-5 sm:grid-cols-2">
              <div>
                <Label>
                  {t("auth.firstName")} <span className="text-error-500">*</span>
                </Label>
                <Input
                  name="nome"
                  placeholder={t("auth.firstNamePlaceholder")}
                  value={form.nome}
                  onChange={(event) => updateField("nome", event.target.value)}
                  required
                />
              </div>
              <div>
                <Label>
                  {t("auth.lastName")} <span className="text-error-500">*</span>
                </Label>
                <Input
                  name="ultimo_nome"
                  placeholder={t("auth.lastNamePlaceholder")}
                  value={form.ultimo_nome}
                  onChange={(event) =>
                    updateField("ultimo_nome", event.target.value)
                  }
                  required
                />
              </div>
            </div>

            <div>
              <Label>
                {t("auth.matricula")} <span className="text-error-500">*</span>
              </Label>
              <Input
                name="matricula"
                placeholder={t("auth.matriculaStaffPlaceholder")}
                value={form.matricula}
                onChange={(event) => updateField("matricula", event.target.value)}
                required
              />
            </div>

            <div>
              <Label>
                {t("auth.role")} <span className="text-error-500">*</span>
              </Label>
              <Input
                name="cargo"
                placeholder={t("auth.rolePlaceholder")}
                value={form.cargo}
                onChange={(event) => updateField("cargo", event.target.value)}
                required
              />
            </div>

            <div>
              <Label>
                {t("auth.username")} <span className="text-error-500">*</span>
              </Label>
              <Input
                name="username"
                placeholder={t("auth.usernamePlaceholder")}
                value={form.username}
                onChange={(event) => updateField("username", event.target.value)}
                required
              />
              <p className="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                {t("auth.emailDerived")}
              </p>
            </div>

            <div>
              <Label>
                {t("auth.password")} <span className="text-error-500">*</span>
              </Label>
              <div className="relative">
                <Input
                  name="password"
                  placeholder={t("auth.passwordMin")}
                  type={showPassword ? "text" : "password"}
                  value={form.password}
                  onChange={(event) => updateField("password", event.target.value)}
                  required
                  minLength={8}
                />
                <button
                  type="button"
                  onClick={() => setShowPassword((visible) => !visible)}
                  aria-label={showPassword ? t("auth.hidePassword") : t("auth.showPassword")}
                  className="absolute z-30 -translate-y-1/2 right-4 top-1/2"
                >
                  {showPassword ? (
                    <EyeIcon className="fill-gray-500 dark:fill-gray-400 size-5" />
                  ) : (
                    <EyeCloseIcon className="fill-gray-500 dark:fill-gray-400 size-5" />
                  )}
                </button>
              </div>
            </div>

            {error && (
              <p className="text-sm text-error-500" role="alert">
                {error}
              </p>
            )}

            <Button className="w-full" size="sm" disabled={isSubmitting}>
              {isSubmitting ? t("auth.creatingAccount") : t("auth.createTitle")}
            </Button>
          </div>
        </form>

        <p className="mt-5 text-sm font-normal text-center text-gray-700 dark:text-gray-400 sm:text-start">
          {t("auth.haveAccount")}{" "}
          <Link
            to="/signin"
            className="text-brand-500 hover:text-brand-600 dark:text-brand-400"
          >
            {t("auth.signIn")}
          </Link>
        </p>
      </div>
    </div>
  );
}