import type { Accept } from "react-dropzone";

// What each kind of upload accepts, for FileDropzone. Extensions are listed for the formats a
// browser often reports without a type, so they are accepted when dropped as well as when picked.
const imageExtensions = [".tif", ".tiff", ".bmp"];
const videoExtensions = [".mp4", ".m4v", ".mov", ".avi", ".mkv", ".webm"];

export const IMAGE_FILES: Accept = { "image/*": imageExtensions };
export const AVATAR_FILES: Accept = { "image/*": [] };
export const VIDEO_FILES: Accept = { "video/*": videoExtensions };
export const SESSION_FILES: Accept = { "application/json": [".json"] };
/** Evidence attached to a case: the formats the server can show and detect faces in. */
export const EVIDENCE_FILES: Accept = {
  "application/pdf": [],
  "image/jpeg": [],
  "image/png": [],
  "image/webp": [],
  "image/gif": [],
  "image/tiff": [],
  "image/bmp": [],
};
/** Material sent in with a request. */
export const MATERIAL_FILES: Accept = { "application/pdf": [], "image/*": imageExtensions, "video/*": videoExtensions };
/** Documents the assist lab reads text from. */
export const DOCUMENT_FILES: Accept = { "application/pdf": [], "text/*": [], "image/*": imageExtensions };
