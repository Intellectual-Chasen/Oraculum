import {
  type Decoder,
  readObject,
  requireArray,
  requireBoolean,
  requireCount,
  requireEnum,
  requireString,
} from "./decoding";

/** 証拠を送る LLM の提供者。定義元は `backend/core/assist_permission.go` の `AssistProvider`。 */
export const assistProviders = ["claude"] as const;
export type AssistProvider = (typeof assistProviders)[number];

/** 送信の許可の改訂が記録したこと。定義元は `AssistPermissionAction`。 */
export const assistPermissionActions = ["grant", "revoke"] as const;
export type AssistPermissionAction = (typeof assistPermissionActions)[number];

/** 送信の許可の改訂 1 つ。定義元は `backend/core/assist_permission.go` の `AssistPermissionRevision`。 */
export type AssistPermissionRevision = {
  provider: AssistProvider;
  revisionNumber: number;
  action: AssistPermissionAction;
  analyst: string;
  recordedAt: string;
};

/**
 * 提供者 1 つへの送信の許可の現在の状態と、記録した順の改訂。
 * 定義元は `backend/api/assist_permissions.go` の `assistProviderPermission`。
 */
export type AssistProviderPermission = {
  provider: AssistProvider;
  permitted: boolean;
  revisions: AssistPermissionRevision[];
};

/** 送信の許可の一覧の応答。定義元は `assistPermissionsResponse`。 */
export type AssistPermissionsResponse = {
  providers: AssistProviderPermission[];
};

const decodeAssistPermissionRevision: Decoder<AssistPermissionRevision> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    provider: requireEnum(source, "provider", path, assistProviders),
    revisionNumber: requireCount(source, "revisionNumber", path),
    action: requireEnum(source, "action", path, assistPermissionActions),
    analyst: requireString(source, "analyst", path),
    recordedAt: requireString(source, "recordedAt", path),
  };
};

/** `AssistProviderPermission` を検証する。 */
export const decodeAssistProviderPermission: Decoder<
  AssistProviderPermission
> = (input, path) => {
  const source = readObject(input, path);
  return {
    provider: requireEnum(source, "provider", path, assistProviders),
    permitted: requireBoolean(source, "permitted", path),
    revisions: requireArray(
      source,
      "revisions",
      path,
      decodeAssistPermissionRevision,
    ),
  };
};

/** `AssistPermissionsResponse` を検証する。 */
export const decodeAssistPermissionsResponse: Decoder<
  AssistPermissionsResponse
> = (input, path) => {
  const source = readObject(input, path);
  return {
    providers: requireArray(
      source,
      "providers",
      path,
      decodeAssistProviderPermission,
    ),
  };
};
