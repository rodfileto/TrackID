import type { ComponentType, ReactNode } from "react";

/** A page an organization adds, shown to those with the capability when one is named. */
export interface ExtensionRoute {
  path: string;
  element: ReactNode;
  need?: "admin" | "coordinator" | "intelligence" | "persons";
}

/** A menu item an organization adds to the sidebar, at the end of a section: management unless it names another. */
export interface ExtensionNavItem {
  name: string;
  /** Optional when the item only opens a submenu. */
  path?: string;
  icon: ReactNode;
  requires?: "admin" | "coordinator" | "intelligence" | "persons";
  subItems?: { name: string; path: string; new?: boolean; soon?: boolean }[];
  section?: NavSectionKey;
}

/** The sidebar's sections an extension can add to. */
export type NavSectionKey = "casework" | "investigation" | "management";

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
   * Slugs of operations whose page shows only their panel, without the card with the operation's name, status,
   * description and edit button: for a deployment whose panel is the operation's whole page.
   */
  panelOnlyOperations?: string[];
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
  /**
   * The organization's own fields on an account's edit dialog (Users), after core's, for admins: an id
   * another system knows the account by, say. Each keeps its value and stores it through its own routes.
   */
  userFields?: ComponentType<UserFieldsProps>[];
}

export interface UserFieldsProps {
  userId: number;
  /**
   * Hands the dialog what saves the field, called again whenever its value changes: the dialog calls the
   * last one given after saving the account, and stays open, showing the error, if it throws.
   */
  onSave: (save: () => Promise<void>) => void;
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
