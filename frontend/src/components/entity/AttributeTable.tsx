import type { AttributeSet } from "../../services/entityProfile";
import Badge from "../ui/badge/Badge";
import { Table, TableBody, TableCell, TableHeader, TableRow } from "../ui/table";
import ClaimValue from "./ClaimValue";

const fieldLabels: Record<string, string> = {
  nome: "Nome",
  dataNascimento: "Data de nascimento",
  cpf: "CPF",
  nomePai: "Nome do pai",
  nomeMae: "Nome da mãe",
  rin: "RIN",
  numeroIdentificacao: "Número de identificação",
};

type AttributeTableProps = {
  attributes: AttributeSet[];
};

// Competing claims are stacked and never merged. There is no primary value and
// no ordering that implies preference: a divergence between two enrollments can
// indicate fraud, so resolving it here would destroy the signal.
export default function AttributeTable({ attributes }: AttributeTableProps) {
  if (attributes.length === 0) {
    return (
      <p className="text-sm text-gray-500 dark:text-gray-400">
        Nenhuma afirmação de identidade registrada para este sujeito.
      </p>
    );
  }

  return (
    <div className="overflow-x-auto">
      <Table className="text-left text-sm">
        <TableHeader>
          <TableRow className="border-b border-gray-100 dark:border-gray-800">
            <TableCell
              isHeader
              className="w-56 px-4 py-3 text-theme-xs font-medium text-gray-500 dark:text-gray-400"
            >
              Campo
            </TableCell>
            <TableCell
              isHeader
              className="px-4 py-3 text-theme-xs font-medium text-gray-500 dark:text-gray-400"
            >
              Afirmações
            </TableCell>
          </TableRow>
        </TableHeader>
        <TableBody>
          {attributes.map((attribute) => (
            <TableRow
              key={attribute.field}
              className={
                attribute.divergent
                  ? "border-b border-l-4 border-gray-100 border-l-error-500 dark:border-gray-800"
                  : "border-b border-gray-100 dark:border-gray-800"
              }
            >
              <TableCell className="px-4 py-4 align-top">
                <div className="flex flex-col gap-2">
                  <span className="font-medium text-gray-700 dark:text-gray-300">
                    {fieldLabels[attribute.field] ?? attribute.field}
                  </span>
                  {attribute.divergent && (
                    <span>
                      <Badge variant="light" size="sm" color="error">
                        {`Divergente · ${attribute.distinctValues} valores`}
                      </Badge>
                    </span>
                  )}
                </div>
              </TableCell>
              <TableCell className="px-4 py-4">
                <div className="flex flex-col gap-3">
                  {attribute.claims.map((claim, index) => (
                    <ClaimValue key={`${claim.sourceType}-${claim.sourceId}-${index}`} claim={claim} />
                  ))}
                  {attribute.missingIn && attribute.missingIn.length > 0 && (
                    <span className="text-theme-xs text-gray-400 dark:text-gray-500">
                      {`Sem valor em: ${attribute.missingIn.join(", ")}`}
                    </span>
                  )}
                </div>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}
