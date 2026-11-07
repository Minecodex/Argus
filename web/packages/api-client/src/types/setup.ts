/** One-time initialization wizard payloads (docs/07 §2-4). */

export type PlatformState = "uninitialized" | "initializing" | "initialized";

export interface SetupStatus {
  state: PlatformState;
  platformName?: string;
}

export interface SetupSubmission {
  /** Deployment-provided one-time Setup Token. */
  setupToken: string;
  enterpriseName: string;
  timezone: string;
  username: string;
  displayName: string;
  password: string;
}

export interface SetupResult {
  success: boolean;
  superAdminUserId: string;
  enterpriseId: string;
  enterpriseAdminUserId: string;
}
