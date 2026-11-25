import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useLocation } from "react-router";
import { useTranslation } from "react-i18next";

import {
  BoxCubeIcon,
  ChatIcon,
  ChevronDownIcon,
  DocsIcon,
  FolderIcon,
  GridIcon,
  GroupIcon,
  HorizontaLDots,
  LockIcon,
  ListIcon,
  PieChartIcon,
  TableIcon,
  ShootingStarIcon,
  TaskIcon,
} from "../icons";
import { useSidebar } from "../context/SidebarContext";
import { useCurrentUser } from "../context/CurrentUserContext";
import Brand from "../components/common/Brand";
import { extension } from "../extensions";
import type { NavSectionKey } from "../extensions/types";
import type { Need } from "../context/CurrentUserContext";

type NavItem = {
  name: string;
  icon: React.ReactNode;
  path?: string;
  /** Shown only to users with this capability or grant (useCurrentUser's can); "persons" is examiners and analysts. */
  requires?: Need;
  subItems?: { name: string; path: string; new?: boolean; soon?: boolean }[];
};

type NavSection = {
  /** The heading; the first section, Home alone, has none. */
  title?: string;
  items: NavItem[];
};

// An extension's items for a section, at its end; those naming none go to management.
const extensionItems = (section: NavSectionKey): NavItem[] =>
  extension.navItems.filter((item) => (item.section ?? "management") === section);

// Grouped by what the user is doing: working a case, investigating across cases, or running the
// unit. The profile is in the header's user menu, not here.
const allNavSections: NavSection[] = [
  {
    items: [{ icon: <GridIcon />, name: "nav.home", path: "/" }],
  },
  {
    title: "nav.sectionCasework",
    items: [
      { icon: <FolderIcon />, name: "nav.criminalCases", path: "/cases", requires: "profile" },
      { icon: <TaskIcon />, name: "nav.sceneCollection", path: "/collections/new", requires: "scene" },
      { icon: <TaskIcon />, name: "nav.fieldNotes", path: "/field-notes", requires: "scene" },
      { icon: <ListIcon />, name: "nav.myAnalyses", path: "/analyses", requires: "profile" },
      {
        icon: <ShootingStarIcon />,
        name: "nav.labs",
        requires: "profile",
        subItems: [
          { name: "nav.imageLab", path: "/image-lab" },
          { name: "nav.faceLab", path: "/face-lab" },
          { name: "nav.videoLab", path: "/video-lab", new: true },
          { name: "nav.assistLab", path: "/assist-lab" },
        ],
      },
      {
        icon: <TaskIcon />,
        name: "nav.examinations",
        requires: "profile",
        subItems: [
          { name: "nav.labExamination", path: "/examinations/laboratory", soon: true },
        ],
      },
      ...extensionItems("casework"),
    ],
  },
  {
    title: "nav.sectionInvestigation",
    items: [
      { icon: <ChatIcon />, name: "nav.ask", path: "/intelligence/ask", requires: "intelligence" },
      { icon: <ShootingStarIcon />, name: "nav.operations", path: "/intelligence/operations", requires: "intelligence" },
      { icon: <GroupIcon />, name: "nav.persons", path: "/persons", requires: "persons" },
      { icon: <BoxCubeIcon />, name: "nav.clusters", path: "/clusters", requires: "intelligence" },
      {
        icon: <PieChartIcon />,
        name: "nav.occurrences",
        requires: "intelligence",
        subItems: [
          { name: "nav.occurrenceMap", path: "/intelligence/map" },
          { name: "nav.newOccurrence", path: "/intelligence/occurrences/new" },
        ],
      },
      ...extensionItems("investigation"),
    ],
  },
  {
    title: "nav.sectionManagement",
    items: [
      { icon: <DocsIcon />, name: "nav.requests", path: "/requests", requires: "coordinator" },
      { icon: <TableIcon />, name: "nav.distribution", path: "/admin/distribution", requires: "coordinator" },
      { icon: <LockIcon />, name: "nav.users", path: "/admin/users", requires: "admin" },
      { icon: <GroupIcon />, name: "nav.organizations", path: "/admin/organizations", requires: "admin" },
      ...extensionItems("management"),
    ],
  },
];

