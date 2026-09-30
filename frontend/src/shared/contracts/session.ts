import {
  type Decoder,
  readObject,
  rejectMember,
  requireEnum,
  requireString,
} from "./decoding";

/** ログインしている利用者。 */
export type SessionAccount = {
  login: string;
  displayName: string;
};

/**
 * 画面を開いた利用者のセッション。定義元は `backend/api/session.go` の `sessionResponse` である。
 * `none` はアカウントを持たずに起動した server であり、ログインせずに使える。
 */
export type Session =
  | { authentication: "none" }
  | ({ authentication: "account" } & SessionAccount);

const authentications = ["none", "account"] as const;

/** `Session` を検証する。`none` の応答はログイン名と表示名を持たない。 */
export const decodeSession: Decoder<Session> = (input, path) => {
  const source = readObject(input, path);
  const authentication = requireEnum(
    source,
    "authentication",
    path,
    authentications,
  );
  if (authentication === "none") {
    rejectMember(source, "login", path, "absent when authentication is none");
    rejectMember(
      source,
      "displayName",
      path,
      "absent when authentication is none",
    );
    return { authentication };
  }
  return {
    authentication,
    login: requireString(source, "login", path),
    displayName: requireString(source, "displayName", path),
  };
};
