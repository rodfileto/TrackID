import { useEffect, useRef, useState } from "react";
import { Link } from "react-router";
import { useTranslation } from "react-i18next";

import PageMeta from "../components/common/PageMeta";
import PageBreadcrumb from "../components/common/PageBreadCrumb";
import Badge from "../components/ui/badge/Badge";
import {
  askQuestion,
  AskError,
  getAskInfo,
  type AskAnswer,
  type AskCaseDetail,
  type AskCluster,
  type AskEntity,
  type AskIdentityConflicts,
  type AskInfo,
  type AskInquiry,
  type AskNear,
  type AskOccurrence,
  type AskOccurrenceSearch,
  type AskPersonCases,
  type AskPersonClusters,
  type AskPersonIdentities,
  type AskPersonSearch,
  type AskStep,
  type AskTurn,
  type BreakdownRow,
  type ListedOccurrence,
} from "../services/ask";
import {
  CaseDetailResult,
  ClusterResult,
  IdentityConflictsResult,
  InquiryResult,
  NearResult,
  OccurrenceResult,
  OccurrenceSearchResult,
  PersonCasesResult,
  PersonClustersResult,
  PersonIdentitiesResult,
  PersonSearchResult,
} from "../components/intelligence/AskResults";

// The server takes at most 20 earlier turns and 300 entities. One list of conflicts can hold over 100,
// and "the first of those" must still resolve.
const MAX_HISTORY = 20;
const MAX_FOCUS = 300;

/** What the conversation has found, each once, latest last. */
function conversationFocus(exchanges: { answer?: AskAnswer }[]): AskEntity[] {
  const byKey = new Map<string, AskEntity>();
  for (const exchange of exchanges) {
    for (const entity of exchange.answer?.entities ?? []) {
      const key = `${entity.kind}:${entity.ref}`;
      byKey.delete(key);
      byKey.set(key, entity);
    }
  }
  return [...byKey.values()].slice(-MAX_FOCUS);
}

/** The walk tools' results; the counts and the rest have their own rendering below. */
function WalkResult({ step }: { step: AskStep }) {
  const result = step.result as unknown;
  switch (step.tool) {
    case "search_persons":
      return <PersonSearchResult result={result as AskPersonSearch} />;
    case "person_cases":
      return <PersonCasesResult result={result as AskPersonCases} />;
    case "identity_conflicts":
      return <IdentityConflictsResult result={result as AskIdentityConflicts} />;
    case "cluster_detail":
      return <ClusterResult result={result as AskCluster} />;
    case "person_clusters":
      return <PersonClustersResult result={result as AskPersonClusters} />;
    case "person_identities":
      return <PersonIdentitiesResult result={result as AskPersonIdentities} />;
    case "case_detail":
      return <CaseDetailResult result={result as AskCaseDetail} />;
    case "find_occurrence":
      return <OccurrenceSearchResult result={result as AskOccurrenceSearch} />;
    case "occurrence_detail":
      return <OccurrenceResult occurrence={(result as { occurrence: AskOccurrence }).occurrence} />;
    case "inquiry_detail":
      return <InquiryResult result={result as AskInquiry} />;
    case "occurrences_near":
      return <NearResult result={result as AskNear} />;
    default:
      return null;
  }
}

const WALK_TOOLS = new Set(["identity_conflicts", "cluster_detail", "person_clusters", "search_persons", "person_cases", "person_identities", "case_detail", "find_occurrence", "occurrence_detail", "inquiry_detail", "occurrences_near"]);

const primaryClass =
  "rounded-lg bg-brand-500 px-4 py-2 text-sm font-medium text-white hover:bg-brand-600 disabled:opacity-50";
const buttonClass =
  "rounded-lg border border-gray-300 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-white/5";

type Exchange = { question: string; answer?: AskAnswer; error?: string };

/** The translation prefix for a breakdown's keys; dimensions absent here show their values as they are. */
const KEY_LABELS: Record<string, string> = {
  type: "occurrenceType",
  group: "occurrenceGroup",
  status: "occurrenceStatus",
  place_type: "placeType",
};

