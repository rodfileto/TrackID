import { useId, useRef, useState, type KeyboardEvent, type ReactNode } from "react";

// Tabs in TailAdmin's two formats (its MIT tabs component): "pill", a gray track with the active tab raised in white,
// and "line", a 2px brand underline; each horizontal or vertical. A tab may carry an icon and a count.

export interface TabItem<K extends string = string> {
  key: K;
  label: ReactNode;
  icon?: ReactNode;
  /** A number beside the label, e.g. how many items the tab holds. */
  count?: number;
  disabled?: boolean;
  /** The panel shown while the tab is active; leave it out to render panels yourself. */
  content?: ReactNode;
}

type Variant = "pill" | "line";
type Orientation = "horizontal" | "vertical";

const listClasses: Record<Variant, Record<Orientation, string>> = {
  pill: {
    horizontal: "inline-flex h-11 w-fit items-center rounded-lg bg-gray-100 p-1 dark:bg-gray-900",
    vertical: "inline-flex h-fit w-fit flex-col items-stretch rounded-lg bg-gray-100 p-1 dark:bg-gray-900",
  },
  line: {
    horizontal: "flex h-12 w-full items-center justify-start gap-2 border-b border-gray-200 dark:border-gray-800",
    vertical: "flex h-fit flex-col items-stretch gap-1 border-e border-gray-200 dark:border-gray-800",
  },
};

function tabClasses(variant: Variant, orientation: Orientation, active: boolean): string {
  const base =
    "relative inline-flex cursor-pointer items-center gap-1.5 font-outfit text-sm font-medium whitespace-nowrap outline-hidden transition-colors duration-200 select-none focus-visible:ring-3 focus-visible:ring-brand-500/20 disabled:cursor-not-allowed disabled:opacity-50 [&_svg]:size-4 [&_svg]:shrink-0";
  const vertical = orientation === "vertical" ? "w-full justify-start" : "justify-center";
  if (variant === "pill") {
    return `${base} ${vertical} h-9 rounded-md px-3 py-2 ${
      active
        ? "bg-white text-gray-900 shadow-theme-xs dark:bg-white/[0.03] dark:text-white"
        : "text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white"
    }`;
  }
  const edge =
    orientation === "vertical"
      ? "-me-px border-e-2 px-3 py-2.5"
      : "-mb-px h-12 border-b-2 px-3";
  return `${base} ${vertical} ${edge} rounded-none ${
    active
      ? "border-brand-500 text-brand-500 dark:border-brand-400 dark:text-brand-400"
      : "border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
  }`;
}

function countClasses(active: boolean): string {
  return `inline-flex min-w-5 items-center justify-center rounded-full px-1.5 text-theme-xs font-medium ${
    active
      ? "bg-brand-50 text-brand-500 dark:bg-brand-500/15 dark:text-brand-400"
      : "bg-gray-200 text-gray-600 dark:bg-white/[0.05] dark:text-gray-400"
  }`;
}

/** Tabs, controlled (value with onChange) or not (defaultValue). Arrow keys move between tabs, Home and End to the
 * ends; disabled tabs are skipped. With items' content, the active tab's panel is rendered below or beside. */
export default function Tabs<K extends string>({
  items,
  value,
  defaultValue,
  onChange,
  variant = "pill",
  orientation = "horizontal",
  label,
  className = "",
  panelClassName = "",
}: {
  items: TabItem<K>[];
  value?: K;
  defaultValue?: K;
  onChange?: (key: K) => void;
  variant?: Variant;
  orientation?: Orientation;
  /** What the tabs choose between, for screen readers. */
  label: string;
  className?: string;
  panelClassName?: string;
}) {
  const id = useId();
  const [own, setOwn] = useState<K | undefined>(defaultValue ?? items.find((i) => !i.disabled)?.key);
  const active = value ?? own;
  const refs = useRef<(HTMLButtonElement | null)[]>([]);

  function select(key: K) {
    if (value === undefined) setOwn(key);
    onChange?.(key);
  }

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    const enabled = items.map((item, i) => ({ item, i })).filter(({ item }) => !item.disabled);
    const at = enabled.findIndex(({ item }) => item.key === active);
    const next = orientation === "vertical" ? "ArrowDown" : "ArrowRight";
    const previous = orientation === "vertical" ? "ArrowUp" : "ArrowLeft";
    let target: number;
    switch (event.key) {
      case next:
        target = (at + 1) % enabled.length;
        break;
      case previous:
        target = (at - 1 + enabled.length) % enabled.length;
        break;
      case "Home":
        target = 0;
        break;
      case "End":
        target = enabled.length - 1;
        break;
      default:
        return;
    }
    event.preventDefault();
    const chosen = enabled[target];
    if (!chosen) return;
    select(chosen.item.key);
    refs.current[chosen.i]?.focus();
  }

  const panel = items.find((i) => i.key === active)?.content;
  const hasPanels = items.some((i) => i.content !== undefined);

  return (
    <div className={`flex gap-4 ${orientation === "vertical" ? "flex-row" : "flex-col"} ${className}`}>
      <div role="tablist" aria-label={label} aria-orientation={orientation} onKeyDown={onKeyDown} className={listClasses[variant][orientation]}>
        {items.map((item, i) => {
          const selected = item.key === active;
          return (
            <button
              key={item.key}
              ref={(el) => {
                refs.current[i] = el;
              }}
              type="button"
              role="tab"
              id={`${id}-tab-${item.key}`}
              aria-selected={selected}
              aria-controls={hasPanels ? `${id}-panel-${item.key}` : undefined}
              tabIndex={selected ? 0 : -1}
              disabled={item.disabled}
              onClick={() => select(item.key)}
              className={tabClasses(variant, orientation, selected)}
            >
              {item.icon}
              {item.label}
              {item.count !== undefined && <span className={countClasses(selected)}>{item.count}</span>}
            </button>
          );
        })}
      </div>
      {hasPanels && active !== undefined && (
        <div
          role="tabpanel"
          id={`${id}-panel-${active}`}
          aria-labelledby={`${id}-tab-${active}`}
          tabIndex={0}
          className={`flex-1 text-sm text-gray-500 outline-none dark:text-gray-400 ${panelClassName}`}
        >
          {panel}
        </div>
      )}
    </div>
  );
}
