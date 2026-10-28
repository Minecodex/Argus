import { useQuery } from "@tanstack/react-query";
import { useApi } from "@argus/api-client";
import { useEnterpriseAuthStore } from "@argus/auth";

export function useModelGovernanceMetadata() {
  const api = useApi();
  const session = useEnterpriseAuthStore((state) => state.session);
  return useQuery({
    queryKey: [
      "model-governance-metadata",
      session?.session.id,
      session?.session.authorization_version,
    ],
    queryFn: async () => {
      const item = (await api.dashboards.list()).find(
        (item) => item.managed_template_key === "argus.model-governance",
      );
      if (!item) throw new Error("MODEL_GOVERNANCE_UNAVAILABLE");
      return (
        await api.dashboards.execute(item.id, {
          resource_ids: [],
          variables: {},
          local_values: {},
          candidates_only: true,
        })
      ).model_governance;
    },
  });
}
