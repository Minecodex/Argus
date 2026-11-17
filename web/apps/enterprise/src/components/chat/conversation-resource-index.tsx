import { useTranslation } from "react-i18next";
import { useState } from "react";
import { useConversationResources } from "./conversation-resources";
import type { ChatMessage } from "./chat-view-model";
import { Badge, DataTable, Pagination, SearchInput, Select } from "@argus/ui";

export function ConversationResourceIndex({
  messages,
}: {
  messages: ChatMessage[];
}) {
  const { t } = useTranslation();
  const resources = useConversationResources(messages);
  const [category, setCategory] = useState("");
  const [search, setSearch] = useState("");
  const [requestedPage, setPage] = useState(1);
  const pageSize = 5;
  const term = search.trim().toLocaleLowerCase();
  const filtered = resources.filter(
    (ref) =>
      (!category || ref.type === category) &&
      (!term || (ref.name ?? "").toLocaleLowerCase().includes(term)),
  );
  const pages = Math.max(1, Math.ceil(filtered.length / pageSize));
  const page = Math.min(requestedPage, pages);
  const offset = (page - 1) * pageSize;
  const rows = filtered.slice(offset, offset + pageSize);
  return (
    <details
      data-testid="conversation-resources-section"
      className="argus-chat-resource-index"
    >
      <summary>
        {t("chat.resourceDetails.references")} · {resources.length}
      </summary>
      <div className="argus-chat-resource-index__filters">
        <Select
          value={category}
          ariaLabel={t("chat.resourceDetails.category")}
          onValueChange={(value) => {
            setCategory(value);
            setPage(1);
          }}
          options={[
            { value: "", label: t("chat.resourceDetails.allCategories") },
            ...[...new Set(resources.map((ref) => ref.type))].map((type) => ({
              value: type,
              label: t(`chat.resourceTypes.${type}`),
            })),
          ]}
        />
        <SearchInput
          value={search}
          aria-label={t("chat.resourceDetails.search")}
          placeholder={t("chat.resourceDetails.search")}
          onChange={(event) => {
            setSearch(event.target.value);
            setPage(1);
          }}
        />
      </div>
      <DataTable
        density="compact"
        surface="plain"
        data={rows}
        getRowKey={(ref) => `${ref.type}:${ref.id}`}
        columns={[
          {
            key: "type",
            header: t("chat.resourceDetails.category"),
            render: (ref) => (
              <Badge>{t(`chat.resourceTypes.${ref.type}`)}</Badge>
            ),
          },
          {
            key: "name",
            header: t("chat.resourceDetails.name"),
            cellClassName: "argus-chat-resource-index__name",
            render: (ref) =>
              ref.unavailable ? (
                <span>{t("chat.composer.resourceFailed")}</span>
              ) : ref.href ? (
                <a href={ref.href}>
                  {ref.name ?? t("chat.composer.resourceLoading")}
                </a>
              ) : (
                <span>{ref.name ?? t("chat.composer.resourceLoading")}</span>
              ),
          },
        ]}
      />
      {!rows.length && <p>{t("chat.resourceDetails.noMatches")}</p>}
      <Pagination
        page={page}
        totalPages={pages}
        onChange={setPage}
        ariaLabel={t("chat.resourceDetails.pagination")}
        summary={t("chat.resourceDetails.range", {
          from: filtered.length ? offset + 1 : 0,
          to: Math.min(offset + pageSize, filtered.length),
          total: filtered.length,
        })}
      />
    </details>
  );
}
