import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { collectionNoteUrl } from "../../services/sceneNotes";

interface Props {
  caseId: string;
  fileId: number;
  filename: string;
}

// An audio the examiner recorded at the scene, played from the case. The file is fetched when
// asked for, since it needs the session's token and may be long.
export default function CollectionAudioNote({ caseId, fileId, filename }: Props) {
  const { t } = useTranslation();
  const [url, setUrl] = useState("");
  const [failed, setFailed] = useState(false);

  useEffect(() => () => {
    if (url) URL.revokeObjectURL(url);
  }, [url]);

  if (url) {
    return (
      <span className="flex flex-col gap-1">
        <span className="text-theme-xs text-gray-500 dark:text-gray-400">{filename}</span>
        <audio controls autoPlay src={url} className="h-9 w-full max-w-md" />
      </span>
    );
  }
  return (
    <button type="button" className="self-start text-sm text-brand-500 hover:underline"
      onClick={() => collectionNoteUrl(caseId, fileId).then(setUrl).catch(() => setFailed(true))}>
      {failed ? t("collections.audioError") : t("collections.playAudio", { name: filename })}
    </button>
  );
}
