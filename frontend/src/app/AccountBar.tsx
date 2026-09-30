import { LogOut } from "lucide-react";
import { useState } from "react";
import type { FetchFailure } from "@/shared/lib/fetchState";
import { FetchFailureNotice } from "@/shared/ui/FetchFailureNotice";
import { IconButton } from "@/shared/ui/IconButton";
import {
  AccountName,
  type SignedIn,
  useSignedIn,
} from "@/shared/ui/SignedInAccount";

/**
 * ヘッダーに置く、ログインしている利用者とログアウトのボタン。アカウントを持たない起動では
 * 何も出さない。ログアウトが失敗したときは失敗を出し、ログインしている状態を保つ。
 *
 * `account` を渡すと、渡した利用者を出す。調査の役割を持たない利用者の画面が使う。
 */
export function AccountBar({
  account,
}: {
  account?: Pick<SignedIn, "account" | "logout">;
}) {
  const fromContext = useSignedIn();
  const signedIn = account ?? fromContext;
  const [failure, setFailure] = useState<FetchFailure | undefined>(undefined);
  const [pending, setPending] = useState(false);
  if (signedIn === undefined) {
    return null;
  }
  return (
    <div className="account-bar">
      <span>
        ログイン中: <AccountName account={signedIn.account} />
      </span>
      <IconButton
        label="ログアウト"
        isDisabled={pending}
        onPress={async () => {
          setPending(true);
          const result = await signedIn.logout();
          setFailure(result);
          setPending(false);
        }}
      >
        <LogOut size={14} aria-hidden="true" />
      </IconButton>
      {failure === undefined ? null : <FetchFailureNotice failure={failure} />}
    </div>
  );
}
