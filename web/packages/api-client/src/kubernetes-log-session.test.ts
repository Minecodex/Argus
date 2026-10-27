import { expect, it } from "vitest";
import { createMockApiClient } from "./mock";
import type { KubernetesLogSessionInput } from "./kubernetes-workbench";

it("consumes one-use log tickets, isolates cursors and clears capabilities after logout", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  await api.auth.login({ username: "root", password: "123456" });
  const cluster = "k8s-prod-east",
    page = await api.kubernetesWorkbench.list(cluster, { type: "pod" }),
    pod = page.items[0]!;
  const input: KubernetesLogSessionInput = {
    namespace_id: page.namespace_references[pod.namespace_uid!]!,
    pod: pod.name,
    pod_uid: pod.uid,
    container: "main",
    previous: false,
    tail_lines: 100,
    since_seconds: 300,
  };
  const ticket = await api.kubernetesWorkbench.createLogSession(cluster, input);
  await expect(
    api.kubernetesWorkbench.readLogSession(cluster, ticket.session.id),
  ).rejects.toThrow();
  await expect(
    api.kubernetesWorkbench.attachLogSession(
      cluster,
      ticket.session.id,
      "wrong",
    ),
  ).rejects.toThrow();
  await api.kubernetesWorkbench.attachLogSession(
    cluster,
    ticket.session.id,
    ticket.ticket,
  );
  await expect(
    api.kubernetesWorkbench.attachLogSession(
      cluster,
      ticket.session.id,
      ticket.ticket,
    ),
  ).rejects.toThrow();
  const first = await api.kubernetesWorkbench.readLogSession(
    cluster,
    ticket.session.id,
  );
  expect(first.content).toContain("mock main current log 1");
  expect(first.redacted).toBe(true);
  const next = await api.kubernetesWorkbench.readLogSession(
    cluster,
    ticket.session.id,
    first.next_cursor,
  );
  expect(next.content).toContain("log 2");
  const other = await api.kubernetesWorkbench.createLogSession(cluster, input);
  await api.kubernetesWorkbench.attachLogSession(
    cluster,
    other.session.id,
    other.ticket,
  );
  await expect(
    api.kubernetesWorkbench.readLogSession(
      cluster,
      other.session.id,
      first.next_cursor,
    ),
  ).rejects.toThrow();
  await api.auth.logout();
  await expect(
    api.kubernetesWorkbench.readLogSession(cluster, ticket.session.id),
  ).rejects.toThrow();
  await api.auth.login({ username: "root", password: "123456" });
  await expect(
    api.kubernetesWorkbench.readLogSession(cluster, ticket.session.id),
  ).rejects.toThrow();
});

it("pins the selected container, bounds previous logs and makes closing effective", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  await api.auth.login({ username: "root", password: "123456" });
  const cluster = "k8s-prod-east",
    page = await api.kubernetesWorkbench.list(cluster, { type: "pod" }),
    pod = page.items[0]!;
  const input: KubernetesLogSessionInput = {
    namespace_id: page.namespace_references[pod.namespace_uid!]!,
    pod: pod.name,
    pod_uid: pod.uid,
    container: "sidecar",
    previous: true,
    tail_lines: 100,
    since_seconds: 300,
  };
  await expect(
    api.kubernetesWorkbench.createLogSession(cluster, {
      ...input,
      pod_uid: "replacement",
    }),
  ).rejects.toThrow();
  await expect(
    api.kubernetesWorkbench.createLogSession(cluster, {
      ...input,
      container: "not-a-container",
    }),
  ).rejects.toThrow();
  await expect(
    api.kubernetesWorkbench.createLogSession(cluster, {
      ...input,
      tail_lines: 1001,
    }),
  ).rejects.toThrow();
  const ticket = await api.kubernetesWorkbench.createLogSession(cluster, input);
  await api.kubernetesWorkbench.attachLogSession(
    cluster,
    ticket.session.id,
    ticket.ticket,
  );
  const result = await api.kubernetesWorkbench.readLogSession(
    cluster,
    ticket.session.id,
  );
  expect(result.content).toContain("sidecar previous");
  expect(result.complete).toBe(true);
  await api.kubernetesWorkbench.closeLogSession(cluster, ticket.session.id);
  await expect(
    api.kubernetesWorkbench.readLogSession(
      cluster,
      ticket.session.id,
      result.next_cursor,
    ),
  ).rejects.toThrow();
});
