import { useCallback, useEffect, useState } from "react";
import { useParams } from "react-router";
import PageMeta from "../../components/common/PageMeta";
import Badge from "../../components/ui/badge/Badge";
import { getPersonProfile, type PersonProfile } from "../../services/entityProfile";
import PersonSections from "./PersonSections";

const statusLabels: Record<string, string> = {
  UNKNOWN: "Não identificado",
  IDENTIFIED: "Identificado",
  ACTIVE: "Ativo",
  MERGED: "Fundido",
};

function statusColor(status?: string) {
  switch (status) {
    case "IDENTIFIED":
      return "success" as const;
    case "MERGED":
      return "info" as const;
    default:
      return "light" as const;
  }
}

export default function EntityPage() {
  const { subjectType, subjectId } = useParams();
  const [profile, setProfile] = useState<PersonProfile | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const loadProfile = useCallback(async () => {
    if (!subjectId) return;
    setLoading(true);
    setError("");
    try {
      setProfile(await getPersonProfile(subjectId));
    } catch (requestError) {
      setProfile(null);
      setError(
        requestError instanceof Error
          ? requestError.message
          : "Erro ao carregar o perfil da entidade",
      );
    } finally {
      setLoading(false);
    }
  }, [subjectId]);

  useEffect(() => {
    void loadProfile();
  }, [loadProfile]);

  if (subjectType !== "person") {
    return (
      <>
        <PageMeta title="Entidade | TrackID" description="Perfil de entidade" />
        <p className="text-sm text-gray-500 dark:text-gray-400">
          Tipo de entidade não suportado.
        </p>
      </>
    );
  }

  return (
    <>
      <PageMeta
        title={`${profile?.subject.displayName || subjectId || "Entidade"} | TrackID`}
        description="Perfil centrado no alvo, com proveniência por afirmação"
      />

      <header className="mb-6">
        <div className="flex flex-wrap items-center gap-3">
          <h1 className="text-xl font-semibold text-gray-800 dark:text-white/90">
            {profile?.subject.displayName || "Sujeito não identificado"}
          </h1>
          {profile?.subject.status && (
            <Badge variant="light" size="sm" color={statusColor(profile.subject.status)}>
              {statusLabels[profile.subject.status] ?? profile.subject.status}
            </Badge>
          )}
          {profile && profile.identity.divergentFields.length > 0 && (
            <Badge variant="light" size="sm" color="error">
              {`${profile.identity.divergentFields.length} campo(s) divergente(s)`}
            </Badge>
          )}
        </div>
        <p className="mt-1 font-mono text-theme-xs text-gray-500 dark:text-gray-400">
          {subjectId}
        </p>
        {profile?.mergedInto && (
          <p className="mt-2 text-sm text-gray-600 dark:text-gray-400">
            {`Este registro foi fundido em ${profile.mergedInto}.`}
          </p>
        )}
      </header>

      {loading && (
        <p className="text-sm text-gray-500 dark:text-gray-400">Carregando perfil...</p>
      )}

      {!loading && error && (
        <div className="border-l-4 border-error-500 bg-error-50 px-4 py-3 text-sm text-error-600 dark:bg-error-500/10 dark:text-error-500">
          {error}
        </div>
      )}

      {!loading && !error && profile && <PersonSections profile={profile} />}
    </>
  );
}
