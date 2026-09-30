import {
  DecodeFailure,
  type Decoder,
  decodeString,
  optionalArray,
  optionalString,
  readObject,
  requireArray,
  requireEnum,
  requireString,
} from "./decoding";

/** 自分がそのワークスペースに持つ権限。所有者、編集できる共有先、閲覧だけの共有先。 */
export type WorkspaceAccess = "owner" | "edit" | "view";

/** 共有先に与える権限。 */
export type WorkspaceShareAccess = Exclude<WorkspaceAccess, "owner">;

const workspaceAccesses: readonly WorkspaceAccess[] = ["owner", "edit", "view"];
const shareAccesses: readonly WorkspaceShareAccess[] = ["view", "edit"];

/** ワークスペースの一覧の 1 件。定義元は `backend/api/workspaces.go` の `workspaceSummary` である。 */
export type WorkspaceSummary = {
  id: string;
  name: string;
  /** 所有者のログイン名。 */
  owner: string;
  access: WorkspaceAccess;
  revision: number;
  updatedAt: string;
  updatedBy: string;
};

/** ワークスペースの共有先 1 件。 */
export type WorkspaceShare = {
  login: string;
  access: WorkspaceShareAccess;
  grantedBy: string;
  grantedAt: string;
};

/**
 * ワークスペース 1 件。state は画面が保存した JSON の object のままである。shares は所有者が
 * 読んだときだけ持つ。
 */
export type WorkspaceItem = WorkspaceSummary & {
  state: unknown;
  shares?: WorkspaceShare[];
};

/** `WorkspaceShare` を検証する。 */
export const decodeWorkspaceShare: Decoder<WorkspaceShare> = (input, path) => {
  const source = readObject(input, path);
  return {
    login: requireString(source, "login", path),
    access: requireEnum(source, "access", path, shareAccesses),
    grantedBy: requireString(source, "grantedBy", path),
    grantedAt: requireString(source, "grantedAt", path),
  };
};

function requireRevision(source: Record<string, unknown>, path: string) {
  const value = source.revision;
  if (typeof value !== "number" || !Number.isSafeInteger(value)) {
    throw new DecodeFailure(`${path}.revision`, "expected a safe integer");
  }
  return value;
}

/** `WorkspaceSummary` を検証する。 */
export const decodeWorkspaceSummary: Decoder<WorkspaceSummary> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    id: requireString(source, "id", path),
    name: requireString(source, "name", path),
    owner: requireString(source, "owner", path),
    access: requireEnum(source, "access", path, workspaceAccesses),
    revision: requireRevision(source, path),
    updatedAt: requireString(source, "updatedAt", path),
    updatedBy: requireString(source, "updatedBy", path),
  };
};

/** `WorkspaceItem` を検証する。state の中身は `decodeWorkspaceState` が読む。 */
export const decodeWorkspaceItem: Decoder<WorkspaceItem> = (input, path) => {
  const source = readObject(input, path);
  readObject(source.state, `${path}.state`);
  const shares = optionalArray(source, "shares", path, decodeWorkspaceShare);
  return {
    ...decodeWorkspaceSummary(input, path),
    state: source.state,
    ...(shares === undefined ? {} : { shares }),
  };
};

/** 状態の最上位の欄を置き換える差分。値の null は欄を消すことを表す。 */
export type WorkspacePatch = Record<string, unknown>;

/** 変更 1 件を記録した結果。 */
export type WorkspaceChangeResult = {
  revision: number;
  clientChangeId: string;
  updatedBy: string;
};

/**
 * 変更が競合した相手の記録。fields が `["*"]` のとき、全体が競合した。state は競合を判定した
 * 時点の最新の状態である。
 */
export type WorkspaceConflict = {
  fields: string[];
  revision: number;
  state: Record<string, unknown>;
  updatedBy: string;
};

/** 同じワークスペースを開いている接続 1 件。selection は接続が送った JSON のままである。 */
export type WorkspaceConnection = {
  connectionId: string;
  login: string;
  displayName: string;
  selection: unknown;
  following: string | null;
};

/** ワークスペースの配信が含む事象。 */
export type WorkspaceEvent =
  | {
      type: "snapshot";
      revision: number;
      state: Record<string, unknown>;
      updatedBy: string;
      name: string;
    }
  | {
      type: "change";
      revision: number;
      fields: string[];
      patch: WorkspacePatch;
      /** 変更した利用者のログイン名。 */
      actor: string;
      clientChangeId?: string;
      /** fields が `["*"]` の変更が持つ、ワークスペースの名前。 */
      name?: string;
    }
  | { type: "presence"; connections: WorkspaceConnection[] }
  | { type: "closed"; reason: "deleted" | "revoked" };

/** `WorkspaceChangeResult` を検証する。 */
export const decodeWorkspaceChangeResult: Decoder<WorkspaceChangeResult> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    revision: requireRevision(source, path),
    clientChangeId: requireString(source, "clientChangeId", path),
    updatedBy: requireString(source, "updatedBy", path),
  };
};

/** `WorkspaceConflict` を検証する。 */
export const decodeWorkspaceConflict: Decoder<WorkspaceConflict> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    fields: requireArray(source, "fields", path, decodeString),
    revision: requireRevision(source, path),
    state: readObject(source.state, `${path}.state`),
    updatedBy: requireString(source, "updatedBy", path),
  };
};

/** 変更した利用者は、ログイン名の文字列か、login を持つ object で届く。 */
function requireActor(source: Record<string, unknown>, path: string): string {
  const actor = source.actor;
  if (typeof actor === "string") return actor;
  return requireString(readObject(actor, `${path}.actor`), "login", path);
}

const decodeConnection: Decoder<WorkspaceConnection> = (input, path) => {
  const source = readObject(input, path);
  const following = source.following;
  return {
    connectionId: requireString(source, "connectionId", path),
    login: requireString(source, "login", path),
    displayName: requireString(source, "displayName", path),
    selection: source.selection ?? null,
    following:
      following === undefined || following === null
        ? null
        : requireString(source, "following", path),
  };
};

/** 配信の事象 1 件を検証する。 */
export const decodeWorkspaceEvent: Decoder<WorkspaceEvent> = (input, path) => {
  const source = readObject(input, path);
  switch (
    requireEnum(source, "type", path, [
      "snapshot",
      "change",
      "presence",
      "closed",
    ])
  ) {
    case "snapshot":
      return {
        type: "snapshot",
        revision: requireRevision(source, path),
        state: readObject(source.state, `${path}.state`),
        updatedBy: requireString(source, "updatedBy", path),
        name: requireString(source, "name", path),
      };
    case "change":
      return {
        type: "change",
        revision: requireRevision(source, path),
        fields: requireArray(source, "fields", path, decodeString),
        patch: readObject(source.patch, `${path}.patch`),
        actor: requireActor(source, path),
        clientChangeId: optionalString(source, "clientChangeId", path),
        name: optionalString(source, "name", path),
      };
    case "presence":
      return {
        type: "presence",
        connections: requireArray(
          source,
          "connections",
          path,
          decodeConnection,
        ),
      };
    case "closed":
      return {
        type: "closed",
        reason: requireEnum(source, "reason", path, ["deleted", "revoked"]),
      };
  }
};

/** ワークスペースの一覧 (`workspacesResponse`) を検証する。 */
export const decodeWorkspaceSummaries: Decoder<WorkspaceSummary[]> = (
  input,
  path,
) =>
  requireArray(
    readObject(input, path),
    "workspaces",
    path,
    decodeWorkspaceSummary,
  );
