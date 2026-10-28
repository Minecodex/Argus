import { useNavigate } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { Search } from "lucide-react";
import type {
  ObservationResourceRef,
  ObservationSignal,
} from "@argus/api-client";
import { Button } from "@argus/ui";
export function ResourceObserveLink({
  type,
  id,
  signal = "metrics",
}: {
  type: ObservationResourceRef["type"];
  id: string;
  signal?: ObservationSignal;
}) {
  const { t } = useTranslation(),
    navigate = useNavigate();
  return (
    <Button
      onPress={() =>
        void navigate({
          to: "/observe",
          search: { resource_type: type, resource_id: id, signal },
        })
      }
    >
      <Search />
      {t("observe.title")}
    </Button>
  );
}
