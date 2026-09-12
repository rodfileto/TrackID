import ComponentCard from "../../components/common/ComponentCard";
import AttributeTable from "../../components/entity/AttributeTable";
import ProvenanceChip from "../../components/entity/ProvenanceChip";
import Badge from "../../components/ui/badge/Badge";
import type { PersonProfile } from "../../services/entityProfile";

function EmptySection({ children }: { children: string }) {
  return <p className="text-sm text-gray-500 dark:text-gray-400">{children}</p>;
}

const claimLabels: Record<string, string> = {
  IDENTIFICATION: "Identificação",
  SAME_SOURCE: "Mesma origem",
  EXCLUSION: "Exclusão",
  INCONCLUSIVE: "Inconclusivo",
};

const roleLabels: Record<string, string> = {
  QUESTIONED: "Questionado",
  REFERENCE: "Referência",
};

// A document that enrolls a subject's identity — an InfoBio NIF today, a CNH, passaporte, or
// carteira de identidade whenever those sources exist. One shared shape, distinguished by type,
// the same way the graph distinguishes them (see docs/graph-schema.md's "Enrollment documents").
const documentTypeLabels: Record<string, string> = {
  INFOBIO_NIF: "InfoBio NIF",
  CNH: "CNH",
  PASSPORT: "Passaporte",
  RG: "Carteira de identidade",
};

const enrollmentSourceLabels: Record<string, string> = {
  INFOBIO_SYNC: "confirmado via InfoBio",
  INFOBIO_CONTAINER_FUSION: "mesmo contêiner InfoBio que outro documento já confirmado",
  FINGERPRINT_MATCH: "correspondência de impressão digital",
};

