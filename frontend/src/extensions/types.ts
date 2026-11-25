import type { ComponentType, ReactNode } from "react";
import type { UserProfile } from "../services/auth";

/**
 * Who a page or menu item is for: those with one of core's capabilities, or those the extension grants a name to
 * (Extension.grants), whatever their capabilities.
 */
export type ExtensionNeed = "admin" | "coordinator" | "intelligence" | "persons" | { grant: string };

/** A page an organization adds, shown to those with the capability or grant when one is named. */
export interface ExtensionRoute {
  path: string;
  element: ReactNode;
  need?: ExtensionNeed;
}

/** A menu item an organization adds to the sidebar, at the end of a section: management unless it names another. */
export interface ExtensionNavItem {
  name: string;
  /** Optional when the item only opens a submenu. */
  path?: string;
  icon: ReactNode;
  requires?: ExtensionNeed;
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
  /**
   * Rendered once on every signed-in page, outside the page itself, so it lives on while the user moves between
   * pages: something that must reach the user wherever they are, such as an alert. It draws nothing of its own
   * unless it means to (a floating banner, say), and must show nothing to someone it doesn't concern. One may come
   * from React's lazy; the layout wraps each in Suspense.
   */
  globalComponents?: ComponentType[];
  /**
   * Buttons in the header, before the notifications' bell, on every signed-in page: an indicator the user should
   * see wherever they are. Each renders nothing for someone it doesn't concern. One may come from React's lazy; the
   * header wraps each in Suspense.
   */
  headerComponents?: ComponentType[];
  /**
   * The names the extension grants the signed-in user, beyond core's capabilities: a role in the organization's own
   * line of work, say. Read once after sign-in and kept with the current user; `{ grant }` in a route's `need` or an
   * item's `requires` names one. As with capabilities, it only keeps the screens from offering what the server would
   * refuse: the routes behind it must check the same themselves. A failure is taken as no grants.
   */
  grants?: () => Promise<string[]>;
  /**
   * The page `/` opens for this user, given their grants, instead of core's Home; core's Home when it gives none.
   */
  home?: (grants: string[], user: UserProfile) => string | undefined;
  /** Profiles an admin can preview the home page and menu as, after core's own: the extension's roles, say. */
  previewProfiles?: PreviewProfile[];
}

/**
 * A profile an admin previews `/` and the menu as (the header's "view as"): its capabilities, false when left out,
 * and the extension's grants. Only the screens change; every request still goes as the admin.
 */
export interface PreviewProfile {
  /** Translation key of its name. */
  name: string;
  access?: Partial<Pick<UserProfile, "examines_fingerprint" | "examines_facial" | "intelligence_analyst" | "coordinator">>;
  grants?: string[];
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
