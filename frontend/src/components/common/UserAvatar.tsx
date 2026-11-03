import { useEffect, useState } from "react";
import InitialsAvatar from "./InitialsAvatar";
import { fetchAvatar, type UserProfile } from "../../services/auth";

/** A user's picture, or their initials while they have none (or it is loading). */
export default function UserAvatar({ user, className = "h-11 w-11 text-sm" }: {
  user: Pick<UserProfile, "id" | "first_name" | "last_name" | "avatar_updated_at"> | null;
  className?: string;
}) {
  const [url, setUrl] = useState<string | null>(null);
  const id = user?.id;
  const version = user?.avatar_updated_at;

  useEffect(() => {
    if (!id || !version) {
      setUrl(null);
      return;
    }
    let cancelled = false;
    let created: string | null = null;
    fetchAvatar(id, version)
      .then((u) => {
        created = u;
        if (cancelled) URL.revokeObjectURL(u);
        else setUrl(u);
      })
      .catch(() => !cancelled && setUrl(null));
    return () => {
      cancelled = true;
      if (created) URL.revokeObjectURL(created);
    };
  }, [id, version]);

  if (!url) return <InitialsAvatar firstName={user?.first_name} lastName={user?.last_name} className={className} />;
  return <img src={url} alt="" className={`shrink-0 rounded-full object-cover ${className}`} />;
}