export default function PersonSections({ profile }: { profile: PersonProfile }) {
  const { sections, identity, sources } = profile;

  return (
    <div className="flex flex-col gap-6">
      <ComponentCard
        title="Identidade"
        desc="Cada valor é uma afirmação de uma fonte. Divergências são exibidas, nunca reconciliadas."
      >
        {sources.enrollments === "UNAVAILABLE" && (
          <div className="rounded-lg border-l-4 border-warning-500 bg-warning-50 px-4 py-3 text-sm text-warning-600 dark:bg-warning-500/10 dark:text-orange-400">
            Dados demográficos dos documentos de identificação indisponíveis. Os valores abaixo vêm
            apenas do grafo e podem estar incompletos.
          </div>
        )}
        <AttributeTable attributes={identity.attributes} />
      </ComponentCard>

      <ComponentCard
        title="Documentos de identificação"
        desc="Documentos que atestam esta identidade — hoje inscrições InfoBio, com o estado de revisão de cada vínculo."
      >
        {sections.enrollments.length === 0 ? (
          <EmptySection>Nenhum documento de identificação vinculado a este sujeito.</EmptySection>
        ) : (
          <div className="flex flex-col gap-3">
            {sections.enrollments.map((enrollment) => (
              <div
                key={enrollment.nif}
                className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-gray-100 px-4 py-3 dark:border-gray-800"
              >
                <div className="flex flex-col gap-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant="light" size="sm" color="info">
                      {documentTypeLabels[enrollment.documentType ?? ""] ?? enrollment.documentType ?? "Documento"}
                    </Badge>
                    <span className="font-mono text-sm font-medium text-gray-800 dark:text-white/90">
                      {enrollment.nif}
                    </span>
                  </div>
                  <span className="text-theme-xs text-gray-500 dark:text-gray-400">
                    {[
                      enrollment.containerNumber ? `contêiner ${enrollment.containerNumber}` : "",
                      enrollment.source ? (enrollmentSourceLabels[enrollment.source] ?? enrollment.source) : "",
                    ]
                      .filter(Boolean)
                      .join(" · ")}
                  </span>
                </div>
                <ProvenanceChip
                  status={enrollment.status}
                  confidence={enrollment.confidence}
                  title={enrollment.source}
                />
              </div>
            ))}
          </div>
        )}
      </ComponentCard>

      <ComponentCard
        title="Evidências"
        desc="Conclusões de comparação que alcançam este sujeito, com autor, método e estado de revisão."
      >
        {sections.evidence.length === 0 ? (
          <EmptySection>Nenhuma evidência alcança este sujeito.</EmptySection>
        ) : (
          <div className="flex flex-col gap-4">
            {sections.evidence.map((evidence) => (
              <div
                key={evidence.evidenceId}
                className="rounded-lg border border-gray-100 px-4 py-3 dark:border-gray-800"
              >
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-sm font-medium text-gray-800 dark:text-white/90">
                    {evidence.claim ? (claimLabels[evidence.claim] ?? evidence.claim) : "Sem alegação"}
                  </span>
                  {evidence.method && (
                    <Badge variant="light" size="sm" color="info">
                      {evidence.method}
                    </Badge>
                  )}
                  <ProvenanceChip
                    status={evidence.status}
                    confidence={evidence.confidence}
                    title={evidence.evidenceId}
                  />
                </div>
                <p className="mt-1 text-theme-xs text-gray-500 dark:text-gray-400">
                  {[
                    evidence.assertedBy ? `afirmado por ${evidence.assertedBy}` : "",
                    evidence.viaNif ? `via NIF ${evidence.viaNif}` : "",
                  ]
                    .filter(Boolean)
                    .join(" · ")}
                </p>
                <div className="mt-3 flex flex-col gap-2">
                  {evidence.traits.map((trait) => (
                    <div
                      key={trait.featureId}
                      className="flex flex-wrap items-center gap-2 text-theme-xs text-gray-600 dark:text-gray-400"
                    >
                      {trait.role && (
                        <Badge variant="light" size="sm" color="light">
                          {roleLabels[trait.role] ?? trait.role}
                        </Badge>
                      )}
                      <span className="font-mono">{trait.featureId}</span>
                      {trait.caseId && <span>{`caso ${trait.caseId}`}</span>}
                      {trait.documentKind && <span>{trait.documentKind}</span>}
                    </div>
                  ))}
                </div>
              </div>
            ))}
          </div>
        )}
      </ComponentCard>

      <ComponentCard
        title="Biometria registrada"
        desc="Amostras capturadas diretamente deste sujeito. Resultados de comparação não aparecem aqui."
      >
        {sections.biometrics.length === 0 ? (
          <EmptySection>Nenhuma biometria registrada por inscrição.</EmptySection>
        ) : (
          <div className="flex flex-col gap-2">
            {sections.biometrics.map((feature) => (
              <div
                key={feature.featureId}
                className="flex flex-wrap items-center gap-2 rounded-lg border border-gray-100 px-4 py-3 text-sm dark:border-gray-800"
              >
                <span className="font-mono text-gray-800 dark:text-white/90">
                  {feature.featureId}
                </span>
                <Badge variant="light" size="sm" color="info">
                  {feature.modality}
                </Badge>
                {feature.finger && (
                  <span className="text-theme-xs text-gray-500 dark:text-gray-400">
                    {feature.finger}
                  </span>
                )}
              </div>
            ))}
          </div>
        )}
      </ComponentCard>

      <ComponentCard
        title="Casos"
        desc="Casos em que este sujeito foi diretamente registrado como envolvido."
      >
        {sections.cases.length === 0 ? (
          <EmptySection>Nenhum envolvimento direto em casos.</EmptySection>
        ) : (
          <div className="flex flex-col gap-2">
            {sections.cases.map((involvement) => (
              <div
                key={involvement.caseId}
                className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-gray-100 px-4 py-3 text-sm dark:border-gray-800"
              >
                <span className="font-mono text-gray-800 dark:text-white/90">
                  {involvement.caseId}
                </span>
                <ProvenanceChip
                  status={involvement.status}
                  confidence={involvement.confidence}
                  title={involvement.role}
                />
              </div>
            ))}
          </div>
        )}
      </ComponentCard>

      {sections.merges.length > 0 && (
        <ComponentCard
          title="Histórico de reconciliação"
          desc="Fusões de registros de pessoa. Nada é apagado: as origens permanecem auditáveis."
        >
          <div className="flex flex-col gap-2">
            {sections.merges.map((merge, index) => (
              <div
                key={merge.eventId ?? `merge-${index}`}
                className="rounded-lg border border-gray-100 px-4 py-3 text-sm dark:border-gray-800"
              >
                <p className="text-gray-800 dark:text-white/90">
                  {merge.mergedIntoPersonId
                    ? `Fundido em ${merge.mergedIntoPersonId}`
                    : `Origem incorporada: ${merge.sourcePersonId ?? "-"}`}
                </p>
                {merge.reason && (
                  <p className="text-theme-xs text-gray-500 dark:text-gray-400">{merge.reason}</p>
                )}
              </div>
            ))}
          </div>
        </ComponentCard>
      )}
    </div>
  );
}
