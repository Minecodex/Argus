import { useTranslation } from "react-i18next";
import { formatApiError } from "@argus/api-client";
import { useCallback } from "react";

export function useInventoryError() {
  const { t } = useTranslation();
  return useCallback(
    (error: unknown) =>
      error
        ? formatApiError(
            error,
            t("kubernetesWorkbench.errors.unavailable"),
            (requestId) => t("common.requestReference", { requestId }),
          )
        : "",
    [t],
  );
}
