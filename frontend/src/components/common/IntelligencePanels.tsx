import { Suspense, type ComponentType } from "react";
import { useCurrentUser } from "../../context/CurrentUserContext";

/** The panels an organization added to a page (Extension.casePanels and so on), for intelligence analysts. */
export default function IntelligencePanels<P extends object>({ panels, props }: { panels?: ComponentType<P>[]; props: P }) {
  const { isIntelligence } = useCurrentUser();
  if (!isIntelligence || !panels?.length) return null;
  return (
    <>
      {panels.map((Panel, index) => (
        <Suspense key={index} fallback={null}>
          <Panel {...props} />
        </Suspense>
      ))}
    </>
  );
}
