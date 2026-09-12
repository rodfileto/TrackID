import { FormEvent, useState } from "react";
import { Link, useNavigate } from "react-router";
import { ChevronLeftIcon, EyeCloseIcon, EyeIcon } from "../../icons";
import Label from "../form/Label";
import Input from "../form/input/InputField";
import Button from "../ui/button/Button";
import { register } from "../../services/auth";

export default function SignUpForm() {
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
        state: { message: "Account created. Sign in with your matrícula." },
      });
    } catch (submitError) {
      setError(
        submitError instanceof Error
          ? submitError.message
          : "Could not create account",
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
          Back to dashboard
        </Link>
      </div>
      <div className="flex flex-col justify-center flex-1 w-full max-w-md mx-auto">
        <div className="mb-5 sm:mb-8">
          <h1 className="mb-2 font-semibold text-gray-800 text-title-sm dark:text-white/90 sm:text-title-md">
            Create account
          </h1>
          <p className="text-sm text-gray-500 dark:text-gray-400">
            Register your staff profile to access TrackID.
          </p>
        </div>

        <form onSubmit={handleSubmit}>
          <div className="space-y-5">
            <div className="grid grid-cols-1 gap-5 sm:grid-cols-2">
              <div>
                <Label>
                  Nome <span className="text-error-500">*</span>
                </Label>
                <Input
                  name="nome"
                  placeholder="Your first name"
                  value={form.nome}
                  onChange={(event) => updateField("nome", event.target.value)}
                  required
                />
              </div>
              <div>
                <Label>
                  Último nome <span className="text-error-500">*</span>
                </Label>
                <Input
                  name="ultimo_nome"
                  placeholder="Your last name"
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
                Matrícula <span className="text-error-500">*</span>
              </Label>
              <Input
                name="matricula"
                placeholder="Your staff registration number"
                value={form.matricula}
                onChange={(event) => updateField("matricula", event.target.value)}
                required
              />
            </div>

            <div>
              <Label>
                Cargo <span className="text-error-500">*</span>
              </Label>
              <Input
                name="cargo"
                placeholder="Your role"
                value={form.cargo}
                onChange={(event) => updateField("cargo", event.target.value)}
                required
              />
            </div>

            <div>
              <Label>
                Username <span className="text-error-500">*</span>
              </Label>
              <Input
                name="username"
                placeholder="Your username"
                value={form.username}
                onChange={(event) => updateField("username", event.target.value)}
                required
              />
              <p className="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                Your email will be derived from your username.
              </p>
            </div>

            <div>
              <Label>
                Password <span className="text-error-500">*</span>
              </Label>
              <div className="relative">
                <Input
                  name="password"
                  placeholder="At least 8 characters"
                  type={showPassword ? "text" : "password"}
                  value={form.password}
                  onChange={(event) => updateField("password", event.target.value)}
                  required
                  minLength={8}
                />
                <button
                  type="button"
                  onClick={() => setShowPassword((visible) => !visible)}
                  aria-label={showPassword ? "Hide password" : "Show password"}
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
              {isSubmitting ? "Creating account..." : "Create account"}
            </Button>
          </div>
        </form>

        <p className="mt-5 text-sm font-normal text-center text-gray-700 dark:text-gray-400 sm:text-start">
          Already have an account?{" "}
          <Link
            to="/signin"
            className="text-brand-500 hover:text-brand-600 dark:text-brand-400"
          >
            Sign in
          </Link>
        </p>
      </div>
    </div>
  );
}