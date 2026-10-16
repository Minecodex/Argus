import type { AlertConfiguration, AlertSchemas } from "@argus/api-client";

// Defaults are tenant configuration, never a demo ID or the first list item.
// Apply only when a fresh draft opens; editing/copying keeps its own targets.
export function defaultPolicyChannelIds(configurations: AlertConfiguration[]) {
  return configurations
    .filter((configuration) => {
      if (configuration.kind !== "channel") return false;
      const channel = configuration.spec as AlertSchemas["AlertChannelSpec"];
      return channel.enabled && channel.default_for_new_policies === true;
    })
    .map((channel) => channel.id);
}
