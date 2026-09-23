import {
  useEffect,
  useRef,
  useState,
  type ChangeEvent,
  type KeyboardEvent,
} from "react";
import { useTranslation } from "react-i18next";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowUp,
  AtSign,
  Cable,
  FileText,
  Paperclip,
  Server,
  Square,
  X,
} from "lucide-react";
import { useApi, formatApiError, type WorkspaceFile } from "@argus/api-client";
import { Button, Tooltip } from "@argus/ui";

type Mention = { kind: "host" | "connector"; id: string; label: string };
export function ChatComposer({
  sending,
  disabled,
  onSend,
  onStop,
  conversationId,
  prepareConversation,
}: {
  sending: boolean;
  disabled?: boolean;
  conversationId?: string;
  onSend(text: string, fileIds?: string[]): Promise<boolean>;
  onStop(): void;
  prepareConversation(): Promise<string>;
}) {
  const { t } = useTranslation();
  const api = useApi();
  const queries = useQueryClient();
  const [text, setText] = useState("");
  const [mentions, setMentions] = useState<Mention[]>([]);
  const [files, setFiles] = useState<WorkspaceFile[]>([]);
  const [picker, setPicker] = useState<{
    start: number;
    end: number;
    query: string;
  } | null>(null);
  const [upload, setUpload] = useState<{
    name: string;
    percent: number;
  } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const area = useRef<HTMLTextAreaElement>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const activeUpload = useRef<AbortController | null>(null);
  const previousConversation = useRef(conversationId);
  useEffect(() => {
    if (
      previousConversation.current &&
      previousConversation.current !== conversationId
    ) {
      activeUpload.current?.abort();
      setFiles([]);
      setText("");
      setMentions([]);
    }
    previousConversation.current = conversationId;
  }, [conversationId]);
  useEffect(() => () => activeUpload.current?.abort(), []);
  const hosts = useQuery({
    queryKey: ["hosts", "picker"],
    queryFn: () => api.hosts.list(),
    enabled: !!picker,
  });
  const connectors = useQuery({
    queryKey: ["connectors", "picker"],
    queryFn: () => api.connectors.list(),
    enabled: !!picker,
  });
  const items: Mention[] = [
    ...(hosts.data?.items ?? []).map((item) => ({
      kind: "host" as const,
      id: item.id,
      label: item.name,
    })),
    ...(connectors.data?.items ?? []).map((item) => ({
      kind: "connector" as const,
      id: item.id,
      label: item.name,
    })),
  ].filter((item) =>
    item.label.toLowerCase().includes(picker?.query.toLowerCase() ?? ""),
  );
  const updateText = (value: string, caret: number) => {
    setText(value);
    const match = /(?:^|\s)@([^\s@]*)$/.exec(value.slice(0, caret));
    setPicker(
      match
        ? { start: caret - match[1]!.length - 1, end: caret, query: match[1]! }
        : null,
    );
  };
  const choose = (item: Mention) => {
    if (!picker) return;
    setMentions((current) =>
      current.some((x) => x.id === item.id) ? current : [...current, item],
    );
    setText(text.slice(0, picker.start) + text.slice(picker.end));
    setPicker(null);
    area.current?.focus();
  };
  const submit = async () => {
    if (!text.trim() || sending || disabled || submitting || upload) return;
    setSubmitting(true);
    setError(null);
    const value = [
      text.trim(),
      ...mentions.map((item) => `@${item.label} (${item.kind}:${item.id})`),
    ].join("\n");
    try {
      if (
        await onSend(
          value,
          files.map((file) => file.id),
        )
      ) {
        setText("");
        setMentions([]);
        setFiles([]);
        setPicker(null);
      }
    } catch (error) {
      setError(
        formatApiError(error, t("planv5.sendFailed"), (requestId) =>
          t("common.requestReference", { requestId }),
        ),
      );
    } finally {
      setSubmitting(false);
    }
  };
  const keyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === "Escape") setPicker(null);
    if (
      event.key === "Enter" &&
      !event.shiftKey &&
      !event.nativeEvent.isComposing
    ) {
      event.preventDefault();
      if (picker && items[0]) choose(items[0]);
      else void submit();
    }
  };
  const pickFiles = async (event: ChangeEvent<HTMLInputElement>) => {
    const selected = Array.from(event.target.files ?? []);
    event.target.value = "";
    if (!selected.length) return;
    const controller = new AbortController();
    activeUpload.current = controller;
    setError(null);
    try {
      const id = await prepareConversation();
      for (const file of selected) {
        setUpload({ name: file.name, percent: 0 });
        const stored = await api.workspace.upload(
          id,
          file,
          (percent) => setUpload({ name: file.name, percent }),
          controller.signal,
        );
        setFiles((current) => [...current, stored]);
      }
      await queries.invalidateQueries({ queryKey: ["workspace-files", id] });
      await queries.invalidateQueries({ queryKey: ["workspace", id] });
    } catch (error) {
      if (!controller.signal.aborted)
        setError(
          formatApiError(error, t("planv5.files.failed"), (requestId) =>
            t("common.requestReference", { requestId }),
          ),
        );
    } finally {
      setUpload(null);
      activeUpload.current = null;
    }
  };
  return (
    <div className="argus-chat-composer">
      <div className="argus-chat-composer__inner">
        <div className="argus-chat-composer__chips">
          {mentions.map((item) => (
            <span className="argus-chat-chip" key={item.id}>
              {item.kind === "host" ? (
                <Server size={12} />
              ) : (
                <Cable size={12} />
              )}{" "}
              {item.label}
              <button
                type="button"
                aria-label={t("planv5.files.remove")}
                onClick={() =>
                  setMentions((current) =>
                    current.filter((x) => x.id !== item.id),
                  )
                }
              >
                <X size={11} />
              </button>
            </span>
          ))}
          {files.map((file) => (
            <span className="argus-chat-chip" key={file.id}>
              <FileText size={12} />
              {file.name}
              <button
                type="button"
                aria-label={t("planv5.files.remove")}
                disabled={sending}
                onClick={() =>
                  setFiles((current) => current.filter((x) => x.id !== file.id))
                }
              >
                <X size={11} />
              </button>
            </span>
          ))}
          {upload && (
            <span role="status">
              {upload.name} ·{" "}
              {t("planv5.files.uploading", { percent: upload.percent })}
              <Button
                variant="ghost"
                size="sm"
                onClick={() => activeUpload.current?.abort()}
              >
                {t("planv5.files.cancel")}
              </Button>
            </span>
          )}
        </div>
        {error && <p role="alert">{error}</p>}
        <div className="argus-chat-composer__box">
          {picker && (
            <div
              className="argus-chat-picker"
              role="listbox"
              aria-label={t("chat.composer.mentionTitle")}
            >
              <div className="argus-chat-picker__list">
                {items.length ? (
                  items.map((item) => (
                    <button
                      className="argus-chat-picker__item"
                      role="option"
                      aria-selected="false"
                      type="button"
                      key={item.id}
                      onClick={() => choose(item)}
                    >
                      {item.label}
                    </button>
                  ))
                ) : (
                  <p>{t("chat.composer.noMatch")}</p>
                )}
              </div>
            </div>
          )}
          <textarea
            ref={area}
            aria-label={t("chat.composer.send")}
            value={text}
            disabled={disabled || sending}
            rows={2}
            placeholder={t(
              disabled ? "chat.composer.noModel" : "chat.composer.placeholder",
            )}
            onKeyDown={keyDown}
            onChange={(event) =>
              updateText(event.target.value, event.target.selectionStart)
            }
          />
          <div className="argus-chat-composer__toolbar">
            <input
              hidden
              type="file"
              disabled={(!conversationId && disabled) || sending || !!upload}
              multiple
              ref={fileInput}
              onChange={(event) => void pickFiles(event)}
            />
            <Tooltip content={t("planv5.files.attach")}>
              <Button
                aria-label={t("planv5.files.attach")}
                variant="ghost"
                size="icon"
                disabled={(!conversationId && disabled) || sending || !!upload}
                onClick={() => fileInput.current?.click()}
              >
                <Paperclip size={15} />
              </Button>
            </Tooltip>
            <Tooltip content={t("chat.composer.mention")}>
              <Button
                aria-label={t("chat.composer.mention")}
                variant="ghost"
                size="icon"
                disabled={disabled || sending}
                onClick={() => {
                  updateText(text + "@", text.length + 1);
                  area.current?.focus();
                }}
              >
                <AtSign size={15} />
              </Button>
            </Tooltip>
            <span>{t("chat.composer.hint")}</span>
            {sending ? (
              <Button
                aria-label={t("chat.composer.stop")}
                onClick={onStop}
                size="icon"
                variant="secondary"
              >
                <Square size={13} />
              </Button>
            ) : (
              <Button
                aria-label={t("chat.composer.send")}
                onClick={() => void submit()}
                disabled={disabled || !text.trim() || submitting || !!upload}
                size="icon"
              >
                <ArrowUp size={16} />
              </Button>
            )}
          </div>
        </div>
        <small className="argus-chat-composer__note">
          {t("chat.composer.disclaimer")}
        </small>
      </div>
    </div>
  );
}