const AppSidebar: React.FC = () => {
  const { t } = useTranslation();
  const { isExpanded, isMobileOpen, isHovered, setIsHovered } = useSidebar();
  const { can } = useCurrentUser();
  // A section whose every item the user cannot reach disappears, heading included.
  const navSections = useMemo(
    () =>
      allNavSections
        .map((section) => ({ ...section, items: section.items.filter((item) => !item.requires || can(item.requires)) }))
        .filter((section) => section.items.length > 0),
    [can],
  );
  const location = useLocation();

  const [openSubmenu, setOpenSubmenu] = useState<string | null>(null);
  const [subMenuHeight, setSubMenuHeight] = useState<Record<string, number>>({});
  const subMenuRefs = useRef<Record<string, HTMLDivElement | null>>({});

  // A page inside a section lights its entry too: /cases/42 is under Biometric Cases. Only the most specific entry
  // lights, so /intelligence/operations/mitra lights its own menu and not Operations (a prefix of it) as well.
  const isActive = useCallback(
    (path: string) => {
      const under = (p: string) => (p === "/" ? location.pathname === "/" : location.pathname === p || location.pathname.startsWith(`${p}/`));
      if (!under(path)) return false;
      return !navSections.some((section) =>
        section.items.some((nav) => [nav.path, ...(nav.subItems ?? []).map((s) => s.path)].some((other) => !!other && other.length > path.length && under(other))),
      );
    },
    [location.pathname, navSections],
  );

  useEffect(() => {
    const parent = navSections
      .flatMap((section) => section.items)
      .find((nav) => nav.subItems?.some((subItem) => isActive(subItem.path)));
    setOpenSubmenu(parent?.name ?? null);
  }, [location, isActive, navSections]);

  useEffect(() => {
    if (openSubmenu !== null && subMenuRefs.current[openSubmenu]) {
      setSubMenuHeight((prevHeights) => ({
        ...prevHeights,
        [openSubmenu]: subMenuRefs.current[openSubmenu]?.scrollHeight || 0,
      }));
    }
  }, [openSubmenu]);

  const handleSubmenuToggle = (name: string) => {
    setOpenSubmenu((previous) => (previous === name ? null : name));
  };

  const showLabels = isExpanded || isHovered || isMobileOpen;

  const renderMenuItems = (items: NavItem[]) => (
    <ul className="flex flex-col gap-1">
      {items.map((nav) => (
        <li key={nav.name}>
          {nav.subItems ? (
            <button
              onClick={() => handleSubmenuToggle(nav.name)}
              aria-expanded={openSubmenu === nav.name}
              className={`menu-item group ${
                openSubmenu === nav.name ? "menu-item-active" : "menu-item-inactive"
              } cursor-pointer ${!isExpanded && !isHovered ? "lg:justify-center" : "lg:justify-start"}`}
            >
              <span
                className={`menu-item-icon-size  ${
                  openSubmenu === nav.name ? "menu-item-icon-active" : "menu-item-icon-inactive"
                }`}
              >
                {nav.icon}
              </span>
              {showLabels && <span className="menu-item-text">{t(nav.name)}</span>}
              {showLabels && (
                <ChevronDownIcon
                  className={`ml-auto w-5 h-5 transition-transform duration-200 ${
                    openSubmenu === nav.name ? "rotate-180 text-brand-500" : ""
                  }`}
                />
              )}
            </button>
          ) : (
            nav.path && (
              <Link
                to={nav.path}
                className={`menu-item group ${
                  isActive(nav.path) ? "menu-item-active" : "menu-item-inactive"
                } ${!isExpanded && !isHovered ? "lg:justify-center" : "lg:justify-start"}`}
              >
                <span
                  className={`menu-item-icon-size ${
                    isActive(nav.path) ? "menu-item-icon-active" : "menu-item-icon-inactive"
                  }`}
                >
                  {nav.icon}
                </span>
                {showLabels && <span className="menu-item-text">{t(nav.name)}</span>}
              </Link>
            )
          )}
          {nav.subItems && showLabels && (
            <div
              ref={(el) => {
                subMenuRefs.current[nav.name] = el;
              }}
              className="overflow-hidden transition-all duration-300"
              style={{ height: openSubmenu === nav.name ? `${subMenuHeight[nav.name]}px` : "0px" }}
            >
              <ul className="mt-2 space-y-1 ml-9">
                {nav.subItems.map((subItem) => {
                  const badgeClass = `ml-auto ${
                    isActive(subItem.path) ? "menu-dropdown-badge-active" : "menu-dropdown-badge-inactive"
                  } menu-dropdown-badge`;
                  return (
                    <li key={subItem.name}>
                      <Link
                        to={subItem.path}
                        className={`menu-dropdown-item ${
                          isActive(subItem.path) ? "menu-dropdown-item-active" : "menu-dropdown-item-inactive"
                        }`}
                      >
                        {t(subItem.name)}
                        <span className="flex items-center gap-1 ml-auto">
                          {subItem.new && <span className={badgeClass}>{t("nav.new")}</span>}
                          {subItem.soon && <span className={badgeClass}>{t("comingSoon.label")}</span>}
                        </span>
                      </Link>
                    </li>
                  );
                })}
              </ul>
            </div>
          )}
        </li>
      ))}
    </ul>
  );

  return (
    <aside
      className={`fixed mt-16 flex flex-col lg:mt-0 top-0 px-5 left-0 bg-white dark:bg-gray-900 dark:border-gray-800 text-gray-900 h-screen transition-all duration-300 ease-in-out z-50 border-r border-gray-200 
        ${
          isExpanded || isMobileOpen
            ? "w-[290px]"
            : isHovered
            ? "w-[290px]"
            : "w-[90px]"
        }
        ${isMobileOpen ? "translate-x-0" : "-translate-x-full"}
        lg:translate-x-0`}
      onMouseEnter={() => !isExpanded && setIsHovered(true)}
      onMouseLeave={() => setIsHovered(false)}
    >
      <div
        className={`py-8 flex ${
          !isExpanded && !isHovered ? "lg:justify-center" : "justify-start"
        }`}
      >
        <Link to="/">
          <Brand iconOnly={!(isExpanded || isHovered || isMobileOpen)} />
        </Link>
      </div>
      <div className="flex flex-col overflow-y-auto duration-300 ease-linear no-scrollbar">
        <nav className="mb-6">
          <div className="flex flex-col gap-5">
            {navSections.map((section) => (
              <div key={section.title ?? "home"}>
                {section.title && (
                  <h2
                    className={`mb-2 text-xs uppercase flex leading-[20px] text-gray-400 ${
                      !isExpanded && !isHovered ? "lg:justify-center" : "justify-start"
                    }`}
                  >
                    {showLabels ? t(section.title) : <HorizontaLDots className="size-6" />}
                  </h2>
                )}
                {renderMenuItems(section.items)}
              </div>
            ))}
          </div>
        </nav>
      </div>
    </aside>
  );
};

export default AppSidebar;
