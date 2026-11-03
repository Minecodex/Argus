import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { useApi, type ObservationResourceRef } from "@argus/api-client";
import { KeyValueGrid } from "@argus/ui";
import { ResourceObserveLink } from "../observe/resource-link";
import { readObservationReference } from "./observation-reference";

export function ResourceReferenceDetails({ type, id }: ObservationResourceRef) {
  const api = useApi(),
    { t } = useTranslation();
  const query = useQuery({
    queryKey: ["chat-resource-reference", type, id],
    retry: false,
    staleTime: 0,
    queryFn: () => readObservationReference(api, { type, id }),
  });
  if (query.isError)
    return <p role="alert">{t("chat.composer.resourceFailed")}</p>;
  if (!query.data || query.isFetching)
    return <p role="status">{t("chat.composer.resourceLoading")}</p>;
  const item = query.data;
  return (
    <>
      <KeyValueGrid
        items={[
          { label: t("chat.resourceDetails.name"), value: item.name },
          {
            label: t("chat.resourceDetails.type"),
            value: t(`chat.resourceTypes.${type}`),
          },
          { label: t("chat.resourceDetails.id"), value: item.id },
          {
            label: t("chat.resourceDetails.environment"),
            value: item.environment || t("chat.resourceDetails.unknown"),
          },
          {
            label: t("chat.resourceDetails.parent"),
            value: item.parent_name || t("chat.resourceDetails.unknown"),
          },
        ]}
      />
      <ResourceObserveLink type={type} id={id} />
    </>
  );
}
