import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import PageMeta from "../components/common/PageMeta";
import { getStoredUser, getToken } from "../services/auth";
import {
  askToPersist,
  deleteLocalNote,
  keepTeam,
  keptTeam,
  listLocalNotes,
  newClientId,
  saveLocalNote,
  type LocalAudio,
  type LocalNote,
  type TeamUser,
} from "../offline/fieldNotesStore";
import { discardSceneNote, fetchTeam, listSceneNotes, SignInNeeded, syncAudio, syncNote, type SceneNote } from "../services/sceneNotes";

const PLACE_TYPES = ["RESIDENCE", "COMMERCE", "BANK", "GOVERNMENT_OFFICE", "PUBLIC_ROAD", "TRANSPORT", "RURAL", "OTHER"];

const inputClass =
  "h-12 w-full rounded-lg border border-gray-300 bg-white px-4 text-base text-gray-800 placeholder:text-gray-400 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90";
const labelClass = "mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300";
const buttonClass =
  "h-12 rounded-lg border border-gray-300 px-4 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-white/[0.03]";
const primaryClass = "h-12 rounded-lg bg-brand-500 px-5 text-sm font-medium text-white hover:bg-brand-600 disabled:opacity-50";
const cardClass = "rounded-2xl border border-gray-200 bg-white p-4 dark:border-gray-800 dark:bg-white/[0.03]";

