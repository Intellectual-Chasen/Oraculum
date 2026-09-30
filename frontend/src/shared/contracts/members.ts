import {
  type Decoder,
  readObject,
  requireArray,
  requireEnum,
  requireString,
} from "./decoding";

/** 調査の役割。定義元は `backend/pipeline/access_store.go` の `Role` である。 */
export const memberRoles = ["viewer", "editor", "admin"] as const;
export type MemberRole = (typeof memberRoles)[number];

/**
 * 調査の役割を持つ利用者 1 人。定義元は `backend/api/access.go` の `memberItem` である。
 * `grantedBy` と `grantedAt` は空文字列を受け付ける。`members/me` の応答は、役割の一覧から
 * 利用者を探せないときに両方を空文字列にする。
 */
export type Member = {
  login: string;
  displayName: string;
  role: MemberRole;
  grantedBy: string;
  grantedAt: string;
};

/** `Member` を検証する。 */
export const decodeMember: Decoder<Member> = (input, path) => {
  const source = readObject(input, path);
  return {
    login: requireString(source, "login", path),
    displayName: requireString(source, "displayName", path),
    role: requireEnum(source, "role", path, memberRoles),
    grantedBy: requireString(source, "grantedBy", path),
    grantedAt: requireString(source, "grantedAt", path),
  };
};

/** 役割を持つ利用者の一覧 (`membersResponse`) を検証する。 */
export const decodeMembers: Decoder<Member[]> = (input, path) =>
  requireArray(readObject(input, path), "members", path, decodeMember);
