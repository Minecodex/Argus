import { createMockApiClient } from "@argus/api-client";
import { expect, it, vi } from "vitest";
import {
  observationReferenceTypes,
  readObservationReference,
} from "./observation-reference";

for (const type of observationReferenceTypes) {
  it(`reads ${type} from the authorized observation directory without management fallback`, async () => {
    const api = createMockApiClient({ persist: false, delay: 0 });
    const item = {
      id: "exact-id",
      type,
      name: "Authorized reference",
      environment: "production",
    };
    const read = vi
      .spyOn(api.telemetry, "resources")
      .mockResolvedValue({ items: [item] });
    const host = vi.spyOn(api.hosts, "get"),
      cluster = vi.spyOn(api.kubernetes, "getCluster"),
      application = vi.spyOn(api.monitoring, "application");
    expect(await readObservationReference(api, { type, id: item.id })).toEqual(
      item,
    );
    expect(read).toHaveBeenCalledWith({
      kind: type,
      search: item.id,
      limit: 1,
    });
    for (const wrong of [
      { items: [] },
      { items: [{ ...item, id: "other-id" }] },
      { items: [{ ...item, type: type === "host" ? "application" : "host" }] },
    ]) {
      read.mockResolvedValueOnce(wrong as never);
      await expect(
        readObservationReference(api, { type, id: item.id }),
      ).rejects.toThrow("Resource unavailable");
    }
    expect(host).not.toHaveBeenCalled();
    expect(cluster).not.toHaveBeenCalled();
    expect(application).not.toHaveBeenCalled();
  });
}