/** A datetime-local value (local time, to the minute) for a date. */
function toLocalInput(d: Date): string {
  const copy = new Date(d);
  copy.setSeconds(0, 0);
  return new Date(copy.getTime() - copy.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
}

const fromLocalInput = (value: string) => (value ? new Date(value).toISOString() : "");
const toInput = (iso: string) => (iso ? toLocalInput(new Date(iso)) : "");

function emptyNote(): LocalNote {
  return {
    clientId: newClientId(),
    placeType: "",
    placeName: "",
    lon: null,
    lat: null,
    address: "",
    neighbourhood: "",
    calledAt: "",
    examinedAt: new Date().toISOString(),
    occurredAt: "",
    team: [],
    otherOfficers: "",
    notes: "",
    writtenAt: new Date().toISOString(),
    audios: [],
    synced: false,
  };
}

function useOnline(): boolean {
  const [online, setOnline] = useState(navigator.onLine);
  useEffect(() => {
    const on = () => setOnline(true);
    const off = () => setOnline(false);
    window.addEventListener("online", on);
    window.addEventListener("offline", off);
    return () => {
      window.removeEventListener("online", on);
      window.removeEventListener("offline", off);
    };
  }, []);
  return online;
}

/** Plays a Blob kept on the device. */
function LocalAudioPlayer({ blob }: { blob: Blob }) {
  const [url, setUrl] = useState("");
  useEffect(() => {
    const created = URL.createObjectURL(blob);
    setUrl(created);
    return () => URL.revokeObjectURL(created);
  }, [blob]);
  return url ? <audio controls src={url} className="h-10 w-full" /> : null;
}

// Notes at a scene (docs/scene-examination-plan.md): written on a phone that may have no signal,
// kept in the browser, and synced when there is one. The page needs no session to write: signing
// in is asked for at sync. A synced note is a draft on the server until it becomes a collection.
export default function FieldNotes() {
  const { t } = useTranslation();
  const online = useOnline();
  const user = getStoredUser();
  const [note, setNote] = useState<LocalNote>(emptyNote);
  const [writing, setWriting] = useState(false);
  const [local, setLocal] = useState<LocalNote[]>([]);
  const [drafts, setDrafts] = useState<SceneNote[]>([]);
  const [team, setTeam] = useState<TeamUser[]>([]);
  const [syncing, setSyncing] = useState(false);
  const [message, setMessage] = useState("");
  const [needSignIn, setNeedSignIn] = useState(false);
  const [locating, setLocating] = useState(false);
  const [recording, setRecording] = useState(false);
  const recorder = useRef<MediaRecorder | null>(null);
  const syncingRef = useRef(false);

  const reloadLocal = useCallback(async () => setLocal(await listLocalNotes()), []);

  const reloadDrafts = useCallback(async () => {
    if (!navigator.onLine || !getToken()) return;
    try {
      setDrafts(await listSceneNotes());
    } catch {
      // Offline after all, or the session ended: the device's own list still shows.
    }
  }, []);

  // Sends what the device holds: each note, then each of its audios. A note wholly on the server
  // leaves the device. Whatever fails stays, with why, for the next try.
  const sync = useCallback(async () => {
    if (syncingRef.current || !navigator.onLine) return;
    syncingRef.current = true;
    setSyncing(true);
    setMessage("");
    let sent = 0;
    try {
      for (const pending of await listLocalNotes()) {
        try {
          let current = pending;
          if (!current.synced || current.serverId === undefined) {
            const kept = await syncNote(current);
            current = { ...current, synced: true, serverId: kept.id, error: undefined };
            await saveLocalNote(current);
          }
          for (const audio of current.audios) {
            if (audio.synced) continue;
            await syncAudio(current.serverId!, audio);
            current = { ...current, audios: current.audios.map((a) => (a.clientId === audio.clientId ? { ...a, synced: true } : a)) };
            await saveLocalNote(current);
          }
          await deleteLocalNote(current.clientId);
          sent++;
        } catch (err) {
          if (err instanceof SignInNeeded) {
            setNeedSignIn(true);
            break;
          }
          await saveLocalNote({ ...pending, error: err instanceof Error ? err.message : String(err) });
        }
      }
      if (sent > 0) setMessage(t("fieldNotes.synced", { count: sent }));
    } finally {
      syncingRef.current = false;
      setSyncing(false);
      await reloadLocal();
      await reloadDrafts();
    }
  }, [reloadDrafts, reloadLocal, t]);

  useEffect(() => {
    void askToPersist();
    void reloadLocal();
    void keptTeam().then(setTeam);
  }, [reloadLocal]);

  // With a signal: refresh the team list for later, and send what is waiting.
  useEffect(() => {
    if (!online) return;
    setNeedSignIn(!getToken());
    if (!getToken()) return;
    fetchTeam()
      .then(async (fetched) => {
        setTeam(fetched);
        await keepTeam(fetched);
      })
      .catch(() => undefined);
    void sync();
  }, [online, sync]);

  const set = (patch: Partial<LocalNote>) => setNote((n) => ({ ...n, ...patch }));

  function locate() {
    if (!("geolocation" in navigator)) {
      setMessage(t("fieldNotes.noPosition"));
      return;
    }
    setLocating(true);
    navigator.geolocation.getCurrentPosition(
      (position) => {
        set({ lon: Number(position.coords.longitude.toFixed(6)), lat: Number(position.coords.latitude.toFixed(6)) });
        setLocating(false);
      },
      () => {
        setMessage(t("fieldNotes.noPosition"));
        setLocating(false);
      },
      { enableHighAccuracy: true, timeout: 20000 },
    );
  }

  async function record() {
    if (recording) {
      recorder.current?.stop();
      return;
    }
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      const chunks: Blob[] = [];
      const started = new Date().toISOString();
      const r = new MediaRecorder(stream);
      r.ondataavailable = (e) => e.data.size > 0 && chunks.push(e.data);
      r.onstop = () => {
        stream.getTracks().forEach((track) => track.stop());
        setRecording(false);
        const blob = new Blob(chunks, { type: r.mimeType || "audio/webm" });
        if (blob.size === 0) return;
        setNote((n) => ({
          ...n,
          audios: [...n.audios, { clientId: newClientId(), title: t("fieldNotes.audioTitle", { n: n.audios.length + 1 }), blob, recordedAt: started, synced: false }],
        }));
      };
      recorder.current = r;
      r.start();
      setRecording(true);
    } catch {
      setMessage(t("fieldNotes.noMicrophone"));
    }
  }

  const setAudio = (clientId: string, patch: Partial<LocalAudio>) =>
    set({ audios: note.audios.map((a) => (a.clientId === clientId ? { ...a, ...patch } : a)) });

  async function save(e: React.FormEvent) {
    e.preventDefault();
    // An edit of a note already sent is sent again: the server replaces it.
    await saveLocalNote({ ...note, writtenAt: new Date().toISOString(), synced: false, error: undefined });
    setNote(emptyNote());
    setWriting(false);
    setMessage(t("fieldNotes.kept"));
    await reloadLocal();
    if (navigator.onLine) void sync();
  }

  async function removeLocal(clientId: string) {
    if (!window.confirm(t("fieldNotes.confirmDiscard"))) return;
    await deleteLocalNote(clientId);
    await reloadLocal();
  }

  async function discardDraft(id: number) {
    if (!window.confirm(t("fieldNotes.confirmDiscard"))) return;
    try {
      await discardSceneNote(id);
      await reloadDrafts();
    } catch (err) {
      setMessage(err instanceof Error ? err.message : String(err));
    }
  }

  const describe = (n: { placeType?: string; placeName?: string; address?: string }) =>
    [n.placeType && t(`placeType.${n.placeType}`), n.placeName, n.address].filter(Boolean).join(" · ") || t("fieldNotes.noPlace");
  const hasContent = note.notes.trim() !== "" || note.audios.length > 0 || note.lon !== null || note.address.trim() !== "" || note.placeName.trim() !== "";

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900">
      <PageMeta title={`${t("fieldNotes.title")} | TrackID`} description={t("fieldNotes.desc")} />
      <header className="sticky top-0 z-10 flex items-center justify-between gap-3 border-b border-gray-200 bg-white px-4 py-3 dark:border-gray-800 dark:bg-gray-900">
        <div className="min-w-0">
          <h1 className="truncate text-lg font-semibold text-gray-800 dark:text-white/90">{t("fieldNotes.title")}</h1>
          <p className="truncate text-xs text-gray-500 dark:text-gray-400">
            {user ? user.username : t("fieldNotes.notSignedIn")}
          </p>
        </div>
        <span role="status"
          className={`shrink-0 rounded-full px-3 py-1 text-xs font-semibold ${online ? "bg-success-50 text-success-700 dark:bg-success-500/15 dark:text-success-400" : "bg-warning-50 text-warning-700 dark:bg-warning-500/15 dark:text-warning-400"}`}>
          {online ? t("fieldNotes.online") : t("fieldNotes.offline")}
        </span>
      </header>

      <main className="mx-auto flex max-w-2xl flex-col gap-4 p-4">
        {message && <p className="text-sm text-gray-600 dark:text-gray-300" role="status">{message}</p>}
        {needSignIn && local.length > 0 && (
          <p className={`${cardClass} text-sm text-warning-700 dark:text-warning-400`}>
            {t("fieldNotes.signInToSync")}{" "}
            <Link to="/signin" state={{ from: { pathname: "/field-notes" } }} className="font-medium text-brand-500 underline">
              {t("fieldNotes.signIn")}
            </Link>
          </p>
        )}

        {!writing && (
          <button type="button" className={primaryClass} onClick={() => { setNote(emptyNote()); setWriting(true); setMessage(""); }}>
            {t("fieldNotes.new")}
          </button>
        )}

        {writing && (
          <form onSubmit={save} className={`${cardClass} flex flex-col gap-4`}>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div>
                <label className={labelClass} htmlFor="fn-type">{t("fieldNotes.placeType")}</label>
                <select id="fn-type" className={inputClass} value={note.placeType} onChange={(e) => set({ placeType: e.target.value })}>
                  <option value="">—</option>
                  {PLACE_TYPES.map((p) => <option key={p} value={p}>{t(`placeType.${p}`)}</option>)}
                </select>
              </div>
              <div>
                <label className={labelClass} htmlFor="fn-name">{t("fieldNotes.placeName")}</label>
                <input id="fn-name" className={inputClass} value={note.placeName} onChange={(e) => set({ placeName: e.target.value })} />
              </div>
            </div>

            <div>
              <span className={labelClass}>{t("fieldNotes.position")}</span>
              <div className="flex flex-wrap items-center gap-3">
                <button type="button" className={buttonClass} disabled={locating} onClick={locate}>
                  {locating ? t("fieldNotes.locating") : t("fieldNotes.useMyPosition")}
                </button>
                <input aria-label={t("fieldNotes.latitude")} placeholder={t("fieldNotes.latitude")} inputMode="decimal" className={`${inputClass} w-36`}
                  value={note.lat ?? ""} onChange={(e) => set({ lat: e.target.value === "" ? null : Number(e.target.value) })} />
                <input aria-label={t("fieldNotes.longitude")} placeholder={t("fieldNotes.longitude")} inputMode="decimal" className={`${inputClass} w-36`}
                  value={note.lon ?? ""} onChange={(e) => set({ lon: e.target.value === "" ? null : Number(e.target.value) })} />
              </div>
              <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">{t("fieldNotes.positionHint")}</p>
            </div>

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div>
                <label className={labelClass} htmlFor="fn-address">{t("fieldNotes.address")}</label>
                <input id="fn-address" className={inputClass} value={note.address} onChange={(e) => set({ address: e.target.value })} />
              </div>
              <div>
                <label className={labelClass} htmlFor="fn-neighbourhood">{t("occurrenceForm.neighbourhood")}</label>
                <input id="fn-neighbourhood" className={inputClass} value={note.neighbourhood} onChange={(e) => set({ neighbourhood: e.target.value })} />
              </div>
            </div>

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
              <div>
                <label className={labelClass} htmlFor="fn-called">{t("fieldNotes.calledAt")}</label>
                <input id="fn-called" type="datetime-local" className={inputClass} value={toInput(note.calledAt)}
                  onChange={(e) => set({ calledAt: fromLocalInput(e.target.value) })} />
              </div>
              <div>
                <label className={labelClass} htmlFor="fn-examined">{t("fieldNotes.examinedAt")}</label>
                <input id="fn-examined" type="datetime-local" className={inputClass} value={toInput(note.examinedAt)}
                  onChange={(e) => set({ examinedAt: fromLocalInput(e.target.value) })} />
              </div>
              <div>
                <label className={labelClass} htmlFor="fn-occurred">{t("fieldNotes.occurredAt")}</label>
                <input id="fn-occurred" type="datetime-local" className={inputClass} value={toInput(note.occurredAt)}
                  onChange={(e) => set({ occurredAt: fromLocalInput(e.target.value) })} />
              </div>
            </div>

            <fieldset>
              <legend className={labelClass}>{t("fieldNotes.team")}</legend>
              {team.length === 0 ? (
                <p className="text-sm text-gray-500 dark:text-gray-400">{t("fieldNotes.noTeamList")}</p>
              ) : (
                <div className="flex flex-wrap gap-x-5 gap-y-2">
                  {team.filter((m) => m.id !== user?.id).map((m) => (
                    <label key={m.id} className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                      <input type="checkbox" className="h-5 w-5" checked={note.team.includes(m.id)}
                        onChange={(e) => set({ team: e.target.checked ? [...note.team, m.id] : note.team.filter((id) => id !== m.id) })} />
                      {m.name || m.username}
                    </label>
                  ))}
                </div>
              )}
            </fieldset>
            <div>
              <label className={labelClass} htmlFor="fn-officers">{t("fieldNotes.otherOfficers")}</label>
              <input id="fn-officers" className={inputClass} value={note.otherOfficers} onChange={(e) => set({ otherOfficers: e.target.value })} />
            </div>

            <div>
              <label className={labelClass} htmlFor="fn-notes">{t("fieldNotes.notes")}</label>
              <textarea id="fn-notes" rows={6} value={note.notes} onChange={(e) => set({ notes: e.target.value })}
                className="w-full rounded-lg border border-gray-300 bg-white px-4 py-3 text-base text-gray-800 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90" />
            </div>

            <div className="flex flex-col gap-3">
              <span className={labelClass}>{t("fieldNotes.audio")}</span>
              {note.audios.map((a) => (
                <div key={a.clientId} className="flex flex-col gap-2 rounded-lg border border-gray-200 p-3 dark:border-gray-800">
                  <div className="flex items-center gap-2">
                    <input aria-label={t("fieldNotes.audioName")} className={`${inputClass} h-10`} value={a.title} onChange={(e) => setAudio(a.clientId, { title: e.target.value })} />
                    <button type="button" className="shrink-0 text-sm text-error-500 hover:underline"
                      onClick={() => set({ audios: note.audios.filter((x) => x.clientId !== a.clientId) })}>
                      {t("common.remove")}
                    </button>
                  </div>
                  <LocalAudioPlayer blob={a.blob} />
                </div>
              ))}
              <button type="button" className={`${buttonClass} ${recording ? "border-error-500 text-error-600 dark:text-error-400" : ""}`} onClick={() => void record()}>
                {recording ? t("fieldNotes.stopRecording") : t("fieldNotes.record")}
              </button>
            </div>

            <div className="flex items-center justify-end gap-3">
              <button type="button" className={buttonClass} onClick={() => { recorder.current?.stop(); setWriting(false); }}>
                {t("common.cancel")}
              </button>
              <button type="submit" className={primaryClass} disabled={!hasContent || recording}>
                {t("fieldNotes.keep")}
              </button>
            </div>
          </form>
        )}

        <section className={`${cardClass} flex flex-col gap-3`}>
          <div className="flex items-center justify-between gap-3">
            <h2 className="text-base font-semibold text-gray-800 dark:text-white/90">{t("fieldNotes.onDevice", { count: local.length })}</h2>
            <button type="button" className={buttonClass} disabled={!online || syncing || local.length === 0} onClick={() => void sync()}>
              {syncing ? t("fieldNotes.syncing") : t("fieldNotes.sync")}
            </button>
          </div>
          {local.length === 0 && <p className="text-sm text-gray-500 dark:text-gray-400">{t("fieldNotes.nothingWaiting")}</p>}
          <ul className="flex flex-col divide-y divide-gray-100 dark:divide-white/[0.05]">
            {local.map((n) => (
              <li key={n.clientId} className="flex flex-col gap-1 py-3">
                <span className="text-sm font-medium text-gray-800 dark:text-white/90">{describe(n)}</span>
                <span className="text-xs text-gray-500 dark:text-gray-400">
                  {new Date(n.writtenAt).toLocaleString()}
                  {n.audios.length > 0 && ` · ${t("fieldNotes.audios", { count: n.audios.length })}`}
                </span>
                {n.notes && <span className="line-clamp-2 text-sm text-gray-600 dark:text-gray-300">{n.notes}</span>}
                {n.error && <span className="text-xs text-error-500">{t("fieldNotes.syncFailed", { error: n.error })}</span>}
                <span className="flex gap-4 text-sm">
                  <button type="button" className="text-brand-500 hover:underline" onClick={() => { setNote(n); setWriting(true); window.scrollTo({ top: 0 }); }}>
                    {t("fieldNotes.edit")}
                  </button>
                  <button type="button" className="text-error-500 hover:underline" onClick={() => void removeLocal(n.clientId)}>
                    {t("fieldNotes.discard")}
                  </button>
                </span>
              </li>
            ))}
          </ul>
        </section>

        {drafts.length > 0 && (
          <section className={`${cardClass} flex flex-col gap-3`}>
            <h2 className="text-base font-semibold text-gray-800 dark:text-white/90">{t("fieldNotes.drafts", { count: drafts.length })}</h2>
            <p className="text-xs text-gray-500 dark:text-gray-400">{t("fieldNotes.draftsHint")}</p>
            <ul className="flex flex-col divide-y divide-gray-100 dark:divide-white/[0.05]">
              {drafts.map((d) => (
                <li key={d.id} className="flex flex-col gap-1 py-3">
                  <span className="text-sm font-medium text-gray-800 dark:text-white/90">{describe(d)}</span>
                  <span className="text-xs text-gray-500 dark:text-gray-400">
                    {new Date(d.writtenAt).toLocaleString()}
                    {d.audioCount > 0 && ` · ${t("fieldNotes.audios", { count: d.audioCount })}`}
                  </span>
                  {d.notes && <span className="line-clamp-2 text-sm text-gray-600 dark:text-gray-300">{d.notes}</span>}
                  <span className="flex gap-4 text-sm">
                    <Link to={`/collections/new?note=${d.id}`} className="text-brand-500 hover:underline">{t("fieldNotes.register")}</Link>
                    <button type="button" className="text-error-500 hover:underline" onClick={() => void discardDraft(d.id)}>
                      {t("fieldNotes.discard")}
                    </button>
                  </span>
                </li>
              ))}
            </ul>
          </section>
        )}

        <Link to="/" className="self-center text-sm text-brand-500 hover:underline">{t("fieldNotes.backToApp")}</Link>
      </main>
    </div>
  );
}
