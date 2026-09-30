import {
  decodeMember,
  decodeMembers,
  type Member,
  type MemberRole,
} from "../contracts/members";
import { type ApiResult, requestJson } from "./httpClient";

/** ログインした利用者の役割 (`GET /api/v0/members/me`) を取得する。 */
export async function fetchMyMember(
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<Member>> {
  return requestJson({
    path: "/members/me",
    decode: decodeMember,
    failureSummary: "調査の役割の取得",
    signal: options.signal,
  });
}

/** 役割を持つ利用者の一覧 (`GET /api/v0/members`) を取得する。管理者だけが取得できる。 */
export async function fetchMembers(
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<Member[]>> {
  return requestJson({
    path: "/members",
    decode: decodeMembers,
    failureSummary: "利用者と役割の一覧の取得",
    signal: options.signal,
  });
}

/** 利用者に役割を与えるか、役割を変える (`PUT /api/v0/members/{login}`)。 */
export async function grantMemberRole(
  login: string,
  role: MemberRole,
): Promise<ApiResult<Member>> {
  return requestJson({
    path: `/members/${encodeURIComponent(login)}`,
    method: "PUT",
    body: { role },
    decode: decodeMember,
    failureSummary: "役割の記録",
  });
}

/** 利用者の役割を外す (`DELETE /api/v0/members/{login}`)。 */
export async function revokeMemberRole(
  login: string,
): Promise<ApiResult<undefined>> {
  return requestJson({
    path: `/members/${encodeURIComponent(login)}`,
    method: "DELETE",
    decode: () => undefined,
    failureSummary: "役割の削除",
  });
}
