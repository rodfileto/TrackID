import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { getFollow, setFollow, type FollowState } from "../../services/follows";

interface FollowButtonProps {
  kind: "PERSON" | "CLUSTER";
  /** A person's person_id, or a cluster's id. */
  id: string | number;
}

/** Follow a person or a cluster, to be told in the bell (and in Teams, with a webhook) when it changes. Only
 * AUTOMATIC clusters are followed, so on a REVIEWED cluster's page it says which AUTOMATIC cluster it follows. */
export default function FollowButton({ kind, id }: FollowButtonProps) {
  const { t } = useTranslation();
  const [state, setState] = useState<FollowState | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setState(null);
    getFollow(kind, id)
      .then((s) => {
        if (!cancelled) setState(s);
      })
      .catch(() => {
        // Nothing to follow, or no right to: the button stays hidden.
      });
    return () => {
      cancelled = true;
    };
  }, [kind, id]);

  if (!state) return null;

  const toggle = async () => {
    setBusy(true);
    try {
      const next = await setFollow(kind, id, !state.following);
      setState(next);
      toast.success(next.following ? t("follow.started") : t("follow.stopped"));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  const other = kind === "CLUSTER" && state.subject.id !== String(id);
  return (
    <div className="flex flex-wrap items-center gap-2">
      {other && (
        <span className="text-xs text-gray-500 dark:text-gray-400">
          {t("follow.automaticCluster", { id: state.subject.id })}
        </span>
      )}
      <button
        type="button"
        onClick={toggle}
        disabled={busy}
        aria-pressed={state.following}
        title={t("follow.hint")}
        className={`inline-flex h-9 items-center gap-1.5 rounded-lg px-3 text-sm font-medium disabled:opacity-50 ${
          state.following
            ? "border border-gray-300 bg-white text-gray-700 hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-white/[0.03]"
            : "bg-brand-500 text-white hover:bg-brand-600"
        }`}
      >
        <svg width="16" height="16" viewBox="0 0 20 20" fill="none" aria-hidden="true">
          <path
            d="M10 2.5a5 5 0 0 0-5 5v3.6L3.6 13.5h12.8L15 11.1V7.5a5 5 0 0 0-5-5Zm-1.7 13.3a1.8 1.8 0 0 0 3.4 0"
            stroke="currentColor"
            strokeWidth="1.5"
            strokeLinecap="round"
            strokeLinejoin="round"
            fill={state.following ? "currentColor" : "none"}
          />
        </svg>
        {state.following ? t("follow.following") : t("follow.follow")}
      </button>
    </div>
  );
}
