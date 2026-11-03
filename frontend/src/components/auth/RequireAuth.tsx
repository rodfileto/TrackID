import { Navigate, Outlet, useLocation } from "react-router";
import { getToken } from "../../services/auth";
import { CurrentUserProvider } from "../../context/CurrentUserContext";

export default function RequireAuth() {
  const location = useLocation();

  if (!getToken()) {
    return <Navigate to="/signin" replace state={{ from: location }} />;
  }

  return (
    <CurrentUserProvider>
      <Outlet />
    </CurrentUserProvider>
  );
}