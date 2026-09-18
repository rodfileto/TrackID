import { BrowserRouter as Router, Routes, Route } from "react-router";
import { Toaster } from "sonner";
import SignIn from "./pages/AuthPages/SignIn";
import SignUp from "./pages/AuthPages/SignUp";
import NotFound from "./pages/OtherPage/NotFound";
import UserProfiles from "./pages/UserProfiles";
import AppLayout from "./layout/AppLayout";
import { ScrollToTop } from "./components/common/ScrollToTop";
import Home from "./pages/Dashboard/Home";
import RequireAuth from "./components/auth/RequireAuth";
import CriminalCases from "./pages/CriminalCases";
import CaseDetailPage from "./pages/CaseDetail";
import PersonLookup from "./pages/PersonLookup";
import PersonProfilePage from "./pages/PersonProfile";
import { useTheme } from "./context/ThemeContext";

export default function App() {
  const { theme } = useTheme();

  return (
    <>
      <Toaster theme={theme} position="top-right" richColors />
      <Router>
        <ScrollToTop />
        <Routes>
          {/* Dashboard Layout */}
          <Route element={<RequireAuth />}>
            <Route element={<AppLayout />}>
              <Route index path="/" element={<Home />} />
              <Route path="/profile" element={<UserProfiles />} />
              <Route path="/cases" element={<CriminalCases />} />
              <Route path="/cases/:caseId" element={<CaseDetailPage />} />
              <Route path="/persons" element={<PersonLookup />} />
              <Route path="/persons/:personId" element={<PersonProfilePage />} />
            </Route>
          </Route>

          {/* Auth Layout */}
          <Route path="/signin" element={<SignIn />} />
          <Route path="/signup" element={<SignUp />} />

          {/* Fallback Route */}
          <Route path="*" element={<NotFound />} />
        </Routes>
      </Router>
    </>
  );
}
