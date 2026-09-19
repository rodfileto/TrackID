import { useTranslation } from "react-i18next";
import PageMeta from "../../components/common/PageMeta";
import AuthLayout from "./AuthPageLayout";
import SignInForm from "../../components/auth/SignInForm";

export default function SignIn() {
  const { t } = useTranslation();
  return (
    <>
      <PageMeta
        title={`${t("auth.signInTitle")} | TrackID`}
        description={t("auth.signInSubtitle")}
      />
      <AuthLayout>
        <SignInForm />
      </AuthLayout>
    </>
  );
}
