import PageMeta from "../../components/common/PageMeta";
import AuthLayout from "./AuthPageLayout";
import SignInForm from "../../components/auth/SignInForm";
import { Navigate } from "react-router";
import { isAuthenticated } from "../../auth";

export default function SignIn() {
  if (isAuthenticated()) {
    return <Navigate to="/" replace />;
  }
  return (
    <>
      <PageMeta
        title="Sign In | TrackID"
        description="Sign in to the TrackID dashboard"
      />
      <AuthLayout>
        <SignInForm />
      </AuthLayout>
    </>
  );
}
