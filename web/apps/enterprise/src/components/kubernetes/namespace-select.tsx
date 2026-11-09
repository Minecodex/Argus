import { useRef } from "react";
import { useTranslation } from "react-i18next";
import { useApi } from "@argus/api-client";
import { ComboBox } from "@argus/ui";
import type { NamespaceChoice } from "./namespace-picker";

const allNamespaces = "all_authorized";

/** Resolve candidates through the authorized inventory, preserving UID-bound IDs. */
export function NamespaceSelect({
  clusterId,
  scopeVersion,
  value,
  onChange,
}: {
  clusterId: string;
  scopeVersion: number;
  value?: NamespaceChoice;
  onChange: (value: NamespaceChoice | undefined) => void;
}) {
  const api = useApi();
  const { t } = useTranslation();
  const choices = useRef(new Map<string, NamespaceChoice>());
  const allLabel = t("kubernetesWorkbench.allAuthorized");
  const allOption = { value: allNamespaces, label: allLabel };
  return (
    <ComboBox
      ariaLabel={t("kubernetesWorkbench.scope")}
      className="argus-k8s-namespace-select"
      value={value?.id ?? allNamespaces}
      displayLabel
      options={[
        allOption,
        ...(value ? [{ value: value.id, label: value.name }] : []),
      ]}
      contextKey={`${clusterId}:${scopeVersion}:${allLabel}`}
      fetchOptions={async (search, cursor, signal) => {
        const page = await api.kubernetesWorkbench.list(
          clusterId,
          {
            type: "namespace",
            namespace_mode: "all_authorized",
            name: search === allLabel || search === value?.name ? "" : search,
            limit: 30,
            cursor,
          },
          signal,
        );
        if (!cursor) choices.current.clear();
        const options = page.items.flatMap((row) => {
          const id = page.namespace_references[row.uid];
          if (!id) return [];
          choices.current.set(id, { id, name: row.name });
          return [{ value: id, label: row.name }];
        });
        return {
          options: cursor ? options : [allOption, ...options],
          next_cursor: page.next_cursor,
        };
      }}
      onValueChange={(id) => {
        if (id === allNamespaces) onChange(undefined);
        else {
          const choice = choices.current.get(id);
          if (choice) onChange(choice);
        }
      }}
    />
  );
}
