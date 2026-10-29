import { lazy, Suspense } from "react";
import { BrowserRouter as Router, Routes, Route } from "react-router";
import { Toaster } from "sonner";
import SignIn from "./pages/AuthPages/SignIn";
import NotFound from "./pages/OtherPage/NotFound";
import UserProfiles from "./pages/UserProfiles";
import AppLayout from "./layout/AppLayout";
import { ScrollToTop } from "./components/common/ScrollToTop";
import Home from "./pages/Dashboard/Home";
import RequireAuth from "./components/auth/RequireAuth";
import RequireCapability from "./components/auth/RequireCapability";
import CriminalCases from "./pages/CriminalCases";
import CaseDetailPage from "./pages/CaseDetail";
import PersonLookup from "./pages/PersonLookup";
import PersonProfilePage from "./pages/PersonProfile";
import Clusters from "./pages/Clusters";
import AnalysisWorkspace from "./pages/AnalysisWorkspace";
import MyAnalyses from "./pages/MyAnalyses";
import Distribution from "./pages/Distribution";
import Requests from "./pages/Requests";
import RegisterRequest from "./pages/RegisterRequest";
import ImageLab from "./pages/ImageLab";
import ComingSoon from "./pages/ComingSoon";
import Users from "./pages/Users";
// A lazy page whose code fails to load -- the tab predates a new build or a dev-server
// restart, so the chunk it asks for no longer exists -- reloads once to fetch the current
// version, instead of leaving a blank screen. The flag stops a reload loop when the failure
// is something else (offline, server down): the error then surfaces.
function lazyPage<T extends React.ComponentType>(load: () => Promise<{ default: T }>) {
  return lazy(async () => {
    const flag = "trackid.chunkReload";
    try {
      const page = await load();
      sessionStorage.removeItem(flag);
      return page;
    } catch (err) {
      if (!sessionStorage.getItem(flag)) {
        sessionStorage.setItem(flag, "1");
        window.location.reload();
        return new Promise<{ default: T }>(() => {});
      }
      throw err;
    }
  });
}

// The map pulls in MapLibre (about 1 MB): loaded only when the page is opened.
const OccurrenceMap = lazyPage(() => import("./pages/OccurrenceMap"));
const NewOccurrence = lazyPage(() => import("./pages/NewOccurrence"));
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
              <Route path="/cases/:caseId/analyses/:analysisId" element={<AnalysisWorkspace />} />
              <Route path="/analyses" element={<MyAnalyses />} />
              <Route path="/image-lab" element={<ImageLab />} />
              <Route path="/examinations/laboratory" element={<ComingSoon kind="laboratory" />} />
              <Route path="/examinations/crime-scene" element={<ComingSoon kind="crimeScene" />} />
              <Route element={<RequireCapability need="coordinator" />}>
                <Route path="/admin/distribution" element={<Distribution />} />
                <Route path="/requests" element={<Requests />} />
                <Route path="/requests/new" element={<RegisterRequest />} />
              </Route>
              <Route element={<RequireCapability need="admin" />}>
                <Route path="/admin/users" element={<Users />} />
              </Route>
              <Route element={<RequireCapability need="persons" />}>
                <Route path="/persons" element={<PersonLookup />} />
                <Route path="/persons/:personId" element={<PersonProfilePage />} />
              </Route>
              <Route element={<RequireCapability need="intelligence" />}>
                <Route path="/clusters" element={<Clusters />} />
                <Route
                  path="/intelligence/map"
                  element={
                    <Suspense fallback={null}>
                      <OccurrenceMap />
                    </Suspense>
                  }
                />
                <Route
                  path="/intelligence/occurrences/new"
                  element={
                    <Suspense fallback={null}>
                      <NewOccurrence />
                    </Suspense>
                  }
                />
              </Route>
            </Route>
          </Route>

          {/* Auth Layout */}
          <Route path="/signin" element={<SignIn />} />

          {/* Fallback Route */}
          <Route path="*" element={<NotFound />} />
        </Routes>
      </Router>
    </>
  );
}
