import type { AuditEvent } from "./audit";
import type { ISODateString as Iso } from "./common";

export type EnterpriseStatus = "active" | "suspended" | "disabled";

export interface Enterprise {
  id: string;
  name: string;
  code: string;
  status: EnterpriseStatus;
  timezone: string;
  sandboxQuotaProfile?: string;
  remark?: string;
  createdAt: Iso;
}

export interface CreateEnterpriseInput {
  name: string;
  code: string;
  timezone?: string;
  sandboxQuotaProfile?: string;
  remark?: string;
}

export interface UpdateEnterpriseInput {
  name?: string;
  timezone?: string;
  sandboxQuotaProfile?: string;
  remark?: string;
}

export type EnterpriseAdminCredentialStatus =
  "temporary_password" | "active" | "disabled";

export interface EnterpriseAdmin {
  id: string;
  enterpriseId: string;
  username: string;
  displayName: string;
  email?: string;
  credentialStatus: EnterpriseAdminCredentialStatus;
  lastLoginAt?: Iso;
  createdAt: Iso;
  version?: number;
  temporaryPassword?: string;
  temporaryPasswordExpiresAt?: Iso;
}

export interface CreateEnterpriseAdminInput {
  enterpriseId: string;
  username: string;
  displayName: string;
  email?: string;
}

export interface SandboxBackend {
  id: string;
  name: string;
  endpoint: string;
  credentialRef: string;
  tlsVerify: boolean;
  enabled: boolean;
  defaultStorage: string;
  healthStatus: "healthy" | "degraded" | "unreachable";
  createdAt: Iso;
}

export interface CreateSandboxBackendInput {
  name: string;
  endpoint: string;
  credentialRef: string;
  tlsVerify?: boolean;
  defaultStorage?: string;
}

export interface SandboxImageLanguage {
  name: string;
  version: string;
}

export interface SandboxImage {
  id: string;
  name: string;
  /** Immutable reference pinned by digest. */
  reference: string;
  digest: string;
  languages: SandboxImageLanguage[];
  scanStatus: "pending" | "passed" | "failed";
  signatureStatus: "verified" | "unsigned" | "failed";
  enabled: boolean;
  createdAt: Iso;
}

export interface CreateSandboxImageInput {
  name: string;
  reference: string;
  digest: string;
  languages: SandboxImageLanguage[];
}

/** Platform-owned offline execution profile. Workspace storage is configured separately. */
export interface SandboxProfile {
  id: string;
  name: string;
  imageId: string;
  resources: { cpu: number; memoryMb: number };
  timeoutSeconds: number;
  taskKinds: Array<"smoke" | "agent_workspace">;
  networkMode: "none" | "restricted";
  enabled: boolean;
  createdAt: Iso;
}

export interface CreateSandboxProfileInput {
  name: string;
  imageId: string;
  resources: { cpu: number; memoryMb: number };
  timeoutSeconds: number;
}

/** Per-enterprise sandbox quota, set by super admins (docs/08 §9). */
export interface EnterpriseSandboxQuota {
  enterpriseId: string;
  version: number;
  maxConcurrentSessions: number;
  monthlySessionSeconds: number;
}

/** Platform-visible sandbox session metadata only, never content. */
export interface SandboxSessionMeta {
  id: string;
  enterpriseId: string;
  userId: string;
  profileId: string;
  conversationId?: string;
  runId?: string;
  purpose?: string;
  status:
    | "requested"
    | "starting"
    | "running"
    | "idle"
    | "terminating"
    | "terminated"
    | "failed"
    | "rejected";
  startedAt?: Iso;
  lastActivityAt?: Iso;
  terminatedAt?: Iso;
}

/** Platform audit is kept separate from enterprise audit (docs/07 §11). */
export type PlatformAuditEvent = AuditEvent;

export type PKIBundleState =
  "stable" | "preparing" | "overlapping" | "retiring" | "failed";
export type PKIRotationDirection = "forward" | "rollback";

export type PKIBundleStatus = {
  epoch: number;
  state: PKIBundleState;
  direction: PKIRotationDirection;
  bundleSha256: string;
  currentCaFingerprints: string[];
  nextCaFingerprints: string[];
  startedAt: Iso;
  retireAt?: Iso;
  lastError?: string;
};

export type PKINodeKind =
  "connector" | "collector" | "kubernetes_connector" | "control_plane";

export type PKINodeTrustStatus =
  "pending" | "acked" | "failed" | "trust_expired";

export type PKINodeStatus = {
  id: string;
  kind: PKINodeKind;
  enterpriseId?: string;
  epoch: number;
  bundleSha256?: string;
  caFingerprints?: string[];
  status: PKINodeTrustStatus;
  blocksCutover: boolean;
  error?: string;
  acknowledgedAt?: Iso;
  updatedAt: Iso;
};

export interface PlatformPKIStatus {
  bundles: PKIBundleStatus[];
  nodes: PKINodeStatus[];
  acknowledgedNodes: number;
  pendingNodes: number;
  failedNodes: number;
  trustExpiredNodes: number;
}
