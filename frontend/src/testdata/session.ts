import type { MemberRole } from "@/shared/contracts/members";
import type { SignedIn } from "@/shared/ui/SignedInAccount";

/** アカウントを持つ起動でログインした利用者の `GET /api/v0/session/me` の応答。 */
export function accountSessionJson() {
  return { authentication: "account", login: "alice", displayName: "石橋" };
}

/** ログインした利用者の欄に出る表示名。ログイン名は title に入る。 */
export const accountLabel = "石橋";

/** 記録の欄の test が `SignedInContext` へ渡す、編集者の役割でログインした利用者。 */
export const signedInAlice: SignedIn = {
  account: { login: "alice", displayName: "石橋" },
  role: "editor",
  logout: async () => undefined,
};

/** 閲覧者の役割でログインした利用者。 */
export const signedInViewer: SignedIn = { ...signedInAlice, role: "viewer" };

/** `GET /api/v0/members/me` と一覧の要素の応答。 */
export function memberJson(
  role: MemberRole,
  login = "alice",
  displayName = "石橋",
) {
  return {
    login,
    displayName,
    role,
    grantedBy: "root",
    grantedAt: "2026-09-01T00:00:00Z",
  };
}
