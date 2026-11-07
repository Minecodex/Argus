import type { ArgusApiClient } from "../client";
import type { User } from "../types";
import type { MockContext } from "./context";
import { createMockEnterprise } from "./platform";
import { nextId } from "./store";

/** One-time platform initialization wizard. */
export function createSetupDomain(ctx: MockContext): ArgusApiClient["setup"] {
  const { db } = ctx;

  return {
    async status() {
      await ctx.pause();
      return {
        state: db.platformState.state,
        platformName: db.platformState.name || undefined,
      };
    },
    async submit(input) {
      await ctx.pause();
      if (db.platformState.state !== "uninitialized") {
        throw new Error("platform already initialized");
      }
      if (!input.setupToken) throw new Error("invalid setup token");
      const createdAt = ctx.nowIso();
      const { enterprise, departmentId } = createMockEnterprise(ctx, {
        name: input.enterpriseName,
        timezone: input.timezone,
      });
      const role = db.roles.find(
        (entry) =>
          entry.enterprise_id === enterprise.id &&
          entry.builtin_key === "enterprise_admin",
      )!;
      const admin: User = {
        id: nextId(db, "u"),
        username: input.username,
        displayName: input.displayName,
        platformRole: "platform_super_admin",
        status: "active",
        mfaEnabled: false,
        createdAt,
      };
      const enterpriseAdmin: User = {
        id: nextId(db, "u"),
        username: input.username,
        displayName: input.displayName,
        status: "active",
        mfaEnabled: false,
        createdAt,
      };
      db.users.push(admin, enterpriseAdmin);
      db.credentials[admin.id] = input.password;
      db.credentials[enterpriseAdmin.id] = input.password;
      db.enterpriseUsers.push({
        userId: enterpriseAdmin.id,
        enterpriseId: enterprise.id,
        departmentId,
      });
      db.enterpriseAdmins.push({
        id: enterpriseAdmin.id,
        enterpriseId: enterprise.id,
        username: enterpriseAdmin.username,
        displayName: enterpriseAdmin.displayName,
        credentialStatus: "active",
        createdAt,
      });
      db.roleBindings.push({
        id: nextId(db, "rb"),
        enterprise_id: enterprise.id,
        subject_type: "user",
        subject_id: enterpriseAdmin.id,
        role_id: role.id,
        status: "active",
        version: 1,
        created_at: createdAt,
        updated_at: createdAt,
      });
      db.platformState = { state: "initialized", name: "Argus" };
      db.auditEvents.unshift({
        id: nextId(db, "aud"),
        enterpriseId: null,
        actorUserId: admin.id,
        actorName: admin.displayName,
        actorType: "platform_user",
        actorUsername: admin.username,
        action: "setup.initialize",
        origin: "platform_ui",
        summary: "平台初始化完成",
        result: "success",
        createdAt: ctx.nowIso(),
      });
      ctx.save();
      return {
        success: true,
        superAdminUserId: admin.id,
        enterpriseId: enterprise.id,
        enterpriseAdminUserId: enterpriseAdmin.id,
      };
    },
  };
}
