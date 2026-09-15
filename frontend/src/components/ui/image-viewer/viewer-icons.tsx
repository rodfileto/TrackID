// Small stroke icons for the ImageViewer toolbar, kept local to this
// component instead of the shared icon set (src/icons) since they're only
// meaningful as viewer controls.

import type { ReactNode } from "react";

function IconBase({
  className,
  children,
}: {
  className?: string;
  children: ReactNode;
}) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.8}
      strokeLinecap="round"
      strokeLinejoin="round"
      className={className}
    >
      {children}
    </svg>
  );
}

export function ZoomInIcon({ className }: { className?: string }) {
  return (
    <IconBase className={className}>
      <circle cx="10.5" cy="10.5" r="6.5" />
      <path d="M15.5 15.5 21 21" />
      <path d="M10.5 7.5v6M7.5 10.5h6" />
    </IconBase>
  );
}

export function ZoomOutIcon({ className }: { className?: string }) {
  return (
    <IconBase className={className}>
      <circle cx="10.5" cy="10.5" r="6.5" />
      <path d="M15.5 15.5 21 21" />
      <path d="M7.5 10.5h6" />
    </IconBase>
  );
}

export function FitToFrameIcon({ className }: { className?: string }) {
  return (
    <IconBase className={className}>
      <path d="M9 3H5a2 2 0 0 0-2 2v4" />
      <path d="M15 3h4a2 2 0 0 1 2 2v4" />
      <path d="M9 21H5a2 2 0 0 1-2-2v-4" />
      <path d="M15 21h4a2 2 0 0 0 2-2v-4" />
    </IconBase>
  );
}

export function RotateIcon({ className }: { className?: string }) {
  return (
    <IconBase className={className}>
      <path d="M3 12a9 9 0 1 1 3 6.7" />
      <path d="M3 21v-5h5" />
    </IconBase>
  );
}
