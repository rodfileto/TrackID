import type { Extension } from "./types";

/** Core adds nothing. An organization replaces this file in its copy of the frontend, importing from
 * core's own folders, and so keeps receiving the rest of the frontend unchanged. */
export const extension: Extension = { routes: [], navItems: [], translations: {} };
