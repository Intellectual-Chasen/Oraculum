import { UserMinus, UserPlus } from "lucide-react";
import { useEffect, useId, useState } from "react";
import {
  fetchWorkspace,
  shareWorkspace,
  unshareWorkspace,
} from "@/shared/api/workspaces";
import type {
  WorkspaceShare,
  WorkspaceShareAccess,
} from "@/shared/contracts/workspaces";
import type { FetchFailure } from "@/shared/lib/fetchState";
import { DataTable } from "@/shared/ui/DataTable";
import { FetchFailureNotice } from "@/shared/ui/FetchFailureNotice";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { RawText } from "@/shared/ui/RawText";

/** 共有先の権限の表示名。 */
export const shareAccessLabels: Record<WorkspaceShareAccess, string> = {
  view: "閲覧のみ",
  edit: "編集",
};

function AccessSelect({
  id,
  label,
  value,
  onChange,
}: {
  id?: string;
  label?: string;
  value: WorkspaceShareAccess;
  onChange: (access: WorkspaceShareAccess) => void;
}) {
  return (
    <select
      id={id}
      aria-label={label}
      value={value}
      onChange={(event) =>
        onChange(event.target.value === "edit" ? "edit" : "view")
      }
    >
      <option value="view">{shareAccessLabels.view}</option>
      <option value="edit">{shareAccessLabels.edit}</option>
    </select>
  );
}

/** 所有者がワークスペースの共有先を足し、権限を変え、取り消す欄。 */
export function WorkspaceShares({
  workspaceId,
  workspaceName,
}: {
  workspaceId: string;
  workspaceName: string;
}) {
  const fieldId = useId();
  const [shares, setShares] = useState<WorkspaceShare[]>();
  const [failure, setFailure] = useState<FetchFailure>();
  const [login, setLogin] = useState("");
  const [access, setAccess] = useState<WorkspaceShareAccess>("view");

  useEffect(() => {
    let active = true;
    void fetchWorkspace(workspaceId).then((result) => {
      if (!active) return;
      if (result.ok) setShares(result.value.shares ?? []);
      else setFailure(result.failure);
    });
    return () => {
      active = false;
    };
  }, [workspaceId]);

  const grant = async (target: string, next: WorkspaceShareAccess) => {
    setFailure(undefined);
    const result = await shareWorkspace(workspaceId, target, next);
    if (!result.ok) {
      setFailure(result.failure);
      return false;
    }
    setShares((items = []) => [
      ...items.filter((item) => item.login !== target),
      result.value,
    ]);
    return true;
  };

  const revoke = async (target: string) => {
    setFailure(undefined);
    const result = await unshareWorkspace(workspaceId, target);
    // 既に取り消されていた共有も、一覧から外す。
    if (result.ok || result.failure.failureCode === "record_not_found") {
      setShares((items = []) => items.filter((item) => item.login !== target));
    }
    if (!result.ok) setFailure(result.failure);
  };

  return (
    <section aria-label={`${workspaceName} の共有`}>
      {failure === undefined ? null : <FetchFailureNotice failure={failure} />}
      <form
        onSubmit={(event) => {
          event.preventDefault();
          void grant(login.trim(), access).then((done) => {
            if (done) setLogin("");
          });
        }}
      >
        <label htmlFor={`${fieldId}-login`}>共有先のログイン名</label>
        <input
          id={`${fieldId}-login`}
          value={login}
          onChange={(event) => setLogin(event.target.value)}
        />
        <label htmlFor={`${fieldId}-access`}>権限</label>
        <AccessSelect
          id={`${fieldId}-access`}
          value={access}
          onChange={setAccess}
        />
        <IconButton
          type="submit"
          label="共有先に追加"
          isDisabled={login.trim() === ""}
          disabledReason={{
            title: "ログイン名なし",
            text: "ログイン名の入力が必要",
          }}
        >
          <UserPlus size={14} aria-hidden="true" />
        </IconButton>
      </form>
      {shares === undefined ? (
        failure === undefined ? (
          <p role="status">共有先の読み込み中</p>
        ) : null
      ) : shares.length === 0 ? (
        <KeyValueList pairs={[{ name: "共有先", value: "0" }]} />
      ) : (
        <DataTable
          label="共有先"
          rows={shares}
          rowKey={(share) => share.login}
          rowProps={(share) => ({ "aria-label": share.login })}
          columns={[
            {
              key: "login",
              header: "ログイン名",
              rowHeader: true,
              sortValue: (share) => share.login,
              cell: (share) => <RawText text={share.login} />,
            },
            {
              key: "access",
              header: "権限",
              sortValue: (share) => shareAccessLabels[share.access],
              cell: (share) => (
                <AccessSelect
                  label={`${share.login} の権限`}
                  value={share.access}
                  onChange={(next) => void grant(share.login, next)}
                />
              ),
            },
            {
              key: "actions",
              header: "操作",
              cell: (share) => (
                <IconButton
                  label="共有を解除"
                  onPress={() => void revoke(share.login)}
                >
                  <UserMinus size={14} aria-hidden="true" />
                </IconButton>
              ),
            },
          ]}
        />
      )}
    </section>
  );
}
