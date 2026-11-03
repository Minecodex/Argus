import { Textarea } from "@argus/ui";
import {
  useEffect,
  useRef,
  useState,
  type ChangeEvent,
  type KeyboardEvent,
} from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  ArrowUp,
  Plus,
  Lightbulb,
  AtSign,
  Cable,
  FileText,
  LayoutDashboard,
  Paperclip,
  Server,
  Square,
  X,
} from "lucide-react";
import {
  useApi,
  formatApiError,
  type WorkspaceFile,
  type ConversationInteraction,
  type InteractionResponse,
} from "@argus/api-client";
import { Button, ComposerActions } from "@argus/ui";
import { usePermission } from "../../lib/permissions";
import { mentionQuery } from "./mention-query";
import { ChatAccessSwitch, useConversationModes } from "./mode-switch";
import { ComposerInteraction } from "./composer-interaction";
import {
  useResourceMentions,
  type ResourceMention,
} from "./use-resource-mentions";
import { useControlResourceMentions } from "./use-control-resource-mentions";

type Mention = ResourceMention;
export function ChatComposer({
  sending,
  disabled,
  onSend,
  onStop,
  conversationId,
  prepareConversation,
  interaction,
  onRespond,
}: {
  interaction?: ConversationInteraction;
  onRespond?: (id: string, value: InteractionResponse) => Promise<void>;
  sending: boolean;
  disabled?: boolean;
  conversationId?: string;
  onSend(
    text: string,
    fileIds?: string[],
    context?: import("@argus/api-client").DashboardChatSelection,
  ): Promise<boolean>;
  onStop(): void;
  prepareConversation(): Promise<string>;
}) {
  const { t } = useTranslation();
  const api = useApi();
  const modes = useConversationModes(conversationId, prepareConversation);
  const [collapsedInteraction, setCollapsedInteraction] = useState<
    string | null
  >(null);
  const showInteraction =
    !!interaction && !!onRespond && collapsedInteraction !== interaction.id;
  const canCreateDashboard = usePermission("telemetry.dashboard.manage");
  const queries = useQueryClient();
  const [text, setText] = useState("");
  const [mentions, setMentions] = useState<Mention[]>([]);
  const [activeMention, setActiveMention] = useState(0);
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
  const [focusRequest, setFocusRequest] = useState(0);
  const area = useRef<HTMLTextAreaElement>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const activeUpload = useRef<AbortController | null>(null);
  const previousConversation = useRef(conversationId);
  useEffect(() => {
    if (!focusRequest) return;
    const frame = requestAnimationFrame(() => area.current?.focus());
    return () => cancelAnimationFrame(frame);
  }, [focusRequest]);
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
  const resources = useResourceMentions(picker?.query ?? "", !!picker);
  const controls = useControlResourceMentions(!!picker);
  const { dashboards } = controls;
  const items: Mention[] = [
    ...controls.items.filter((item) => item.kind === "dashboard"),
    ...resources.items,
    ...controls.items.filter((item) => item.kind === "connector"),
  ].filter(
    (item) =>
      (item.kind !== "dashboard" && item.kind !== "connector") ||
      [item.label, item.id].some((value) =>
        value.toLowerCase().includes(picker?.query.toLowerCase() ?? ""),
      ),
  );
  const updateText = (value: string, caret: number) => {
    setActiveMention(0);
    setText(value);
    setPicker(mentionQuery(value, caret));
  };
  const choose = (item: Mention) => {
    if (!picker) return;
    if (
      mentions.length >= 20 &&
      !mentions.some((ref) => ref.kind === item.kind && ref.id === item.id)
    ) {
      setError(t("chat.composer.referenceLimit"));
      return;
    }
    setMentions((current) =>
      current.some((x) => x.kind === item.kind && x.id === item.id)
        ? current
        : [...current, item],
    );
    setText(text.slice(0, picker.start) + text.slice(picker.end));
    setPicker(null);
    setFocusRequest((value) => value + 1);
  };
  const submit = async () => {
    if (!text.trim() || sending || disabled || submitting || upload) return;
    setSubmitting(true);
    setError(null);
    const value = [
      text.trim(),
      ...mentions.map((item) => `@${item.label}`),
    ].join("\n");
    try {
      const currentContext =
        mentions.length && conversationId
          ? await api.conversations.dashboardContext(conversationId)
          : undefined;
      const context = mentions.length
        ? {
            mode: currentContext?.mode ?? ("none" as const),
            dashboard_ids: currentContext?.dashboard_ids ?? [],
            resource_refs: mentions.map((item) => ({
              type: item.kind,
              id: item.id,
            })),
          }
        : undefined;
      if (
        await onSend(
          value,
          files.map((file) => file.id),
          context,
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
      picker &&
      items.length &&
      (event.key === "ArrowDown" || event.key === "ArrowUp")
    ) {
      event.preventDefault();
      setActiveMention(
        (index) =>
          (index + (event.key === "ArrowDown" ? 1 : items.length - 1)) %
          items.length,
      );
      return;
    }
    if (
      event.key === "Enter" &&
      !event.shiftKey &&
      !event.nativeEvent.isComposing
    ) {
      event.preventDefault();
      if (picker && items.length) choose(items[activeMention] ?? items[0]!);
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
            <span className="argus-chat-chip" key={`${item.kind}:${item.id}`}>
              {item.kind === "host" ? (
                <Server size={12} />
              ) : item.kind === "dashboard" ? (
                <LayoutDashboard size={12} />
              ) : (
                <Cable size={12} />
              )}{" "}
              {item.label}
              <Button
                variant="ghost"
                type="button"
                aria-label={t("planv5.files.remove")}
                onPress={() =>
                  setMentions((current) =>
                    current.filter(
                      (x) => x.kind !== item.kind || x.id !== item.id,
                    ),
                  )
                }
              >
                <X size={11} />
              </Button>
            </span>
          ))}
          {files.map((file) => (
            <span className="argus-chat-chip" key={file.id}>
              <FileText size={12} />
              {file.name}
              <Button
                variant="ghost"
                type="button"
                aria-label={t("planv5.files.remove")}
                isDisabled={sending}
                onPress={() =>
                  setFiles((current) => current.filter((x) => x.id !== file.id))
                }
              >
                <X size={11} />
              </Button>
            </span>
          ))}
          {upload && (
            <span role="status">
              {upload.name} ·{" "}
              {t("planv5.files.uploading", { percent: upload.percent })}
              <Button
                variant="ghost"
                size="sm"
                onPress={() => activeUpload.current?.abort()}
              >
                {t("planv5.files.cancel")}
              </Button>
            </span>
          )}
        </div>
        {(error || modes.error) && <p role="alert">{error ?? modes.error}</p>}
        <div className="argus-chat-composer__box">
          {picker && (
            <div
              className="argus-chat-picker"
              id="argus-chat-resource-options"
              role="listbox"
              aria-label={t("chat.composer.mentionTitle")}
            >
              <div
                className="argus-chat-picker__list"
                onScroll={(event) => {
                  const node = event.currentTarget;
                  if (
                    node.scrollHeight - node.scrollTop - node.clientHeight <
                      80 &&
                    resources.hasNextPage &&
                    !resources.isFetchingNextPage
                  )
                    void resources.fetchNextPage();
                }}
              >
                {dashboards.isError && (
                  <p role="alert">{t("chat.dashboard.failed")}</p>
                )}
                {resources.isError && (
                  <p role="alert">{t("chat.composer.resourceFailed")}</p>
                )}
                {(resources.isFetching || controls.isFetching) && (
                  <p role="status">{t("chat.composer.resourceLoading")}</p>
                )}
                {items.length ? (
                  items.map((item, index) => (
                    <Button
                      variant="ghost"
                      className={`argus-chat-picker__item${index === activeMention ? " is-highlighted" : ""}`}
                      role="option"
                      aria-selected={index === activeMention}
                      id={`argus-mention-${item.kind}-${item.id}`}
                      type="button"
                      key={`${item.kind}:${item.id}`}
                      onPress={() => choose(item)}
                    >
                      {item.kind === "dashboard" && (
                        <LayoutDashboard size={13} />
                      )}{" "}
                      {item.label}
                      <small>
                        {t(`chat.resourceTypes.${item.kind}`)}
                        {item.context ? ` · ${item.context}` : ""}
                      </small>
                    </Button>
                  ))
                ) : !resources.isFetching && !controls.isFetching ? (
                  <p>{t("chat.composer.noMatch")}</p>
                ) : null}
              </div>
            </div>
          )}
          {interaction && onRespond && (
            <div hidden={!showInteraction}>
              <ComposerInteraction
                key={interaction.id}
                interaction={interaction}
                onRespond={onRespond}
                onClose={() => setCollapsedInteraction(interaction.id)}
              />
            </div>
          )}
          {!showInteraction && (
            <Textarea
              appearance="embedded"
              ref={area}
              aria-label={t("chat.composer.send")}
              aria-activedescendant={
                picker && items[activeMention]
                  ? `argus-mention-${items[activeMention]!.kind}-${items[activeMention]!.id}`
                  : undefined
              }
              aria-controls={picker ? "argus-chat-resource-options" : undefined}
              value={text}
              disabled={disabled || sending}
              rows={2}
              placeholder={t(
                disabled
                  ? "chat.composer.noModel"
                  : "chat.composer.placeholder",
              )}
              onKeyDown={keyDown}
              onChange={(event) =>
                updateText(event.target.value, event.target.selectionStart)
              }
            />
          )}
          <div className="argus-chat-composer__toolbar">
            <input
              hidden
              type="file"
              disabled={(!conversationId && disabled) || sending || !!upload}
              multiple
              ref={fileInput}
              onChange={(event) => void pickFiles(event)}
            />
            <ComposerActions
              label={t("chatInteractions.add")}
              trigger={
                <Button
                  aria-label={t("chatInteractions.add")}
                  variant="ghost"
                  isIconOnly
                >
                  <Plus size={17} />
                </Button>
              }
              sections={[
                {
                  label: t("chatInteractions.additions"),
                  items: [
                    {
                      id: "upload",
                      label: t("planv5.files.attach"),
                      icon: <Paperclip size={16} />,
                      disabled: disabled || sending || !!upload,
                      onSelect: () => fileInput.current?.click(),
                    },
                    {
                      id: "reference",
                      label: t("chat.composer.mention"),
                      icon: <AtSign size={16} />,
                      disabled: disabled || sending,
                      onSelect: () => {
                        const value =
                          text + (text && !/\s$/.test(text) ? " @" : "@");
                        updateText(value, value.length);
                        setFocusRequest((value) => value + 1);
                      },
                    },
                    {
                      id: "plan",
                      label: t("chatInteractions.enablePlan"),
                      icon: <Lightbulb size={16} />,
                      selected: modes.mode === "plan",
                      disabled: modes.saving,
                      onSelect: () =>
                        void modes.changeMode(
                          modes.mode === "plan" ? "execute" : "plan",
                        ),
                    },
                  ],
                },
                {
                  label: t("chatInteractions.skills"),
                  items: [
                    {
                      id: "dashboard",
                      label: t("chat.dashboard.createEntry"),
                      icon: <LayoutDashboard size={16} />,
                      disabled: disabled || sending || !canCreateDashboard,
                      onSelect: () => {
                        setText(t("chat.dashboard.command") + " ");
                        area.current?.focus();
                      },
                    },
                  ],
                },
              ]}
            />
            <ChatAccessSwitch
              value={modes.access}
              disabled={modes.saving}
              onChange={(value) => void modes.changeAccess(value)}
            />
            {modes.mode === "plan" && (
              <Button
                variant="ghost"
                size="sm"
                aria-label={t("chatInteractions.exitPlan")}
                isDisabled={modes.saving}
                onPress={() => void modes.changeMode("execute")}
              >
                <Lightbulb size={15} />
                {t("chatInteractions.plan")}
                <X size={12} />
              </Button>
            )}
            {interaction && !showInteraction && (
              <Button
                variant="ghost"
                size="sm"
                onPress={() => setCollapsedInteraction(null)}
              >
                {t("chatInteractions.reopen")}
              </Button>
            )}
            <span>{t("chat.composer.hint")}</span>
            {sending ? (
              <Button
                aria-label={t("chat.composer.stop")}
                onPress={onStop}
                isIconOnly
                variant="secondary"
              >
                <Square size={13} />
              </Button>
            ) : (
              <Button
                aria-label={t("chat.composer.send")}
                onPress={() => void submit()}
                isDisabled={disabled || !text.trim() || submitting || !!upload}
                isIconOnly
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