function BreakdownTable({ rows, groupBy }: { rows: BreakdownRow[]; groupBy: string }) {
  const { t, i18n } = useTranslation();
  const prefix = KEY_LABELS[groupBy];
  return (
    <div className="max-h-72 overflow-auto rounded-lg border border-gray-200 dark:border-gray-800">
      <table className="w-full text-sm">
        <thead className="sticky top-0 bg-gray-50 text-left text-xs text-gray-500 dark:bg-gray-900 dark:text-gray-400">
          <tr>
            <th className="px-3 py-1.5 font-medium">{t(`ask.dimension.${groupBy}`, groupBy)}</th>
            <th className="px-3 py-1.5 text-right font-medium">{t("ask.count")}</th>
            <th className="px-3 py-1.5 text-right font-medium">%</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.key} className="border-t border-gray-100 text-gray-700 dark:border-gray-800 dark:text-gray-300">
              <td className="px-3 py-1.5">
                {row.key === "" ? t("ask.notRecorded") : prefix ? t(`${prefix}.${row.key}`, row.key) : row.key}
              </td>
              <td className="px-3 py-1.5 text-right tabular-nums">{row.count.toLocaleString(i18n.language)}</td>
              <td className="px-3 py-1.5 text-right tabular-nums">{row.sharePercent.toLocaleString(i18n.language)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function OccurrenceTable({ rows }: { rows: ListedOccurrence[] }) {
  const { t } = useTranslation();
  return (
    <div className="max-h-96 overflow-auto rounded-lg border border-gray-200 dark:border-gray-800">
      <table className="w-full text-sm">
        <thead className="sticky top-0 bg-gray-50 text-left text-xs text-gray-500 dark:bg-gray-900 dark:text-gray-400">
          <tr>
            <th className="px-3 py-1.5 font-medium">{t("ask.occurrence")}</th>
            <th className="px-3 py-1.5 font-medium">{t("ask.date")}</th>
            <th className="px-3 py-1.5 font-medium">{t("ask.types")}</th>
            <th className="px-3 py-1.5 font-medium">{t("ask.place")}</th>
            <th className="px-3 py-1.5 font-medium">{t("ask.cases")}</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row, i) => (
            <tr key={i} className="border-t border-gray-100 align-top text-gray-700 dark:border-gray-800 dark:text-gray-300">
              <td className="px-3 py-1.5 whitespace-nowrap">
                {row.trackidNumber ?? [row.agency, row.number].filter(Boolean).join(" ")}
                {!row.trackidNumber && !row.number && row.inquiryNumber}
              </td>
              <td className="px-3 py-1.5 whitespace-nowrap">{row.occurredOn ?? t("ask.notRecorded")}</td>
              <td className="px-3 py-1.5">
                {row.types.map((c) => t(`occurrenceType.${c}`, c)).join(" · ")}
                {row.attempted && ` (${t("ask.attempted")})`}
              </td>
              <td className="px-3 py-1.5">{[row.placeName, row.neighbourhood, row.uf].filter(Boolean).join(" · ")}</td>
              <td className="px-3 py-1.5 whitespace-nowrap">
                {row.cases.map((c) => (
                  <Link key={c} to={`/cases/${encodeURIComponent(c)}`} className="mr-2 text-brand-500 hover:underline">
                    {c}
                  </Link>
                ))}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/** One query the model ran: its filters and what the database returned, not the model's reading of it. */
function StepCard({ step }: { step: AskStep }) {
  const { t, i18n } = useTranslation();
  const result = step.result ?? {};
  const rows = result.rows as unknown[] | undefined;
  const isCount = step.tool === "count_occurrences" || step.tool === "count_cases";
  const undated = Number(result.undatedExcluded ?? 0);
  const unplaced = Number(result.unplacedExcluded ?? 0);
  return (
    <div className="flex flex-col gap-2 rounded-lg bg-gray-50 p-3 dark:bg-white/[0.03]">
      <div className="flex flex-wrap items-baseline gap-2">
        <span className="text-sm font-medium text-gray-700 dark:text-gray-300">{t(`ask.tool.${step.tool}`, step.tool)}</span>
        <code className="text-xs break-all text-gray-500 dark:text-gray-400">{JSON.stringify(step.arguments)}</code>
      </div>
      {step.error && <p className="text-xs text-error-600 dark:text-error-400">{step.error}</p>}
      {isCount && !step.error && (
        <p className="text-sm text-gray-700 dark:text-gray-300">
          {t("ask.total")}: <span className="font-semibold tabular-nums">{Number(result.total ?? 0).toLocaleString(i18n.language)}</span>
        </p>
      )}
      {undated > 0 && <p className="text-xs text-warning-600 dark:text-warning-400">{t("ask.undatedExcluded", { count: undated })}</p>}
      {unplaced > 0 && <p className="text-xs text-warning-600 dark:text-warning-400">{t("ask.unplacedExcluded", { count: unplaced })}</p>}
      {Boolean(result.truncated) && <p className="text-xs text-warning-600 dark:text-warning-400">{t("ask.truncated")}</p>}
      {isCount && rows && rows.length > 0 && <BreakdownTable rows={rows as BreakdownRow[]} groupBy={String(result.groupBy)} />}
      {step.tool === "list_occurrences" && rows && rows.length > 0 && <OccurrenceTable rows={rows as ListedOccurrence[]} />}
      {WALK_TOOLS.has(step.tool) && !step.error && <WalkResult step={step} />}
      {!isCount && step.tool !== "list_occurrences" && !WALK_TOOLS.has(step.tool) && !step.error && (
        <pre className="overflow-auto font-mono text-xs text-gray-600 dark:text-gray-400">{JSON.stringify(step.result, null, 2)}</pre>
      )}
    </div>
  );
}

function AnswerCard({ answer }: { answer: AskAnswer }) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-3">
      <p className="text-sm whitespace-pre-wrap text-gray-800 dark:text-white/90">{answer.answer}</p>
      {answer.unverifiedNumbers.length > 0 && (
        <p className="rounded-lg border border-warning-500/40 bg-warning-50 p-2 text-xs text-warning-700 dark:bg-warning-500/10 dark:text-warning-400">
          {t("ask.unverified", { numbers: answer.unverifiedNumbers.join(", ") })}
        </p>
      )}
      {answer.steps.length === 0 ? (
        <p className="text-xs text-gray-500 dark:text-gray-400">{t("ask.noQueries")}</p>
      ) : (
        <details open={answer.steps.length <= 3} className="group">
          <summary className="cursor-pointer text-xs font-medium text-gray-500 dark:text-gray-400">
            {t("ask.howComputed", { count: answer.steps.length })}
          </summary>
          <div className="mt-2 flex flex-col gap-2">
            {answer.steps.map((step, i) => (
              <StepCard key={i} step={step} />
            ))}
          </div>
        </details>
      )}
      <p className="font-mono text-xs text-gray-400 dark:text-gray-500">
        {t("ask.footer", {
          model: answer.model,
          seconds: (answer.durationMs / 1000).toFixed(1),
          input: answer.usage.inputTokens,
          output: answer.usage.outputTokens,
          id: answer.id,
        })}
      </p>
    </div>
  );
}

export default function AskAI() {
  const { t } = useTranslation();
  const [info, setInfo] = useState<AskInfo | null>(null);
  const [infoError, setInfoError] = useState(false);
  const [exchanges, setExchanges] = useState<Exchange[]>([]);
  const [question, setQuestion] = useState("");
  const [asking, setAsking] = useState(false);
  const [elapsed, setElapsed] = useState(0);
  const abort = useRef<AbortController | null>(null);
  const end = useRef<HTMLDivElement>(null);

  useEffect(() => {
    getAskInfo().then(setInfo).catch(() => setInfoError(true));
  }, []);

  // A local model can take a minute or more: show the seconds passing while it works.
  useEffect(() => {
    if (!asking) return;
    const start = Date.now();
    setElapsed(0);
    const id = setInterval(() => setElapsed(Math.floor((Date.now() - start) / 1000)), 1000);
    return () => clearInterval(id);
  }, [asking]);

  useEffect(() => {
    end.current?.scrollIntoView({ behavior: "smooth", block: "nearest" });
  }, [exchanges, asking]);

  useEffect(() => () => abort.current?.abort(), []);

  function errorMessage(err: unknown): string {
    if (err instanceof AskError && err.code) return t(`ask.error.${err.code}`, err.message);
    return err instanceof Error ? err.message : String(err);
  }

  async function ask(text: string) {
    const trimmed = text.trim();
    if (!trimmed || asking) return;
    // Only answered exchanges are context: a failed question has nothing for the model to build on.
    const history: AskTurn[] = exchanges
      .filter((e) => e.answer)
      .flatMap((e): AskTurn[] => [
        { role: "user", text: e.question },
        { role: "assistant", text: e.answer!.answer },
      ])
      .slice(-MAX_HISTORY);
    setExchanges((list) => [...list, { question: trimmed }]);
    setQuestion("");
    setAsking(true);
    const controller = new AbortController();
    abort.current = controller;
    try {
      const answer = await askQuestion(trimmed, history, conversationFocus(exchanges), controller.signal);
      setExchanges((list) => list.map((e, i) => (i === list.length - 1 ? { ...e, answer } : e)));
    } catch (err) {
      const message = controller.signal.aborted ? t("ask.cancelled") : errorMessage(err);
      setExchanges((list) => list.map((e, i) => (i === list.length - 1 ? { ...e, error: message } : e)));
      // The question isn't lost to a failure after minutes of waiting.
      setQuestion((current) => current || trimmed);
    } finally {
      setAsking(false);
    }
  }

  const examples = [t("ask.example1"), t("ask.example2"), t("ask.example3"), t("ask.example4"), t("ask.example5")];
  const enabled = info?.enabled ?? false;

  return (
    <>
      <PageMeta title={`${t("ask.title")} | TrackID`} description={t("ask.desc")} />
      <PageBreadcrumb pageTitle={t("ask.title")} />
      <div className="mx-auto flex max-w-4xl flex-col gap-4">
        <p className="text-sm text-gray-500 dark:text-gray-400">{t("ask.desc")}</p>

        {info && !enabled && (
          <div>
            <Badge size="sm" color="warning">
              {t("ask.disabled")}
            </Badge>
          </div>
        )}
        {infoError && <p className="text-sm text-error-600 dark:text-error-400">{t("ask.infoFailed")}</p>}
        {enabled && info?.external && (
          <p className="rounded-lg border border-warning-500/40 bg-warning-50 p-3 text-sm text-warning-700 dark:bg-warning-500/10 dark:text-warning-400">
            {t("ask.externalWarning", { host: info.host })}
          </p>
        )}
        {enabled && !info?.external && (
          <p className="text-xs text-gray-500 dark:text-gray-400">{t("ask.localModel", { model: info?.model, host: info?.host })}</p>
        )}

        {exchanges.length === 0 && enabled && (
          <div className="flex flex-wrap gap-2">
            {examples.map((example) => (
              <button key={example} type="button" className={buttonClass} onClick={() => void ask(example)}>
                {example}
              </button>
            ))}
          </div>
        )}

        <div className="flex flex-col gap-4" aria-live="polite">
          {exchanges.map((exchange, i) => (
            <div key={i} className="flex flex-col gap-2">
              <div className="self-end rounded-2xl bg-brand-500 px-4 py-2 text-sm whitespace-pre-wrap text-white">{exchange.question}</div>
              <div className="rounded-2xl border border-gray-200 bg-white p-4 dark:border-gray-800 dark:bg-white/[0.03]">
                {exchange.answer && <AnswerCard answer={exchange.answer} />}
                {exchange.error && <p className="text-sm break-words text-error-600 dark:text-error-400">{exchange.error}</p>}
                {!exchange.answer && !exchange.error && (
                  <div className="flex items-center gap-3 text-sm text-gray-500 dark:text-gray-400">
                    <span>{t("ask.working", { seconds: elapsed })}</span>
                    <button type="button" className="underline" onClick={() => abort.current?.abort()}>
                      {t("ask.cancel")}
                    </button>
                  </div>
                )}
              </div>
            </div>
          ))}
          <div ref={end} />
        </div>

        <form
          className="flex flex-col gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            void ask(question);
          }}
        >
          <label htmlFor="ask-question" className="sr-only">
            {t("ask.question")}
          </label>
          <textarea
            id="ask-question"
            value={question}
            onChange={(e) => setQuestion(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                void ask(question);
              }
            }}
            rows={2}
            maxLength={4000}
            disabled={!enabled}
            placeholder={t("ask.placeholder")}
            className="w-full rounded-lg border border-gray-300 bg-transparent p-3 text-sm text-gray-800 focus:border-brand-300 focus:ring-3 focus:ring-brand-500/10 focus:outline-hidden disabled:opacity-50 dark:border-gray-700 dark:text-white/90"
          />
          <div className="flex items-center justify-between gap-2">
            <span className="text-xs text-gray-500 dark:text-gray-400">{t("ask.notice")}</span>
            <div className="flex shrink-0 gap-2">
              {exchanges.length > 0 && (
                <button type="button" className={buttonClass} disabled={asking} onClick={() => setExchanges([])}>
                  {t("ask.newConversation")}
                </button>
              )}
              <button type="submit" className={primaryClass} disabled={!enabled || asking || question.trim() === ""}>
                {t("ask.send")}
              </button>
            </div>
          </div>
        </form>
      </div>
    </>
  );
}
