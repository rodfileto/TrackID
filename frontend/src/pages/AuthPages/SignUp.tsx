import PageMeta from "../../components/common/PageMeta";
import AuthLayout from "./AuthPageLayout";
import SignUpForm from "../../components/auth/SignUpForm";
import { Navigate } from "react-router";
import { isAuthenticated } from "../../auth";

export default function SignUp() {
  if (isAuthenticated()) {
    return <Navigate to="/" replace />;
  }
  return (
    <>
      <PageMeta
        title="Sign Up | TrackID"
        description="Create a TrackID account"
      />
      <AuthLayout>
        <SignUpForm />
      </AuthLayout>
    </>
  );
}
