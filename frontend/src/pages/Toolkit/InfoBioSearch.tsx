import { useEffect, useMemo, useState } from "react";
import type { FormEvent } from "react";
import PageMeta from "../../components/common/PageMeta";
import {
  createInfoBioSession,
  entryValue,
  getInfoBioDetails,
  getInfoBioSessionStatus,
  InfoBioEntry,
  invalidateInfoBioSession,
  isInfoBioAuthError,
  SearchMode,
  searchInfoBio,
} from "../../services/infobio";

const modeLabels: Record<SearchMode, string> = {
  nome: "Nome",
  nif: "NIF",
  rin: "RIN",
};

export default function InfoBioSearch() {
  const [mode, setMode] = useState<SearchMode>("nome");
  const [value, setValue] = useState("");
  const [nomeMae, setNomeMae] = useState("");
  const [nomePai, setNomePai] = useState("");
  const [dataNascimento, setDataNascimento] = useState("");
  const [password, setPassword] = useState("");
  const [sessionActive, setSessionActive] = useState<boolean | null>(null);
  const [sessionPrompt, setSessionPrompt] = useState(false);
  const [sessionExpired, setSessionExpired] = useState(false);
  const [loading, setLoading] = useState(false);
  const [authLoading, setAuthLoading] = useState(false);
  const [error, setError] = useState("");
  const [results, setResults] = useState<InfoBioEntry[]>([]);
  const [selected, setSelected] = useState<InfoBioEntry | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);

  useEffect(() => {
    getInfoBioSessionStatus()
      .then(({ active }) => {
        setSessionActive(active);
        setSessionPrompt(!active);
      })
      .catch(() => setSessionActive(null));
  }, []);

  const resultCount = useMemo(() => results.length, [results]);

  async function handleAuth(event: FormEvent) {
    event.preventDefault();
    if (!password.trim()) return;
    setAuthLoading(true);
    setError("");
    try {
      await createInfoBioSession(password);
      setPassword("");
      setSessionActive(true);
      setSessionPrompt(false);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "Não foi possível autenticar no InfoBio");
    } finally {
      setAuthLoading(false);
    }
  }

  async function handleSearch(event: FormEvent) {
    event.preventDefault();
    if (!value.trim()) {
      setError(`Informe um ${modeLabels[mode].toLowerCase()} para buscar.`);
      return;
    }
    setLoading(true);
    setError("");
    try {
      const response = await searchInfoBio({ mode, value: value.trim(), nomeMae, nomePai, dataNascimento });
      setResults(response.resultados ?? (response.resultado ? [response.resultado] : []));
    } catch (requestError) {
      if (isInfoBioAuthError(requestError)) {
        setSessionPrompt(true);
        setSessionExpired(Boolean(requestError.expired));
        setSessionActive(false);
      }
      setError(requestError instanceof Error ? requestError.message : "Erro ao buscar no InfoBio");
    } finally {
      setLoading(false);
    }
  }

  async function handleDetails(entry: InfoBioEntry) {
    const nif = entryValue(entry, "NIF", "nif", "numeroNIF");
    if (nif === "-") return;
    setSelected(entry);
    setDetailLoading(true);
    setError("");
    try {
      setSelected(await getInfoBioDetails(nif));
    } catch (requestError) {
      if (isInfoBioAuthError(requestError)) {
        setSelected(null);
        setSessionPrompt(true);
        setSessionExpired(Boolean(requestError.expired));
        setSessionActive(false);
      }
      setError(requestError instanceof Error ? requestError.message : "Erro ao carregar detalhes");
    } finally {
      setDetailLoading(false);
    }
  }

  async function handleInvalidate() {
    await invalidateInfoBioSession().catch(() => undefined);
    setSessionActive(false);
    setSessionPrompt(true);
    setSessionExpired(false);
  }

  return (
    <>
      <PageMeta title="InfoBio Search | TrackID" description="Consulta de registros biométricos no InfoBio" />
      <div className="space-y-6">
        <header className="relative overflow-hidden rounded-2xl bg-gray-900 px-6 py-7 text-white shadow-theme-lg sm:px-8">
          <div className="absolute right-0 top-0 h-full w-1/3 bg-gradient-to-l from-brand-500/30 to-transparent" />
          <div className="relative flex flex-col justify-between gap-5 sm:flex-row sm:items-end">
            <div>
              <p className="mb-2 text-xs font-semibold uppercase tracking-[0.18em] text-brand-300">Toolkit / biometria</p>
              <h1 className="text-3xl font-semibold tracking-tight">Busca InfoBio</h1>
              <p className="mt-2 max-w-xl text-sm text-gray-300">Consulte identificações e abra o registro completo de um NIF.</p>
            </div>
            <div className="flex items-center gap-2 text-xs text-gray-300">
              <span className={`h-2 w-2 rounded-full ${sessionActive ? "bg-success-400" : "bg-warning-400"}`} />
              {sessionActive ? "Sessão ativa" : "Sessão protegida"}
            </div>
          </div>
        </header>

        {sessionPrompt && (
          <form onSubmit={handleAuth} className="border border-warning-200 bg-warning-25 px-5 py-4 shadow-theme-sm dark:border-warning-900 dark:bg-warning-950/30">
            <div className="flex flex-col gap-4 md:flex-row md:items-end">
              <div className="flex-1">
                <p className="text-sm font-semibold text-warning-900 dark:text-warning-200">{sessionExpired ? "A sessão InfoBio expirou" : "Autentique-se no InfoBio"}</p>
                <p className="mt-1 text-xs text-warning-800 dark:text-warning-300">A senha é usada somente para criar a sessão externa e não é armazenada.</p>
                <input aria-label="Senha InfoBio" type="password" value={password} onChange={(event) => setPassword(event.target.value)} placeholder="Senha do InfoBio" className="mt-3 h-11 w-full max-w-md rounded-lg border border-warning-300 bg-white px-3 text-sm outline-none focus:border-warning-500 focus:ring-2 focus:ring-warning-500/20 dark:border-warning-800 dark:bg-gray-900" />
              </div>
              <button disabled={authLoading} className="h-11 rounded-lg bg-gray-900 px-5 text-sm font-semibold text-white hover:bg-gray-800 disabled:opacity-60 dark:bg-white dark:text-gray-900">{authLoading ? "Conectando..." : "Conectar InfoBio"}</button>
            </div>
          </form>
        )}

        <section className="rounded-2xl border border-gray-200 bg-white p-5 shadow-theme-sm dark:border-gray-800 dark:bg-gray-900 sm:p-6">
          <div className="mb-5 flex items-center justify-between gap-4">
            <div><h2 className="text-lg font-semibold text-gray-900 dark:text-white">Nova consulta</h2><p className="mt-1 text-sm text-gray-500 dark:text-gray-400">Pesquise por nome, NIF ou RIN.</p></div>
            {sessionActive && <button type="button" onClick={handleInvalidate} className="text-xs font-semibold text-gray-500 hover:text-error-600 dark:text-gray-400">Encerrar sessão</button>}
          </div>
          <form onSubmit={handleSearch}>
            <div className="mb-5 flex flex-wrap gap-2 border-b border-gray-100 pb-4 dark:border-gray-800">
              {(Object.keys(modeLabels) as SearchMode[]).map((item) => <button key={item} type="button" onClick={() => { setMode(item); setValue(""); setError(""); }} className={`rounded-lg px-4 py-2 text-sm font-semibold transition ${mode === item ? "bg-brand-500 text-white" : "text-gray-500 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-white/5"}`}>{modeLabels[item]}</button>)}
            </div>
            <div className="grid gap-4 md:grid-cols-[1fr_auto]">
              <input value={value} onChange={(event) => setValue(event.target.value)} placeholder={mode === "nome" ? "Nome completo" : `Número do ${modeLabels[mode]}`} className="h-12 rounded-lg border border-gray-300 bg-transparent px-4 text-sm text-gray-900 outline-none focus:border-brand-500 focus:ring-2 focus:ring-brand-500/10 dark:border-gray-700 dark:text-white" />
              <button disabled={loading} className="h-12 rounded-lg bg-brand-500 px-7 text-sm font-semibold text-white shadow-sm hover:bg-brand-600 disabled:opacity-60">{loading ? "Buscando..." : "Buscar registros"}</button>
            </div>
            {mode === "nome" && <div className="mt-4 grid gap-4 md:grid-cols-3"><input value={nomeMae} onChange={(event) => setNomeMae(event.target.value)} placeholder="Nome da mãe (opcional)" className="h-11 rounded-lg border border-gray-300 bg-transparent px-3 text-sm text-gray-900 outline-none focus:border-brand-500 dark:border-gray-700 dark:text-white" /><input value={nomePai} onChange={(event) => setNomePai(event.target.value)} placeholder="Nome do pai (opcional)" className="h-11 rounded-lg border border-gray-300 bg-transparent px-3 text-sm text-gray-900 outline-none focus:border-brand-500 dark:border-gray-700 dark:text-white" /><input type="date" value={dataNascimento} onChange={(event) => setDataNascimento(event.target.value)} className="h-11 rounded-lg border border-gray-300 bg-transparent px-3 text-sm text-gray-900 outline-none focus:border-brand-500 dark:border-gray-700 dark:text-white" /></div>}
          </form>
          {error && <div className="mt-4 border-l-4 border-error-500 bg-error-50 px-4 py-3 text-sm text-error-700 dark:bg-error-950/30 dark:text-error-300">{error}</div>}
        </section>

        <section className="rounded-2xl border border-gray-200 bg-white shadow-theme-sm dark:border-gray-800 dark:bg-gray-900">
          <div className="flex items-center justify-between border-b border-gray-100 px-5 py-4 dark:border-gray-800"><div><h2 className="font-semibold text-gray-900 dark:text-white">Resultados</h2><p className="text-xs text-gray-500 dark:text-gray-400">{resultCount} registro{resultCount === 1 ? "" : "s"} encontrado{resultCount === 1 ? "" : "s"}</p></div></div>
          {results.length === 0 ? <div className="px-5 py-14 text-center text-sm text-gray-500 dark:text-gray-400">Os resultados da sua consulta aparecerão aqui.</div> : <div className="overflow-x-auto"><table className="w-full min-w-[680px] text-left text-sm"><thead className="bg-gray-50 text-xs uppercase text-gray-500 dark:bg-white/5 dark:text-gray-400"><tr><th className="px-5 py-3">NIF</th><th className="px-5 py-3">Nome</th><th className="px-5 py-3">Nascimento</th><th className="px-5 py-3">Mãe</th><th className="px-5 py-3 text-right">Ação</th></tr></thead><tbody className="divide-y divide-gray-100 dark:divide-gray-800">{results.map((entry, index) => <tr key={`${entryValue(entry, "NIF", "nif", "numeroNIF")}-${index}`} className="text-gray-700 dark:text-gray-300"><td className="px-5 py-4 font-mono text-xs">{entryValue(entry, "NIF", "nif", "numeroNIF")}</td><td className="px-5 py-4 font-medium text-gray-900 dark:text-white">{entryValue(entry, "NOME", "nomePessoa", "nome")}</td><td className="px-5 py-4">{entryValue(entry, "DT_NASCIMENTO", "dataNascimento", "data_nascimento", "dt_nascimento")}</td><td className="px-5 py-4">{entryValue(entry, "MAE", "nomeMae", "nome_mae", "mae")}</td><td className="px-5 py-4 text-right"><button type="button" onClick={() => handleDetails(entry)} className="font-semibold text-brand-500 hover:text-brand-700">Ver detalhes</button></td></tr>)}</tbody></table></div>}
        </section>
      </div>

      {selected && <div role="dialog" aria-modal="true" className="fixed inset-0 z-50 flex items-center justify-center bg-gray-950/60 p-4" onClick={() => setSelected(null)}><div className="max-h-[90vh] w-full max-w-3xl overflow-y-auto rounded-2xl bg-white shadow-theme-xl dark:bg-gray-900" onClick={(event) => event.stopPropagation()}><div className="flex items-center justify-between border-b border-gray-200 px-6 py-4 dark:border-gray-800"><div><p className="text-xs uppercase tracking-widest text-brand-500">Registro biométrico</p><h2 className="mt-1 text-xl font-semibold text-gray-900 dark:text-white">NIF {entryValue(selected, "NIF", "nif", "numeroNIF")}</h2></div><button type="button" onClick={() => setSelected(null)} aria-label="Fechar detalhes" className="rounded-lg px-3 py-2 text-xl text-gray-400 hover:bg-gray-100 dark:hover:bg-white/5">×</button></div>{detailLoading ? <div className="p-12 text-center text-sm text-gray-500">Carregando detalhes...</div> : <div className="grid gap-x-8 gap-y-5 p-6 sm:grid-cols-2">{Object.entries(selected).filter(([key, item]) => key !== "sql" && item !== null && item !== "" && item !== 0).map(([key, item]) => <div key={key}><p className="text-xs font-semibold uppercase tracking-wide text-gray-400">{key.replace(/_/g, " ")}</p><p className="mt-1 break-words text-sm text-gray-900 dark:text-gray-200">{typeof item === "object" ? JSON.stringify(item) : String(item)}</p></div>)}</div>}</div></div>}
    </>
  );
}
