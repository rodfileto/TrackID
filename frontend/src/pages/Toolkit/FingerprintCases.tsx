import { FormEvent, useEffect, useState } from "react";
import { useNavigate } from "react-router";
import PageMeta from "../../components/common/PageMeta";
import { FingerprintCase, listFingerprintCases } from "../../services/fingerprintCases";
import { resolvePersonIdForEnrollment, type EntityProfileError } from "../../services/entityProfile";

const pageSize = 25;

export default function FingerprintCases() {
  const navigate = useNavigate();
  const [records, setRecords] = useState<FingerprintCase[]>([]);
  const [total, setTotal] = useState(0);
  const [totalPages, setTotalPages] = useState(0);
  const [page, setPage] = useState(1);
  const [caseId, setCaseId] = useState("");
  const [query, setQuery] = useState("");
  const [comparisonType, setComparisonType] = useState("");
  const [reference, setReference] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [selected, setSelected] = useState<FingerprintCase | null>(null);
  const [resolvingSubject, setResolvingSubject] = useState(false);
  const [resolveError, setResolveError] = useState("");

  async function openSubjectProfile(nif: string) {
    setResolvingSubject(true);
    setResolveError("");
    try {
      const personId = await resolvePersonIdForEnrollment(nif);
      if (!personId) {
        setResolveError("Este NIF ainda não está vinculado a nenhuma identidade conhecida.");
        return;
      }
      navigate(`/entity/person/${encodeURIComponent(personId)}`);
    } catch (requestError) {
      const status = (requestError as EntityProfileError)?.status;
      setResolveError(
        status === 404
          ? "Este NIF ainda não está vinculado a nenhuma identidade conhecida."
          : requestError instanceof Error
            ? requestError.message
            : "Não foi possível localizar este NIF.",
      );
    } finally {
      setResolvingSubject(false);
    }
  }

  async function loadRecords(nextPage = page) {
    setLoading(true);
    setError("");
    try {
      const result = await listFingerprintCases({ page: nextPage, pageSize, caseId, query, comparisonType, reference });
      setRecords(result.items);
      setTotal(result.total);
      setTotalPages(result.totalPages);
      setPage(result.page);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "Erro ao carregar os casos criminais");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { void loadRecords(1); }, []);

  function submit(event: FormEvent) {
    event.preventDefault();
    void loadRecords(1);
  }

  return (
    <>
      <PageMeta title="Fingerprint Cases | TrackID" description="Casos criminais com impressões digitais latentes" />
      <div className="space-y-6">
        <header className="flex flex-col justify-between gap-3 sm:flex-row sm:items-end"><div><p className="text-xs font-semibold uppercase tracking-[0.18em] text-brand-500">Toolkit / papiloscopia</p><h1 className="mt-2 text-3xl font-semibold text-gray-900 dark:text-white">Casos com impressões digitais</h1><p className="mt-2 text-sm text-gray-500 dark:text-gray-400">Registros de vestígios latentes e seus vínculos de comparação.</p></div><span className="text-sm text-gray-500 dark:text-gray-400">{total} registros</span></header>
        <section className="rounded-2xl border border-gray-200 bg-white p-5 shadow-theme-sm dark:border-gray-800 dark:bg-gray-900"><form onSubmit={submit} className="grid gap-3 md:grid-cols-2 xl:grid-cols-5"><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Buscar na descrição" className="h-11 rounded-lg border border-gray-300 bg-transparent px-3 text-sm outline-none focus:border-brand-500 dark:border-gray-700 dark:text-white xl:col-span-2" /><input value={caseId} onChange={(event) => setCaseId(event.target.value)} placeholder="Identificador do caso" className="h-11 rounded-lg border border-gray-300 bg-transparent px-3 text-sm outline-none focus:border-brand-500 dark:border-gray-700 dark:text-white" /><select value={comparisonType} onChange={(event) => setComparisonType(event.target.value)} className="h-11 rounded-lg border border-gray-300 bg-transparent px-3 text-sm outline-none focus:border-brand-500 dark:border-gray-700 dark:text-white"><option value="">Todos os tipos</option><option value="TPvsULF">TP vs ULF</option><option value="LTvsULF">LT vs ULF</option></select><input value={reference} onChange={(event) => setReference(event.target.value)} placeholder="NIF ou caso relacionado" className="h-11 rounded-lg border border-gray-300 bg-transparent px-3 text-sm outline-none focus:border-brand-500 dark:border-gray-700 dark:text-white" /><button className="h-11 rounded-lg bg-brand-500 px-5 text-sm font-semibold text-white hover:bg-brand-600">Filtrar</button></form>{error && <div className="mt-4 border-l-4 border-error-500 bg-error-50 px-4 py-3 text-sm text-error-700 dark:bg-error-950/30 dark:text-error-300">{error}</div>}</section>
        <section className="overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-theme-sm dark:border-gray-800 dark:bg-gray-900"><div className="overflow-x-auto"><table className="w-full min-w-[900px] text-left text-sm"><thead className="bg-gray-50 text-xs uppercase text-gray-500 dark:bg-white/5 dark:text-gray-400"><tr><th className="px-5 py-3">Caso</th><th className="px-5 py-3">Descrição</th><th className="px-5 py-3">Tipo</th><th className="px-5 py-3">Referência</th><th className="px-5 py-3">Usuário</th><th className="px-5 py-3 text-right">Detalhes</th></tr></thead><tbody className="divide-y divide-gray-100 dark:divide-gray-800">{loading ? <tr><td colSpan={6} className="px-5 py-14 text-center text-gray-500">Carregando registros...</td></tr> : records.length === 0 ? <tr><td colSpan={6} className="px-5 py-14 text-center text-gray-500">Nenhum caso encontrado.</td></tr> : records.map((record) => <tr key={`${record.sourceDataset}-${record.sourceRow}`} className="align-top text-gray-700 dark:text-gray-300"><td className="whitespace-nowrap px-5 py-4 font-mono text-xs text-gray-900 dark:text-white">{record.caseId}</td><td className="max-w-md px-5 py-4">{record.description}</td><td className="whitespace-nowrap px-5 py-4">{record.comparisonType ?? "-"}</td><td className="whitespace-nowrap px-5 py-4 font-mono text-xs">{record.relatedReference ?? "-"}</td><td className="whitespace-nowrap px-5 py-4">{record.responsibleUser ?? "-"}</td><td className="px-5 py-4 text-right"><button type="button" onClick={() => { setSelected(record); setResolveError(""); }} className="font-semibold text-brand-500 hover:text-brand-700">Abrir</button></td></tr>)}</tbody></table></div><div className="flex items-center justify-between border-t border-gray-100 px-5 py-4 text-sm dark:border-gray-800"><span className="text-gray-500 dark:text-gray-400">Página {page} de {Math.max(totalPages, 1)}</span><div className="flex gap-2"><button type="button" disabled={page <= 1 || loading} onClick={() => void loadRecords(page - 1)} className="rounded-lg border border-gray-300 px-3 py-2 text-gray-600 disabled:opacity-40 dark:border-gray-700 dark:text-gray-300">Anterior</button><button type="button" disabled={page >= totalPages || loading} onClick={() => void loadRecords(page + 1)} className="rounded-lg border border-gray-300 px-3 py-2 text-gray-600 disabled:opacity-40 dark:border-gray-700 dark:text-gray-300">Próxima</button></div></div></section>
      </div>
      {selected && <div role="dialog" aria-modal="true" className="fixed inset-0 z-50 flex items-center justify-center bg-gray-950/60 p-4" onClick={() => setSelected(null)}><div className="w-full max-w-2xl rounded-2xl bg-white p-6 shadow-theme-xl dark:bg-gray-900" onClick={(event) => event.stopPropagation()}><div className="flex items-start justify-between gap-4"><div><p className="text-xs uppercase tracking-widest text-brand-500">Registro de origem</p><h2 className="mt-1 font-mono text-lg font-semibold text-gray-900 dark:text-white">{selected.caseId}</h2></div><button type="button" onClick={() => setSelected(null)} aria-label="Fechar detalhes" className="text-xl text-gray-400">×</button></div><dl className="mt-6 grid gap-4 sm:grid-cols-2"><div className="sm:col-span-2"><dt className="text-xs uppercase text-gray-400">Descrição</dt><dd className="mt-1 text-sm text-gray-900 dark:text-gray-200">{selected.description}</dd></div>{[["Tipo", selected.comparisonType], ["Referência", selected.relatedReference], ["Tipo de referência", selected.relatedReferenceKind], ["Usuário", selected.responsibleUser], ["Linha de origem", String(selected.sourceRow)]].map(([label, value]) => <div key={label}><dt className="text-xs uppercase text-gray-400">{label}</dt><dd className="mt-1 text-sm text-gray-900 dark:text-gray-200">{value ?? "-"}</dd></div>)}</dl>{selected.relatedReferenceKind === "INFOBIO_NIF" && selected.relatedReference && <div className="mt-6"><button type="button" disabled={resolvingSubject} onClick={() => void openSubjectProfile(selected.relatedReference!.replace(/[^0-9]/g, ""))} className="inline-flex text-sm font-medium text-brand-500 hover:underline disabled:opacity-50">{resolvingSubject ? "Localizando identidade..." : <>Ver perfil do sujeito &rarr;</>}</button>{resolveError && <p className="mt-2 text-sm text-error-600 dark:text-error-500">{resolveError}</p>}</div>}</div></div>}
    </>
  );
}