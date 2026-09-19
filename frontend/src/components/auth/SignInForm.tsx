import { FormEvent, useState } from "react";
import { Link, useLocation, useNavigate } from "react-router";
import { useTranslation } from "react-i18next";
import { ChevronLeftIcon, EyeCloseIcon, EyeIcon } from "../../icons";
import Label from "../form/Label";
import Input from "../form/input/InputField";
import Checkbox from "../form/input/Checkbox";
import Button from "../ui/button/Button";
import { login } from "../../services/auth";

export default function SignInForm() {
  const { t } = useTranslation();
  const [showPassword, setShowPassword] = useState(false);
  const [isChecked, setIsChecked] = useState(false);
  const [matricula, setMatricula] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const navigate = useNavigate();
  const location = useLocation();
  const signupMessage = (location.state?.message as string | undefined) ?? "";

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setIsSubmitting(true);

    try {
      await login(matricula, password, isChecked);
      const destination = location.state?.from?.pathname ?? "/";
      navigate(destination, { replace: true });
    } catch (submitError) {
      setError(
        submitError instanceof Error
          ? submitError.message
          : t("auth.signInError"),
      );
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <div className="flex flex-col flex-1">
      <div className="w-full max-w-md pt-10 mx-auto">
        <Link
          to="/"
          className="inline-flex items-center text-sm text-gray-500 transition-colors hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300"
        >
          <ChevronLeftIcon className="size-5" />
          {t("auth.backToDashboard")}
        </Link>
      </div>
      <div className="flex flex-col justify-center flex-1 w-full max-w-md mx-auto">
        <div>
          <div className="mb-5 sm:mb-8">
            <h1 className="mb-2 font-semibold text-gray-800 text-title-sm dark:text-white/90 sm:text-title-md">
              {t("auth.signInTitle")}
            </h1>
            <p className="text-sm text-gray-500 dark:text-gray-400">
              {t("auth.signInSubtitle")}
            </p>
          </div>
          {signupMessage && (
            <div className="mb-5 rounded-lg border border-success-200 bg-success-50 px-4 py-3 text-sm text-success-700 dark:border-success-500/30 dark:bg-success-500/10 dark:text-success-400">
              {signupMessage}
            </div>
          )}
          <div>
            <form onSubmit={handleSubmit}>
              <div className="space-y-6">
                <div>
                  <Label>
                    {t("auth.matricula")} <span className="text-error-500">*</span>{" "}
                  </Label>
                  <Input
                    id="matricula"
                    name="matricula"
                    placeholder={t("auth.matriculaPlaceholder")}
                    value={matricula}
                    onChange={(event) => setMatricula(event.target.value)}
                  />
                </div>
                <div>
                  <Label>
                    {t("auth.password")} <span className="text-error-500">*</span>{" "}
                  </Label>
                  <div className="relative">
                    <Input
                      type={showPassword ? "text" : "password"}
                      placeholder={t("auth.passwordPlaceholder")}
                      id="password"
                      name="password"
                      value={password}
                      onChange={(event) => setPassword(event.target.value)}
                    />
                    <span
                      onClick={() => setShowPassword(!showPassword)}
                      className="absolute z-30 -translate-y-1/2 cursor-pointer right-4 top-1/2"
                    >
                      {showPassword ? (
                        <EyeIcon className="fill-gray-500 dark:fill-gray-400 size-5" />
                      ) : (
                        <EyeCloseIcon className="fill-gray-500 dark:fill-gray-400 size-5" />
                      )}
                    </span>
                  </div>
                </div>
                {error && (
                  <p className="text-sm text-error-500" role="alert">
                    {error}
                  </p>
                )}
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <Checkbox checked={isChecked} onChange={setIsChecked} />
                    <span className="block font-normal text-gray-700 text-theme-sm dark:text-gray-400">
                      {t("auth.keepLoggedIn")}
                    </span>
                  </div>
                  <Link
                    to="/reset-password"
                    className="text-sm text-brand-500 hover:text-brand-600 dark:text-brand-400"
                  >
                    {t("auth.forgotPassword")}
                  </Link>
                </div>
                <div>
                  <Button className="w-full" size="sm" disabled={isSubmitting}>
                    {isSubmitting ? t("auth.signingIn") : t("auth.signIn")}
                  </Button>
                </div>
              </div>
            </form>

            <div className="mt-5">
              <p className="text-sm font-normal text-center text-gray-700 dark:text-gray-400 sm:text-start">
                {t("auth.noAccount")} {""}
                <Link
                  to="/signup"
                  className="text-brand-500 hover:text-brand-600 dark:text-brand-400"
                >
                  {t("auth.signUp")}
                </Link>
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
