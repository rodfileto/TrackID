import type { ReactNode } from "react";

/** A page an organization adds, shown to those with the capability when one is named. */
export interface ExtensionRoute {
  path: string;
  element: ReactNode;
  need?: "admin" | "coordinator" | "intelligence" | "persons";
}

/** A menu item an organization adds to the management section. */
export interface ExtensionNavItem {
  name: string;
  path: string;
  icon: ReactNode;
  requires?: "admin" | "coordinator" | "intelligence" | "persons";
}

export interface Extension {
  routes: ExtensionRoute[];
  navItems: ExtensionNavItem[];
  /** Merged over core's translations, by language. */
  translations: Record<string, object>;
  /** A panel on an intelligence operation's page, by the operation's slug. */
  operationPanels?: Record<string, ReactNode>;
}
