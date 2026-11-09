import { useSyncExternalStore } from "react";
import { Link, useLocation, useNavigate } from "react-router";
import { useTranslation } from "react-i18next";
import { cameFrom, subscribeToTrail } from "../../utils/navigationTrail";

interface BackLinkProps {
  /** Where to go when this tab has nothing to go back to: a link opened in a new tab, or a reload
   * of one. It is also the link's address, so opening it in a new tab works. */
  fallback: string;
  /** The link's text when it leads to the fallback, as in "Back to clusters". */
  label: string;
  className?: string;
}

/** Goes back like the browser's button, to the page the user actually came from, with its filters,
 * page and scroll position; a plain link to the list would lose them. It says only "Back" when
 * that page is not the one the label names. */
export default function BackLink({ fallback, label, className }: BackLinkProps) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const location = useLocation();
  const from = useSyncExternalStore(subscribeToTrail, () => cameFrom(location.key));
  const fallbackPathname = fallback.split(/[?#]/)[0];
  const namesFrom = !from || from.split(/[?#]/)[0] === fallbackPathname;

  return (
    <Link
      to={fallback}
      className={className ?? "inline-flex items-center gap-1.5 text-sm text-gray-500 transition-colors hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"}
      onClick={(event) => {
        // Ctrl/cmd/shift/middle click keep their usual meaning: open the fallback elsewhere.
        if (!from || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
        event.preventDefault();
        navigate(-1);
      }}
    >
      <svg className="stroke-current" width="17" height="16" viewBox="0 0 17 16" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
        <path d="M10.9235 12.667L6.75683 8.50033L10.9235 4.33366" strokeWidth="1.2" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
      {namesFrom ? label : t("common.back")}
    </Link>
  );
}
