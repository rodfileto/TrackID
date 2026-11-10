import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Dropdown } from "../ui/dropdown/Dropdown";
import { DropdownItem } from "../ui/dropdown/DropdownItem";
import {
  listNotifications,
  markAllNotificationsRead,
  markNotificationRead,
  type AppNotification,
  type NotificationPage,
} from "../../services/notifications";
import { subscribeRealtime } from "../../services/realtime";
import { useSearchParams } from "react-router";

const refreshingEvents = new Set(["connected", "resync", "notifications.changed"]);

const itemClass =
  "flex w-full gap-3 rounded-lg border-b border-gray-100 p-3 px-4.5 py-3 text-left hover:bg-gray-100 dark:border-gray-800 dark:hover:bg-white/5";

export default function NotificationDropdown() {
  const { t, i18n } = useTranslation();
  const [isOpen, setIsOpen] = useState(false);
  const [page, setPage] = useState<NotificationPage>({ items: [], unread: 0 });
  const latestRequest = useRef(0);
  const [searchParams, setSearchParams] = useSearchParams();

  // A Teams message's button opens the app with the bell open.
  useEffect(() => {
    if (searchParams.get("notifications") !== "1") return;
    setIsOpen(true);
    const next = new URLSearchParams(searchParams);
    next.delete("notifications");
    setSearchParams(next, { replace: true });
  }, [searchParams, setSearchParams]);

  const load = useCallback(async () => {
    latestRequest.current += 1;
    const request = latestRequest.current;
    try {
      const next = await listNotifications();
      if (request === latestRequest.current) setPage(next);
    } catch {
      // The bell keeps what it shows; the next signal or opening fetches again.
    }
  }, []);

  useEffect(() => {
    void load();
    return subscribeRealtime((event) => {
      if (refreshingEvents.has(event.type)) void load();
    });
  }, [load]);

  const closeDropdown = useCallback(() => setIsOpen(false), []);

  const toggleDropdown = () => {
    if (!isOpen) void load();
    setIsOpen(!isOpen);
  };

  const markRead = (item: AppNotification) => {
    if (item.read) return;
    setPage((current) => ({
      items: current.items.map((other) => (other.id === item.id ? { ...other, read: true } : other)),
      unread: Math.max(0, current.unread - 1),
    }));
    markNotificationRead(item.id).catch(() => void load());
  };

  const markAllRead = () => {
    setPage((current) => ({
      items: current.items.map((item) => ({ ...item, read: true })),
      unread: 0,
    }));
    markAllNotificationsRead().catch(() => void load());
  };

  const subjectTitle = (changes: NonNullable<AppNotification["changes"]>): string =>
    changes.subjectKind === "PERSON"
      ? t("notifications.person", { name: changes.name || changes.subjectId })
      : t("notifications.cluster", { id: changes.subjectId });

  const changeLine = (c: { change: string; count: number; refs?: string[] }): string => {
    const refs = (c.refs ?? []).join(", ");
    return t(`notifications.change.${c.change}`, {
      count: c.count,
      refs,
      defaultValue: t("notifications.change.other", { count: c.count, change: c.change }),
    });
  };

  const target = (item: AppNotification): string | undefined => {
    if (item.changes) {
      const c = item.changes;
      if (c.subjectKind === "PERSON") return c.personNumber ? `/persons/${c.personNumber}` : undefined;
      return `/clusters/${c.mergedInto ?? c.subjectId}`;
    }
    if (item.analysisId !== undefined) {
      return `/cases/${encodeURIComponent(item.caseId ?? "")}/analyses/${item.analysisId}`;
    }
    return undefined;
  };

  const message = (item: AppNotification): string => {
    if (item.kind === "PERSON_CHANGED" || item.kind === "CLUSTER_CHANGED") {
      return item.changes ? subjectTitle(item.changes) : t("notifications.followUnavailable");
    }
    if (item.analysisId === undefined) return t("notifications.assignmentMoved");
    if (item.blind) return t("notifications.assignedBlind");
    return t("notifications.assigned", {
      type: t(`analyses.type.${item.analysisType}`),
      caseId: item.caseId,
    });
  };

  const content = (item: AppNotification) => (
    <>
      <span
        className={`mt-1.5 block h-2 w-2 shrink-0 rounded-full ${item.read ? "bg-transparent" : "bg-orange-400"}`}
      />
      <span className="block">
        <span
          className={`mb-1.5 block text-theme-sm ${
            item.read ? "text-gray-500 dark:text-gray-400" : "font-medium text-gray-800 dark:text-white/90"
          }`}
        >
          {message(item)}
        </span>
        {item.changes && item.changes.changes.length > 0 && (
          <span className="mb-1.5 block text-theme-xs text-gray-600 dark:text-gray-300">
            {item.changes.changes.map((c) => (
              <span key={c.change} className="block">
                {changeLine(c)}
              </span>
            ))}
          </span>
        )}
        <span className="flex items-center gap-2 text-gray-500 text-theme-xs dark:text-gray-400">
          {item.step && (
            <>
              <span>{t(`analyses.step.${item.step}`)}</span>
              <span className="w-1 h-1 bg-gray-400 rounded-full"></span>
            </>
          )}
          <span>{new Date(item.updatedAt ?? item.createdAt).toLocaleString(i18n.language)}</span>
        </span>
      </span>
    </>
  );

  return (
    <div className="relative">
      <button
        className="relative flex items-center justify-center text-gray-500 transition-colors bg-white border border-gray-200 rounded-full dropdown-toggle hover:text-gray-700 h-11 w-11 hover:bg-gray-100 dark:border-gray-800 dark:bg-gray-900 dark:text-gray-400 dark:hover:bg-gray-800 dark:hover:text-white"
        onClick={toggleDropdown}
        aria-label={t("notifications.open")}
      >
        <span
          className={`absolute right-0 top-0.5 z-10 h-2 w-2 rounded-full bg-orange-400 ${
            page.unread === 0 ? "hidden" : "flex"
          }`}
        >
          <span className="absolute inline-flex w-full h-full bg-orange-400 rounded-full opacity-75 animate-ping"></span>
        </span>
        <svg className="fill-current" width="20" height="20" viewBox="0 0 20 20" xmlns="http://www.w3.org/2000/svg">
          <path
            fillRule="evenodd"
            clipRule="evenodd"
            d="M10.75 2.29248C10.75 1.87827 10.4143 1.54248 10 1.54248C9.58583 1.54248 9.25004 1.87827 9.25004 2.29248V2.83613C6.08266 3.20733 3.62504 5.9004 3.62504 9.16748V14.4591H3.33337C2.91916 14.4591 2.58337 14.7949 2.58337 15.2091C2.58337 15.6234 2.91916 15.9591 3.33337 15.9591H4.37504H15.625H16.6667C17.0809 15.9591 17.4167 15.6234 17.4167 15.2091C17.4167 14.7949 17.0809 14.4591 16.6667 14.4591H16.375V9.16748C16.375 5.9004 13.9174 3.20733 10.75 2.83613V2.29248ZM14.875 14.4591V9.16748C14.875 6.47509 12.6924 4.29248 10 4.29248C7.30765 4.29248 5.12504 6.47509 5.12504 9.16748V14.4591H14.875ZM8.00004 17.7085C8.00004 18.1228 8.33583 18.4585 8.75004 18.4585H11.25C11.6643 18.4585 12 18.1228 12 17.7085C12 17.2943 11.6643 16.9585 11.25 16.9585H8.75004C8.33583 16.9585 8.00004 17.2943 8.00004 17.7085Z"
            fill="currentColor"
          />
        </svg>
      </button>
      <Dropdown
        isOpen={isOpen}
        onClose={closeDropdown}
        className="mt-[17px] flex h-[480px] w-[361px] flex-col rounded-2xl border border-gray-200 bg-white p-3 shadow-theme-lg dark:border-gray-800 dark:bg-gray-dark max-lg:!fixed max-lg:!left-4 max-lg:!right-4 max-lg:mx-auto max-lg:max-h-[calc(100dvh-11rem)] max-lg:!w-auto max-lg:max-w-[361px]"
      >
        <div className="flex items-center justify-between pb-3 mb-3 border-b border-gray-100 dark:border-gray-700">
          <h5 className="text-lg font-semibold text-gray-800 dark:text-gray-200">{t("notifications.title")}</h5>
          <div className="flex items-center gap-3">
            {page.unread > 0 && (
              <button onClick={markAllRead} className="text-theme-xs text-brand-500 hover:underline">
                {t("notifications.markAllRead")}
              </button>
            )}
            <button
              onClick={closeDropdown}
              aria-label={t("common.close")}
              className="text-gray-500 transition dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200"
            >
              <svg className="fill-current" width="24" height="24" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                <path
                  fillRule="evenodd"
                  clipRule="evenodd"
                  d="M6.21967 7.28131C5.92678 6.98841 5.92678 6.51354 6.21967 6.22065C6.51256 5.92775 6.98744 5.92775 7.28033 6.22065L11.999 10.9393L16.7176 6.22078C17.0105 5.92789 17.4854 5.92788 17.7782 6.22078C18.0711 6.51367 18.0711 6.98855 17.7782 7.28144L13.0597 12L17.7782 16.7186C18.0711 17.0115 18.0711 17.4863 17.7782 17.7792C17.4854 18.0721 17.0105 18.0721 16.7176 17.7792L11.999 13.0607L7.28033 17.7794C6.98744 18.0722 6.51256 18.0722 6.21967 17.7794C5.92678 17.4865 5.92678 17.0116 6.21967 16.7187L10.9384 12L6.21967 7.28131Z"
                  fill="currentColor"
                />
              </svg>
            </button>
          </div>
        </div>
        {page.items.length === 0 ? (
          <p className="p-4 text-center text-theme-sm text-gray-500 dark:text-gray-400">{t("notifications.empty")}</p>
        ) : (
          <ul className="flex flex-col h-auto overflow-y-auto custom-scrollbar">
            {page.items.map((item) => (
              <li key={item.id}>
                {target(item) !== undefined ? (
                  <DropdownItem
                    tag="a"
                    to={target(item)}
                    onClick={() => markRead(item)}
                    onItemClick={closeDropdown}
                    baseClassName=""
                    className={itemClass}
                  >
                    {content(item)}
                  </DropdownItem>
                ) : (
                  <DropdownItem onClick={() => markRead(item)} baseClassName="" className={itemClass}>
                    {content(item)}
                  </DropdownItem>
                )}
              </li>
            ))}
          </ul>
        )}
      </Dropdown>
    </div>
  );
}
