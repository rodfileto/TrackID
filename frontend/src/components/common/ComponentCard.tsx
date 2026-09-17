import { useId, useState } from "react";
import { ChevronDownIcon } from "../../icons";

interface ComponentCardProps {
  title: string;
  children: React.ReactNode;
  className?: string; // Additional custom classes for styling
  desc?: string; // Description text
  /** Makes the header a toggle that shows/hides the body. */
  collapsible?: boolean;
  /** Initial state when `collapsible`. */
  defaultCollapsed?: boolean;
}

const ComponentCard: React.FC<ComponentCardProps> = ({
  title,
  children,
  className = "",
  desc = "",
  collapsible = false,
  defaultCollapsed = false,
}) => {
  const [collapsed, setCollapsed] = useState(collapsible && defaultCollapsed);
  const bodyId = useId();

  const heading = (
    <>
      <h3 className="text-base font-medium text-gray-800 dark:text-white/90">
        {title}
      </h3>
      {desc && (
        <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">{desc}</p>
      )}
    </>
  );

  return (
    <div
      className={`rounded-2xl border border-gray-200 bg-white dark:border-gray-800 dark:bg-white/[0.03] ${className}`}
    >
      {/* Card Header */}
      {collapsible ? (
        <button
          type="button"
          onClick={() => setCollapsed((current) => !current)}
          aria-expanded={!collapsed}
          aria-controls={bodyId}
          className="flex w-full items-center justify-between gap-4 px-6 py-5 text-left"
        >
          <span className="min-w-0">{heading}</span>
          <ChevronDownIcon
            className={`h-5 w-5 shrink-0 text-gray-500 transition-transform dark:text-gray-400 ${
              collapsed ? "" : "rotate-180"
            }`}
          />
        </button>
      ) : (
        <div className="px-6 py-5">{heading}</div>
      )}

      {/* Card Body -- kept mounted while collapsed so its contents (e.g. an
          image viewer's zoom and filters) survive a collapse/expand. */}
      <div
        id={bodyId}
        hidden={collapsed}
        className="p-4 border-t border-gray-100 dark:border-gray-800 sm:p-6"
      >
        <div className="space-y-6">{children}</div>
      </div>
    </div>
  );
};

export default ComponentCard;
