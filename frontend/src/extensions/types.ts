import type { ComponentType, ReactNode } from "react";

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

/**
 * A number a request arrives under in the organization's own systems (an ePol task, a SEI process),
 * asked for as a field of its own on the request form and kept as a reference with this label.
 * identifies: the request may go by it alone, when its document has no number of its own.
 */
export interface RequestReferenceField {
  label: string;
  /** Translation key of the field's name; the label itself when absent. */
  name?: string;
  placeholder?: string;
  identifies?: boolean;
}

export interface Extension {
  routes: ExtensionRoute[];
  navItems: ExtensionNavItem[];
  /** Merged over core's translations, by language. */
  translations: Record<string, object>;
  /** A panel on an intelligence operation's page, by the operation's slug. */
  operationPanels?: Record<string, ReactNode>;
  /**
   * Slugs of operations that never close, such as a standing line of work: their page offers no "close". Hiding the
   * button only keeps the page from offering it; a deployment that must forbid closing guards it in the database.
   */
  permanentOperations?: string[];
  /**
   * Panels on a case's, a person's and a cluster's page, in this order, for intelligence analysts only.
   * Hiding them only keeps the page from offering what the server would refuse: a panel's routes must be
   * guarded themselves (api.RequireIntelligence), and it must show nothing when refused, as it will be for
   * an analyst working a step blind, whom the page can't tell apart. A panel may come from React's lazy.
   */
  casePanels?: ComponentType<CasePanelProps>[];
  personPanels?: ComponentType<PersonPanelProps>[];
  clusterPanels?: ComponentType<ClusterPanelProps>[];
  /** Fields of the request form for the organization's own reference numbers, first in the form. */
  requestReferences?: RequestReferenceField[];
}

export interface CasePanelProps {
  caseId: string;
  modality: string;
}

export interface PersonPanelProps {
  personId: string;
}

export interface ClusterPanelProps {
  clusterId: number;
  modality: string;
}
