import { SidebarProvider, useSidebar } from "../context/SidebarContext";
import { PageMemoryProvider } from "../context/PageMemoryContext";
import { LayoutWidthProvider, useLayoutWide } from "../context/LayoutWidthContext";
import { Outlet } from "react-router";
import AppHeader from "./AppHeader";
import Backdrop from "./Backdrop";
import AppSidebar from "./AppSidebar";

const LayoutContent: React.FC = () => {
  const { isExpanded, isHovered, isMobileOpen } = useSidebar();
  // A page may ask for more room on large screens (useWideLayout).
  const wide = useLayoutWide();

  return (
    <div className="min-h-screen xl:flex">
      <div>
        <AppSidebar />
        <Backdrop />
      </div>
      <div
        className={`flex-1 transition-all duration-300 ease-in-out ${
          isExpanded || isHovered ? "lg:ml-[290px]" : "lg:ml-[90px]"
        } ${isMobileOpen ? "ml-0" : ""}`}
      >
        <AppHeader />
        <div className={`p-4 mx-auto md:p-6 ${wide ? "max-w-(--breakpoint-3xl)" : "max-w-(--breakpoint-2xl)"}`}>
          <Outlet />
        </div>
      </div>
    </div>
  );
};

const AppLayout: React.FC = () => {
  return (
    <SidebarProvider>
      <PageMemoryProvider>
        <LayoutWidthProvider>
          <LayoutContent />
        </LayoutWidthProvider>
      </PageMemoryProvider>
    </SidebarProvider>
  );
};

export default AppLayout;
