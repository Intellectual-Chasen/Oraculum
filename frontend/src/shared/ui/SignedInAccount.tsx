import { createContext, useContext } from "react";
import type { MemberRole } from "../contracts/members";
import type { SessionAccount } from "../contracts/session";
import type { FetchFailure } from "../lib/fetchState";
import { toVisibleRawText } from "../lib/rawText";
import { Hint } from "./Hint";
import { RawText } from "./RawText";
import { TextField } from "./TextField";

/** ログインしている利用者と、その利用者の調査の役割と、ログアウトの操作。 */
export type SignedIn = {
  account: SessionAccount;
  role: MemberRole;
  /** ログアウトする。失敗したときは失敗を返す。 */
  logout: () => Promise<FetchFailure | undefined>;
};

/**
 * ログインしている利用者。アカウントを持たずに起動した server では `undefined` であり、
 * 分析者の名前を記録する欄は分析者が入力した名前を送る。
 */
export const SignedInContext = createContext<SignedIn | undefined>(undefined);

/** ログインしている利用者とログアウトの操作を返す。アカウントを持たない起動では `undefined` を返す。 */
export function useSignedIn(): SignedIn | undefined {
  return useContext(SignedInContext);
}

/** ログインしている利用者を返す。アカウントを持たない起動では `undefined` を返す。 */
export function useSignedInAccount(): SessionAccount | undefined {
  return useSignedIn()?.account;
}

/**
 * 記録と段階の開始を行えるかを返す。閲覧者の役割だけが行えない。アカウントを持たない起動では
 * 役割が無く、行える。server も役割の足りない要求を退ける。
 */
export function useCanWrite(): boolean {
  return useSignedIn()?.role !== "viewer";
}

/** 閲覧者の役割で記録の操作を止めた理由の短いラベル。 */
export const viewerCannotRecord = "閲覧者は記録不可";

/** 利用者を表示名で描き、ログイン名を title に入れる。 */
export function AccountName({ account }: { account: SessionAccount }) {
  return (
    <Hint text={`ログイン名: ${toVisibleRawText(account.login)}`}>
      <RawText text={account.displayName} />
    </Hint>
  );
}

type AuthorFieldProps = {
  id: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
};

/**
 * 記録する分析者の欄。ログインしているときは入力欄を出さず、server が著者にする利用者を出す。
 * アカウントを持たない起動では分析者の名前の入力欄を出す。
 */
export function AuthorField({ id, label, value, onChange }: AuthorFieldProps) {
  const account = useSignedInAccount();
  if (account !== undefined) {
    return (
      <p>
        分析者: <AccountName account={account} />
      </p>
    );
  }
  return <TextField id={id} label={label} value={value} onChange={onChange} />;
}
