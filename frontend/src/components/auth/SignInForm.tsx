import { FormEvent, useState } from "react";
import { useLocation, useNavigate } from "react-router";
import { useTranslation } from "react-i18next";
import { EyeCloseIcon, EyeIcon } from "../../icons";
import Label from "../form/Label";
import Input from "../form/input/InputField";
import Checkbox from "../form/input/Checkbox";
import Button from "../ui/button/Button";
import { login } from "../../services/auth";

export default function SignInForm() {
  const { t } = useTranslation();
  const [showPassword, setShowPassword] = useState(false);
  const [isChecked, setIsChecked] = useState(false);
  const [staffNumber, setStaffNumber] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const navigate = useNavigate();
  const location = useLocation();

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setIsSubmitting(true);

    try {
      await login(staffNumber, password, isChecked);
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
          <div>
            <form onSubmit={handleSubmit}>
              <div className="space-y-6">
                <div>
                  <Label>
                    {t("auth.staffNumber")} <span className="text-error-500">*</span>{" "}
                  </Label>
                  <Input
                    id="staff_number"
                    name="staff_number"
                    placeholder={t("auth.staffNumberPlaceholder")}
                    value={staffNumber}
                    onChange={(event) => setStaffNumber(event.target.value)}
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
                {t("auth.accountsByAdmin")}
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
