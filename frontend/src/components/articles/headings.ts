import type { ArticleDoc, ArticleNode } from "../../services/articles";

/** The headings of a document, for its table of contents. */
export function headingsOf(doc: ArticleDoc | undefined): { level: number; text: string }[] {
  const text = (node: ArticleNode): string =>
    node.type === "text"
      ? node.text ?? ""
      : node.type === "mention"
        ? `@${String(node.attrs?.label ?? node.attrs?.ref ?? "")}`
        : (node.content ?? []).map(text).join("");
  return (doc?.content ?? [])
    .filter((node) => node.type === "heading")
    .map((node) => ({ level: Number(node.attrs?.level ?? 2), text: text(node) }))
    .filter((h) => h.text.trim() !== "");
}
