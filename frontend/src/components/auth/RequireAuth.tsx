import { Navigate, Outlet, useLocation } from "react-router";
import { getToken } from "../../services/auth";

export default function RequireAuth() {
  const location = useLocation();

  if (!getToken()) {
    return <Navigate to="/signin" replace state={{ from: location }} />;
  }

  return <Outlet />;
}