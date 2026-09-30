import {
  decodeWorkspaceChangeResult,
  decodeWorkspaceEvent,
  decodeWorkspaceItem,
  decodeWorkspaceShare,
  decodeWorkspaceSummaries,
  type WorkspaceChangeResult,
  type WorkspaceEvent,
  type WorkspaceItem,
  type WorkspacePatch,
  type WorkspaceShare,
  type WorkspaceShareAccess,
  type WorkspaceSummary,
} from "../contracts/workspaces";
import { readApiBaseUrl } from "./config";
import { type ApiResult, requestJson } from "./httpClient";

function workspacePath(id: string) {
  return `/workspaces/${encodeURIComponent(id)}`;
}

/** 自分のワークスペースと、自分に共有されたワークスペースの一覧 (`GET /api/v0/workspaces`) を取得する。 */
export async function fetchWorkspaces(): Promise<
  ApiResult<WorkspaceSummary[]>
> {
  return requestJson({
    path: "/workspaces",
    decode: decodeWorkspaceSummaries,
    failureSummary: "ワークスペースの一覧の取得",
  });
}

/** ワークスペース 1 件 (`GET /api/v0/workspaces/{id}`) を取得する。 */
export async function fetchWorkspace(
  id: string,
): Promise<ApiResult<WorkspaceItem>> {
  return requestJson({
    path: workspacePath(id),
    decode: decodeWorkspaceItem,
    failureSummary: "ワークスペースの取得",
  });
}

/**
 * ワークスペースを作る (`POST /api/v0/workspaces`)。name が undefined なら server が
 * 「ワークスペース N」の名前を付ける。
 */
export async function createWorkspace(
  name: string | undefined,
  state: unknown,
): Promise<ApiResult<WorkspaceItem>> {
  return requestJson({
    path: "/workspaces",
    method: "POST",
    body: name === undefined ? { state } : { name, state },
    decode: decodeWorkspaceItem,
    failureSummary: "ワークスペースの作成",
  });
}

/**
 * ワークスペースを置き換える (`PUT /api/v0/workspaces/{id}`)。baseRevision より後に別の画面が
 * 変更していれば `workspace_changed` で失敗する。keepalive はページを離れる間に送る要求に使う。
 */
export async function replaceWorkspace(
  id: string,
  body: { name: string; state: unknown; baseRevision: number },
  options: { keepalive?: boolean } = {},
): Promise<ApiResult<WorkspaceItem>> {
  return requestJson({
    path: workspacePath(id),
    method: "PUT",
    body,
    decode: decodeWorkspaceItem,
    failureSummary: "ワークスペースの保存",
    keepalive: options.keepalive,
  });
}

/**
 * 状態の最上位の欄を置き換える変更を記録する (`POST /api/v0/workspaces/{id}/changes`)。
 * 同じ clientChangeId の再送には前の結果が返る。baseRevision より後に別の画面が同じ欄を
 * 変更していれば `workspace_changed` で失敗し、失敗の conflict が競合の記録を持つ。
 */
export async function postWorkspaceChange(
  id: string,
  body: { clientChangeId: string; baseRevision: number; patch: WorkspacePatch },
  options: { keepalive?: boolean } = {},
): Promise<ApiResult<WorkspaceChangeResult>> {
  return requestJson({
    path: `${workspacePath(id)}/changes`,
    method: "POST",
    body,
    decode: decodeWorkspaceChangeResult,
    failureSummary: "ワークスペースの保存",
    keepalive: options.keepalive,
  });
}

/** 自分の接続の選択と追従先を知らせる (`POST /api/v0/workspaces/{id}/presence`)。 */
export async function postWorkspacePresence(
  id: string,
  body: { connectionId: string; selection: unknown; following: string | null },
): Promise<ApiResult<undefined>> {
  return requestJson({
    path: `${workspacePath(id)}/presence`,
    method: "POST",
    body,
    decode: () => undefined,
    failureSummary: "選択と追従先のサーバーへの送信",
  });
}

/**
 * ワークスペースの配信 (`GET /api/v0/workspaces/{id}/events`) を購読し、読めた事象を onEvent へ
 * 渡す。読めない事象は捨てる。切れた接続はブラウザーがつなぎ直し、server が snapshot から
 * 送り直す。返す関数を呼ぶと購読をやめる。EventSource を持たない実行環境では何もしない。
 */
export function subscribeWorkspaceEvents(
  id: string,
  connectionId: string,
  onEvent: (event: WorkspaceEvent) => void,
): () => void {
  if (typeof EventSource === "undefined") return () => undefined;
  const source = new EventSource(
    `${readApiBaseUrl()}${workspacePath(id)}/events?${new URLSearchParams({ connectionId })}`,
  );
  source.onmessage = (message: MessageEvent) => {
    let event: WorkspaceEvent;
    try {
      event = decodeWorkspaceEvent(JSON.parse(`${message.data}`), "event");
    } catch {
      return;
    }
    // closed の後に server は接続を閉じる。つなぎ直さない。
    if (event.type === "closed") source.close();
    onEvent(event);
  };
  return () => source.close();
}

function sharePath(id: string, login: string) {
  return `${workspacePath(id)}/shares/${encodeURIComponent(login)}`;
}

/** 共有先を足すか、共有先の権限を変える (`PUT /api/v0/workspaces/{id}/shares/{login}`)。所有者だけが実行できる。 */
export async function shareWorkspace(
  id: string,
  login: string,
  access: WorkspaceShareAccess,
): Promise<ApiResult<WorkspaceShare>> {
  return requestJson({
    path: sharePath(id, login),
    method: "PUT",
    body: { access },
    decode: decodeWorkspaceShare,
    failureSummary: "ワークスペースの共有の記録",
  });
}

/** 共有を取り消す (`DELETE /api/v0/workspaces/{id}/shares/{login}`)。 */
export async function unshareWorkspace(
  id: string,
  login: string,
): Promise<ApiResult<undefined>> {
  return requestJson({
    path: sharePath(id, login),
    method: "DELETE",
    decode: () => undefined,
    failureSummary: "ワークスペースの共有の取り消し",
  });
}

/** ワークスペースを削除する (`DELETE /api/v0/workspaces/{id}`)。 */
export async function deleteWorkspace(
  id: string,
): Promise<ApiResult<undefined>> {
  return requestJson({
    path: workspacePath(id),
    method: "DELETE",
    decode: () => undefined,
    failureSummary: "ワークスペースの削除",
  });
}
