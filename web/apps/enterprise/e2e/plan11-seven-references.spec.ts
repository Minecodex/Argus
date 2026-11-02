import { expect, test } from "@playwright/test";
import { writeFileSync } from "node:fs";
import {
  dashboardBinding,
  openSealedDashboard,
} from "./helpers/sealed-dashboard";

for (const english of [false, true])
  for (const dark of [false, true]) {
    test(`seven reference types and expired access ${english ? "en" : "zh"} ${dark ? "dark" : "light"}`, async ({
      page,
    }, info) => {
      test.setTimeout(90000);
      await page.setViewportSize({ width: 1440, height: 1000 });
      await openSealedDashboard(page, english, dark, dashboardBinding.Data);
      const conversation = new URL(page.url()).searchParams.get("c")!;
      const refs = await page.evaluate(() => {
        const key = "argus-mock:db-v14",
          db = JSON.parse(localStorage.getItem(key)!);
        const hint = JSON.parse(
          localStorage.getItem("argus.auth_hint.enterprise")!,
        );
        const member = db.enterpriseUsers.find(
          (row: { userId: string }) => row.userId === hint.user_id,
        );
        const enterprise = member.enterpriseId,
          at = new Date().toISOString();
        const refs = [
          "host",
          "kubernetes_cluster",
          "kubernetes_namespace",
          "application",
          "monitoring_target",
          "dashboard",
          "connector",
        ].map((type) => ({
          type,
          id: crypto.randomUUID(),
          name: `p11-${type.replaceAll("_", "-")}`,
        }));
        const ref = (type: string) => refs.find((row) => row.type === type)!;
        const host = db.hosts.find(
          (row: { enterpriseId: string; connectorId?: string }) =>
            row.enterpriseId === enterprise && row.connectorId,
        );
        const connector = db.connectors.find(
          (row: { id: string }) => row.id === host.connectorId,
        );
        const cluster = db.clusters.find(
          (row: { enterpriseId: string }) => row.enterpriseId === enterprise,
        );
        db.hosts.push({
          ...host,
          id: ref("host").id,
          name: ref("host").name,
          connectorId: ref("connector").id,
        });
        db.connectors.push({
          ...connector,
          id: ref("connector").id,
          name: ref("connector").name,
          hostId: ref("host").id,
        });
        db.clusters.push({
          ...cluster,
          id: ref("kubernetes_cluster").id,
          name: ref("kubernetes_cluster").name,
        });
        db.kubernetesInventory ??= {};
        db.kubernetesInventory[ref("kubernetes_cluster").id] = {
          objects: [],
          namespaces: [
            {
              id: ref("kubernetes_namespace").id,
              uid: crypto.randomUUID(),
              name: ref("kubernetes_namespace").name,
              version: 1,
            },
          ],
        };
        db.monitoring ??= {};
        const domain = (db.monitoring[enterprise] ??= {
          applications: [],
          services: [],
          bindings: [],
          targets: [],
        });
        domain.applications.push({
          ...ref("application"),
          application_key: ref("application").id,
          environment: "production",
          status: "active",
          version: 1,
          created_at: at,
          updated_at: at,
        });
        domain.targets.push({
          ...ref("monitoring_target"),
          kind: "database",
          engine: "mysql",
          identity: {},
          status: "active",
          version: 1,
        });
        const revision = crypto.randomUUID();
        db.dashboards.items.push({
          enterprise,
          value: {
            ...ref("dashboard"),
            lifecycle: "active",
            owner_user_id: hint.user_id,
            active_revision_id: revision,
            version: 1,
            created_at: at,
            updated_at: at,
          },
        });
        db.dashboards.revisions.push({
          id: revision,
          dashboard_id: ref("dashboard").id,
          name: ref("dashboard").name,
          revision_number: 1,
          spec: {
            schema_version: "argus.dashboard/v2",
            panels: [],
            variables: [],
          },
          created_at: at,
        });
        db.dataAuthorizationGrants ??= [];
        for (const row of refs.filter((row) => row.type !== "connector"))
          db.dataAuthorizationGrants.push({
            subject_type: "user",
            subject_id: hint.user_id,
            resource_type: row.type,
            resource_id: row.id,
            active: true,
          });
        localStorage.setItem(key, JSON.stringify(db));
        return refs;
      });
      await page.reload();
      const area = page.locator(".argus-chat-composer").getByRole("textbox");
      for (const ref of refs) {
        await area.fill(`@${ref.name}`);
        const option = page
          .getByRole("listbox")
          .locator(`#argus-mention-${ref.type}-${ref.id}`);
        await expect(option).toHaveCount(1);
        await expect(option).toContainText(ref.name);
        await option.click();
      }
      await expect(page.locator(".argus-chat-chip")).toHaveCount(7);
      await area.fill(
        english
          ? "Inspect the seven referenced resources"
          : "检查这些已引用的资源",
      );
      await page
        .locator(".argus-chat-composer")
        .getByRole("button", {
          name: english ? "Send" : "发送",
          exact: true,
        })
        .click();
      await expect(area).toHaveValue("");
      await page.evaluate((conversation) => {
        const key = "argus-mock:db-v14",
          db = JSON.parse(localStorage.getItem(key)!);
        db.messages = db.messages.filter(
          (row: { conversationId: string; role: string }) =>
            row.conversationId !== conversation || row.role === "user",
        );
        localStorage.setItem(key, JSON.stringify(db));
      }, conversation);
      await page.reload();
      const summary = page.getByRole("button", {
        name: english ? "Pinned summary" : "置顶摘要",
        exact: true,
      });
      await summary.click();
      const list = page.locator(".argus-chat-resource-list");
      await expect(list.locator("li")).toHaveCount(7);
      const host = refs.find((ref) => ref.type === "host")!;
      for (const ref of refs) {
        const href =
          ref.type === "host" || ref.type === "connector"
            ? `/hosts/${host.id}`
            : ref.type === "kubernetes_cluster"
              ? `/kubernetes/${ref.id}`
              : ref.type === "application"
                ? `/applications/${ref.id}`
                : ref.type === "dashboard"
                  ? `/dashboards/${ref.id}`
                  : `/observe?resource_type=${ref.type}&resource_id=${ref.id}`;
        await expect(
          list.getByRole("link", { name: ref.name, exact: true }),
        ).toHaveAttribute("href", href);
      }
      await page.screenshot({
        path: info.outputPath("seven-reference-links.png"),
      });
      await page.keyboard.press("Escape");
      await page.evaluate(
        ({ refs }) => {
          const key = "argus-mock:db-v14",
            db = JSON.parse(localStorage.getItem(key)!);
          const ids = new Set(refs.map((row) => row.id));
          db.dataAuthorizationGrants = db.dataAuthorizationGrants.filter(
            (row: { resource_id: string }) => !ids.has(row.resource_id),
          );
          // Connector is a control-plane object, not a QueryScope data grant.
          db.connectors = db.connectors.filter(
            (row: { id: string }) => !ids.has(row.id),
          );
          const hint = JSON.parse(
            localStorage.getItem("argus.auth_hint.enterprise")!,
          );
          db.dataAuthorizationVersions ??= {};
          db.dataAuthorizationVersions[`user:${hint.user_id}`] =
            (db.dataAuthorizationVersions[`user:${hint.user_id}`] ?? 1) + 1;
          localStorage.setItem(key, JSON.stringify(db));
        },
        { refs },
      );
      await page.reload();
      await summary.click();
      await expect(list.locator("li")).toHaveCount(7);
      await expect(
        list.getByText(
          english
            ? "Resource directory unavailable or access changed"
            : "资源目录读取失败或权限已变更",
          { exact: true },
        ),
      ).toHaveCount(7);
      await expect(list.locator("a")).toHaveCount(0);
      for (const ref of refs) await expect(list).not.toContainText(ref.name);
      await page.screenshot({
        path: info.outputPath("expired-reference-links.png"),
      });
      writeFileSync(
        info.outputPath("receipt.json"),
        JSON.stringify(
          {
            passed: true,
            layer: "L4",
            controlled_mock_domains: true,
            locale: english ? "en-US" : "zh-CN",
            theme: dark ? "dark" : "light",
            reference_types: refs.map((row) => row.type),
            typed_mentions_and_summary_links: true,
            six_data_grants_revoked: true,
            connector_deleted_not_a_data_grant: true,
            real_deployed_revocation_claimed: false,
          },
          null,
          2,
        ),
      );
    });
  }
