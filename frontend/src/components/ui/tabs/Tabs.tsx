import { ReactNode, useState } from "react";

export interface TabItem {
  id: string;
  label: ReactNode;
  icon?: ReactNode;
  badge?: number | string;
  content: ReactNode;
}

export interface TabsProps {
  items: TabItem[];
  defaultActiveId?: string;
  variant?: "default" | "underline" | "icon" | "badge" | "vertical";
  className?: string;
}

export default function Tabs({
  items,
  defaultActiveId,
  variant = "default",
  className = "",
}: TabsProps) {
  const [activeId, setActiveId] = useState(defaultActiveId || items[0]?.id);

  const activeItem = items.find((item) => item.id === activeId);

  const baseButtonClass =
    "px-4 py-2 font-medium transition-colors focus:outline-none";

  const getButtonClass = (isActive: boolean) => {
    switch (variant) {
      case "underline":
        return `${baseButtonClass} text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white ${
          isActive
            ? "text-blue-600 border-b-2 border-blue-600 dark:text-blue-500 dark:border-blue-500"
            : "border-b-2 border-transparent"
        }`;

      case "icon":
        return `${baseButtonClass} flex items-center gap-2 text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white border border-gray-200 dark:border-gray-700 rounded-lg ${
          isActive
            ? "bg-blue-50 text-blue-600 border-blue-200 dark:bg-blue-900/20 dark:text-blue-500 dark:border-blue-700"
            : ""
        }`;

      case "badge":
        return `${baseButtonClass} text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white ${
          isActive
            ? "text-blue-600 border-b-2 border-blue-600 dark:text-blue-500 dark:border-blue-500"
            : "border-b-2 border-transparent"
        }`;

      case "vertical":
        return `${baseButtonClass} w-full text-left ${
          isActive
            ? "bg-blue-50 text-blue-600 border-l-4 border-blue-600 dark:bg-blue-900/20 dark:text-blue-500 dark:border-blue-500"
            : "text-gray-600 hover:bg-gray-50 border-l-4 border-transparent dark:text-gray-400 dark:hover:bg-gray-800"
        }`;

      case "default":
      default:
        return `${baseButtonClass} text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white ${
          isActive
            ? "bg-white text-gray-900 dark:bg-gray-900 dark:text-white"
            : "bg-gray-100 dark:bg-gray-800"
        }`;
    }
  };

  if (variant === "vertical") {
    return (
      <div className={`flex gap-6 ${className}`}>
        <div className="flex flex-col border-l border-gray-200 dark:border-gray-700">
          {items.map((item) => (
            <button
              key={item.id}
              onClick={() => setActiveId(item.id)}
              className={getButtonClass(item.id === activeId)}
            >
              {item.label}
            </button>
          ))}
        </div>
        <div className="flex-1">
          {activeItem && (
            <>
              <h3 className="font-semibold mb-3">{activeItem.label}</h3>
              {activeItem.content}
            </>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className={className}>
      <div
        className={`flex gap-0 border-b border-gray-200 dark:border-gray-700 ${
          variant === "icon" ? "gap-3 border-b-0 border rounded-lg border-gray-200 dark:border-gray-700 p-3" : ""
        }`}
      >
        {items.map((item) => (
          <button
            key={item.id}
            onClick={() => setActiveId(item.id)}
            className={getButtonClass(item.id === activeId)}
          >
            <span className="flex items-center gap-2">
              {item.icon && <span>{item.icon}</span>}
              <span>{item.label}</span>
              {item.badge && (
                <span className="ml-1 inline-flex items-center justify-center px-2 py-1 text-xs font-semibold leading-none text-blue-600 bg-blue-100 rounded-full dark:bg-blue-900 dark:text-blue-200">
                  {item.badge}
                </span>
              )}
            </span>
          </button>
        ))}
      </div>
      <div className="py-4">{activeItem?.content}</div>
    </div>
  );
}
