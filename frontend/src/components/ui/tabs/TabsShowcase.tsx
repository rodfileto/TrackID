import Tabs from "./Tabs";
import ComponentCard from "../../common/ComponentCard";
import {
  PieChartIcon,
  InfoIcon,
  UserIcon,
  GridIcon,
} from "../../../icons";

export function DefaultTabShowcase() {
  const items = [
    {
      id: "overview",
      label: "Overview",
      content: (
        <p className="text-gray-600 dark:text-gray-400">
          Overview ipsum dolor sit amet consectetur. Non vitae facilisis urna
          tortor placerat egestas donec. Faucibus diam gravida enim elit lacus
          a. Tincidunt fermentum condimentum quis et a et tempus. Tristique
          urna nisl nulla elit sit libero scelerisque ante.
        </p>
      ),
    },
    {
      id: "notification",
      label: "Notification",
      content: (
        <p className="text-gray-600 dark:text-gray-400">
          Notification content goes here. You can add any content you want in
          this tab.
        </p>
      ),
    },
    {
      id: "analytics",
      label: "Analytics",
      content: (
        <p className="text-gray-600 dark:text-gray-400">
          Analytics content goes here with charts and metrics.
        </p>
      ),
    },
    {
      id: "customers",
      label: "Customers",
      content: (
        <p className="text-gray-600 dark:text-gray-400">
          Customer data and information will be displayed here.
        </p>
      ),
    },
  ];

  return (
    <ComponentCard title="Default Tab">
      <Tabs items={items} />
    </ComponentCard>
  );
}

export function UnderlineTabShowcase() {
  const items = [
    {
      id: "overview",
      label: "Overview",
      content: (
        <p className="text-gray-600 dark:text-gray-400">
          Overview ipsum dolor sit amet consectetur. Non vitae facilisis urna
          tortor placerat egestas donec. Faucibus diam gravida enim elit lacus
          a. Tincidunt fermentum condimentum quis et a et tempus. Tristique
          urna nisl nulla elit sit libero scelerisque ante.
        </p>
      ),
    },
    {
      id: "notification",
      label: "Notification",
      content: (
        <p className="text-gray-600 dark:text-gray-400">
          Notification content goes here.
        </p>
      ),
    },
    {
      id: "analytics",
      label: "Analytics",
      content: (
        <p className="text-gray-600 dark:text-gray-400">
          Analytics content goes here.
        </p>
      ),
    },
    {
      id: "customers",
      label: "Customers",
      content: (
        <p className="text-gray-600 dark:text-gray-400">
          Customer content goes here.
        </p>
      ),
    },
  ];

  return (
    <ComponentCard title="Tab With Underline">
      <Tabs items={items} variant="underline" />
    </ComponentCard>
  );
}

export function IconTabShowcase() {
  const items = [
    {
      id: "overview",
      label: "Overview",
      icon: <PieChartIcon className="w-5 h-5" />,
      content: (
        <p className="text-gray-600 dark:text-gray-400">
          Overview with icon indicator.
        </p>
      ),
    },
    {
      id: "notification",
      label: "Notification",
      icon: <InfoIcon className="w-5 h-5" />,
      content: (
        <p className="text-gray-600 dark:text-gray-400">
          Notification with icon indicator.
        </p>
      ),
    },
    {
      id: "analytics",
      label: "Analytics",
      icon: <GridIcon className="w-5 h-5" />,
      content: (
        <p className="text-gray-600 dark:text-gray-400">
          Analytics with icon indicator.
        </p>
      ),
    },
    {
      id: "customers",
      label: "Customers",
      icon: <UserIcon className="w-5 h-5" />,
      content: (
        <p className="text-gray-600 dark:text-gray-400">
          Customers with icon indicator.
        </p>
      ),
    },
  ];

  return (
    <ComponentCard title="Tab with line and icon">
      <Tabs items={items} variant="icon" />
    </ComponentCard>
  );
}

export function BadgeTabShowcase() {
  const items = [
    {
      id: "overview",
      label: "Overview",
      badge: 8,
      content: (
        <p className="text-gray-600 dark:text-gray-400">
          Overview with badge notification.
        </p>
      ),
    },
    {
      id: "notification",
      label: "Notification",
      content: (
        <p className="text-gray-600 dark:text-gray-400">
          Notification tab content.
        </p>
      ),
    },
    {
      id: "analytics",
      label: "Analytics",
      badge: 4,
      content: (
        <p className="text-gray-600 dark:text-gray-400">
          Analytics with badge.
        </p>
      ),
    },
    {
      id: "customers",
      label: "Customers",
      badge: 12,
      content: (
        <p className="text-gray-600 dark:text-gray-400">
          Customers with badge count.
        </p>
      ),
    },
  ];

  return (
    <ComponentCard title="Tab with badge">
      <Tabs items={items} variant="badge" />
    </ComponentCard>
  );
}

export function VerticalTabShowcase() {
  const items = [
    {
      id: "overview",
      label: "Overview",
      content: (
        <p className="text-gray-600 dark:text-gray-400">
          Overview ipsum dolor sit amet consectetur. Non vitae facilisis urna
          tortor placerat egestas donec.
        </p>
      ),
    },
    {
      id: "settings",
      label: "Settings",
      content: (
        <p className="text-gray-600 dark:text-gray-400">
          Settings and configuration options go here.
        </p>
      ),
    },
    {
      id: "notification",
      label: "Notification",
      content: (
        <p className="text-gray-600 dark:text-gray-400">
          Notification preferences and settings.
        </p>
      ),
    },
  ];

  return (
    <ComponentCard title="Vertical Tab">
      <Tabs items={items} variant="vertical" className="bg-white dark:bg-gray-800 p-4 rounded-lg" />
    </ComponentCard>
  );
}
