import PageBreadcrumb from "../components/common/PageBreadCrumb";
import PageMeta from "../components/common/PageMeta";
import {
  DefaultTabShowcase,
  UnderlineTabShowcase,
  IconTabShowcase,
  BadgeTabShowcase,
  VerticalTabShowcase,
} from "../components/ui/tabs/TabsShowcase";

export default function Tabs() {
  return (
    <div>
      <PageMeta
        title="React.js Tabs Dashboard | TailAdmin - React.js Admin Dashboard Template"
        description="This is React.js Tabs Dashboard page for TailAdmin - React.js Tailwind CSS Admin Dashboard Template"
      />
      <PageBreadcrumb pageTitle="Tabs" />
      <div className="space-y-6">
        <DefaultTabShowcase />
        <UnderlineTabShowcase />
        <IconTabShowcase />
        <BadgeTabShowcase />
        <VerticalTabShowcase />
      </div>
    </div>
  );
}
