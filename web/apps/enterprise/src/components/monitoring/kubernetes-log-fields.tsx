import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useApi,
  kubernetesObjectContainers,
  type KubernetesInventoryObject,
  type MonitoringSchemas,
} from "@argus/api-client";
import { Button, Field, ResourcePickerDialog, Select } from "@argus/ui";
import {
  NamespacePicker,
  type NamespaceChoice,
} from "../kubernetes/namespace-picker";
import { useInventoryError } from "../kubernetes/inventory-error";

type Workload = MonitoringSchemas["MonitoringLogWorkload"];

export function KubernetesLogFields({
  clusterId,
  onChange,
}: {
  clusterId: string;
  onChange: (value: Workload | undefined) => void;
}) {
  const { t } = useTranslation();
  const [namespace, setNamespace] = useState<NamespaceChoice>(),
    [workload, setWorkload] = useState<KubernetesInventoryObject>(),
    [containers, setContainers] = useState<string[]>([]);
  const [picker, setPicker] = useState<
    "namespace" | "workload" | "containers"
  >();
  const publish = (object: KubernetesInventoryObject, names: string[]) => {
    if (!namespace || !names.length) {
      onChange(undefined);
      return;
    }
    onChange({
      namespace_id: namespace.id,
      type: object.definition.key as Workload["type"],
      name: object.name,
      uid: object.uid,
      containers: names,
    });
  };
  return (
    <>
      <Field label={t("kubernetesWorkbench.namespace")} requirement="required">
        <Button type="button" onPress={() => setPicker("namespace")}>
          {namespace?.name ?? t("kubernetesWorkbench.chooseNamespace")}
        </Button>
      </Field>
      <Field
        label={t("logCollection.workload")}
        requirement="required"
        hint={t("logCollection.workloadHint")}
      >
        <Button
          type="button"
          isDisabled={!namespace}
          onPress={() => setPicker("workload")}
        >
          {workload?.name ?? t("logCollection.chooseWorkload")}
        </Button>
      </Field>
      <Field label={t("logCollection.containers")} requirement="required">
        <Button
          type="button"
          isDisabled={!workload}
          onPress={() => setPicker("containers")}
        >
          {containers.length
            ? containers.join(", ")
            : t("logCollection.chooseContainers")}
        </Button>
      </Field>
      {picker === "namespace" && (
        <NamespacePicker
          clusterId={clusterId}
          onClose={() => setPicker(undefined)}
          onSelect={(value) => {
            setNamespace(value);
            setWorkload(undefined);
            setContainers([]);
            onChange(undefined);
            setPicker(undefined);
          }}
        />
      )}
      {picker === "workload" && namespace && (
        <WorkloadPicker
          clusterId={clusterId}
          namespace={namespace}
          onClose={() => setPicker(undefined)}
          onSelect={(value) => {
            setWorkload(value);
            setContainers([]);
            onChange(undefined);
            setPicker(undefined);
          }}
        />
      )}
      {picker === "containers" && workload && (
        <ContainerPicker
          object={workload}
          selected={containers}
          onClose={() => setPicker(undefined)}
          onDone={(names) => {
            setContainers(names);
            publish(workload, names);
            setPicker(undefined);
          }}
        />
      )}
    </>
  );
}

function WorkloadPicker({
  clusterId,
  namespace,
  onClose,
  onSelect,
}: {
  clusterId: string;
  namespace: NamespaceChoice;
  onClose: () => void;
  onSelect: (value: KubernetesInventoryObject) => void;
}) {
  const api = useApi(),
    { t } = useTranslation(),
    formatError = useInventoryError();
  const [type, setType] = useState("deployment"),
    [search, setSearch] = useState(""),
    [cursors, setCursors] = useState<Array<string | undefined>>([undefined]);
  const query = useQuery({
    queryKey: [
      "kubernetes-workbench",
      clusterId,
      "log-workload",
      namespace.id,
      type,
      search,
      cursors.at(-1),
    ],
    queryFn: ({ signal }) =>
      api.kubernetesWorkbench.list(
        clusterId,
        {
          type,
          namespace_mode: "explicit",
          namespace_ids: [namespace.id],
          name: search,
          limit: 30,
          cursor: cursors.at(-1),
        },
        signal,
      ),
  });
  return (
    <ResourcePickerDialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={t("logCollection.chooseWorkload")}
      description={`${namespace.name} · ${t("logCollection.exactNameHint")}`}
      data={query.error ? [] : (query.data?.items ?? [])}
      columns={[
        { key: "name", header: t("kubernetesWorkbench.name") },
        { key: "uid", header: t("kubernetesWorkbench.uid") },
      ]}
      getRowKey={(row) => row.uid}
      getRowLabel={(row) => row.name}
      search={search}
      onSearch={(value) => {
        setSearch(value);
        setCursors([undefined]);
      }}
      page={cursors.length}
      loading={query.isFetching}
      error={formatError(query.error)}
      hasPrevious={cursors.length > 1}
      hasNext={!!query.data?.next_cursor}
      onPrevious={() => setCursors((v) => v.slice(0, -1))}
      onNext={() => setCursors((v) => [...v, query.data?.next_cursor])}
      isDisabled={(row) => kubernetesObjectContainers(row).length === 0}
      onSelect={onSelect}
      filters={
        <Field label={t("logCollection.workloadType")} requirement="required">
          <Select
            value={type}
            onValueChange={(value) => {
              setType(value);
              setSearch("");
              setCursors([undefined]);
            }}
            options={[
              "deployment",
              "pod",
              "daemonset",
              "statefulset",
              "replicaset",
              "job",
              "cronjob",
            ].map((value) => ({
              value,
              label: t(`kubernetesWorkbench.types.${value}`, {
                defaultValue: value,
              }),
            }))}
          />
        </Field>
      }
    />
  );
}

function ContainerPicker({
  object,
  selected,
  onClose,
  onDone,
}: {
  object: KubernetesInventoryObject;
  selected: string[];
  onClose: () => void;
  onDone: (names: string[]) => void;
}) {
  const { t } = useTranslation(),
    [search, setSearch] = useState(""),
    [draft, setDraft] = useState(selected);
  const rows = kubernetesObjectContainers(object).filter((row) =>
    row.name.includes(search),
  );
  return (
    <ResourcePickerDialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={t("logCollection.chooseContainers")}
      description={object.name}
      data={rows}
      columns={[
        { key: "name", header: t("logCollection.containerName") },
        {
          key: "kind",
          header: t("logCollection.containerKind"),
          render: (row) => t(`logCollection.containerKinds.${row.kind}`),
        },
      ]}
      getRowKey={(row) => row.name}
      getRowLabel={(row) => row.name}
      search={search}
      onSearch={setSearch}
      selectedKeys={draft}
      onToggle={(row) =>
        setDraft((values) =>
          values.includes(row.name)
            ? values.filter((name) => name !== row.name)
            : [...values, row.name],
        )
      }
      onConfirm={() => onDone(draft)}
      confirmDisabled={!draft.length}
    />
  );
}
