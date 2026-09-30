import { UserMinus, UserPlus } from "lucide-react";
import { useEffect, useId, useState } from "react";
import type { ApiResult } from "@/shared/api/httpClient";
import {
  fetchMembers,
  grantMemberRole,
  revokeMemberRole,
} from "@/shared/api/members";
import {
  type Member,
  type MemberRole,
  memberRoles,
} from "@/shared/contracts/members";
import type { FetchFailure, FetchState } from "@/shared/lib/fetchState";
import { DataTable, type DataTableColumn } from "@/shared/ui/DataTable";
import { FetchFailureNotice } from "@/shared/ui/FetchFailureNotice";
import { FetchStateView } from "@/shared/ui/FetchStateView";
import { IconButton } from "@/shared/ui/IconButton";
import { MissingValue } from "@/shared/ui/MissingValue";
import { RawText } from "@/shared/ui/RawText";
import { useSignedInAccount } from "@/shared/ui/SignedInAccount";
import { StatusLabel } from "@/shared/ui/StatusLabel";

/** 役割の表示。 */
export const memberRoleLabels: Record<MemberRole, string> = {
  viewer: "閲覧者",
  editor: "編集者",
  admin: "管理者",
};

/** 自分の行の役割の変更と削除を止めた理由。 */
const ownRowReason = { title: "自分の役割", text: "ほかの管理者が変更" };

/** 記録の応答を待つ間に操作を止めた理由。 */
const pendingReason = { title: "記録中", text: "記録の完了を待機" };

function RoleSelect({
  id,
  label,
  value,
  disabled,
  describedBy,
  onChange,
}: {
  id?: string;
  label?: string;
  value: MemberRole;
  disabled: boolean;
  describedBy?: string;
  onChange: (role: MemberRole) => void;
}) {
  return (
    <select
      id={id}
      aria-label={label}
      aria-describedby={describedBy}
      value={value}
      disabled={disabled}
      onChange={(event) => {
        const role = memberRoles.find(
          (candidate) => candidate === event.target.value,
        );
        if (role !== undefined) {
          onChange(role);
        }
      }}
    >
      {memberRoles.map((role) => (
        <option key={role} value={role}>
          {memberRoleLabels[role]}
        </option>
      ))}
    </select>
  );
}

/**
 * 調査に参加する利用者と役割の一覧を出し、役割の変更・追加・取り外しを受け付ける。管理者だけに出す。
 *
 * 操作が成功するたびに一覧を取り直す。取り直す間は前の一覧を出し続ける。
 * ログインした利用者の行は、役割の変更と取り外しを止める。
 */
export function MemberRoles() {
  const fieldId = useId();
  const ownRowReasonId = `${fieldId}-own`;
  const ownLogin = useSignedInAccount()?.login;
  const [state, setState] = useState<FetchState<Member[]>>({
    status: "loading",
  });
  const [changedCount, setChangedCount] = useState(0);
  const [failure, setFailure] = useState<FetchFailure | undefined>(undefined);
  const [pending, setPending] = useState(false);
  const [newLogin, setNewLogin] = useState("");
  const [newRole, setNewRole] = useState<MemberRole>("viewer");

  // biome-ignore lint/correctness/useExhaustiveDependencies: changedCount が増えたときに一覧を取り直す。
  useEffect(() => {
    const controller = new AbortController();
    void fetchMembers({ signal: controller.signal }).then((result) => {
      if (controller.signal.aborted) {
        return;
      }
      setState(
        result.ok
          ? { status: "loaded", value: result.value }
          : { status: "failed", failure: result.failure },
      );
    });
    return () => controller.abort();
  }, [changedCount]);

  const run = async (action: () => Promise<ApiResult<unknown>>) => {
    setPending(true);
    const result = await action();
    setPending(false);
    if (!result.ok) {
      setFailure(result.failure);
      return false;
    }
    setFailure(undefined);
    setChangedCount((count) => count + 1);
    return true;
  };

  const grantedValue = (value: string) =>
    value === "" ? (
      <MissingValue description="記録なし" />
    ) : (
      <RawText text={value} />
    );
  const columns: DataTableColumn<Member>[] = [
    {
      key: "login",
      header: "ログイン名",
      rowHeader: true,
      cell: (member) => <RawText text={member.login} />,
    },
    {
      key: "displayName",
      header: "表示名",
      cell: (member) => <RawText text={member.displayName} />,
    },
    {
      key: "role",
      header: "役割",
      cell: (member) => {
        const own = member.login === ownLogin;
        return (
          <RoleSelect
            label={`${member.login} の役割`}
            value={member.role}
            disabled={pending || own}
            describedBy={own ? ownRowReasonId : undefined}
            onChange={(role) =>
              void run(() => grantMemberRole(member.login, role))
            }
          />
        );
      },
    },
    {
      key: "grantedBy",
      header: "付与した利用者",
      cell: (member) => grantedValue(member.grantedBy),
    },
    {
      key: "grantedAt",
      header: "付与した時刻",
      mono: true,
      cell: (member) => grantedValue(member.grantedAt),
    },
    {
      key: "action",
      header: "操作",
      cell: (member) => {
        const own = member.login === ownLogin;
        return (
          <IconButton
            label="役割を削除"
            accessibleName={`${member.login} の役割を削除`}
            isDisabled={pending || own}
            disabledReason={own ? ownRowReason : pendingReason}
            onPress={() => void run(() => revokeMemberRole(member.login))}
          >
            <UserMinus size={14} aria-hidden="true" />
          </IconButton>
        );
      },
    },
  ];

  return (
    <section aria-labelledby={`${fieldId}-heading`} className="view-pane">
      <h2 id={`${fieldId}-heading`}>利用者と役割</h2>
      {failure === undefined ? null : <FetchFailureNotice failure={failure} />}
      {pending ? (
        <p role="status">
          <StatusLabel status="running" label="記録中" />
        </p>
      ) : null}
      <span id={ownRowReasonId} className="sr-only">
        {`${ownRowReason.title}: ${ownRowReason.text}`}
      </span>
      <FetchStateView state={state} loadingDescription="利用者の読み込み中">
        {(members) => (
          <DataTable
            label="利用者"
            columns={columns}
            rows={members}
            rowKey={(member) => member.login}
          />
        )}
      </FetchStateView>
      <form
        onSubmit={async (event) => {
          event.preventDefault();
          if (await run(() => grantMemberRole(newLogin, newRole))) {
            setNewLogin("");
          }
        }}
      >
        <fieldset disabled={pending} className="flex items-end gap-2">
          <legend className="sr-only">利用者の追加</legend>
          <p>
            <label htmlFor={`${fieldId}-login`}>ログイン名</label>
            <input
              id={`${fieldId}-login`}
              required
              value={newLogin}
              onChange={(event) => setNewLogin(event.target.value)}
            />
          </p>
          <p>
            <label htmlFor={`${fieldId}-role`}>役割</label>
            <RoleSelect
              id={`${fieldId}-role`}
              value={newRole}
              disabled={pending}
              onChange={setNewRole}
            />
          </p>
          <IconButton type="submit" label="利用者を追加">
            <UserPlus size={14} aria-hidden="true" />
          </IconButton>
        </fieldset>
      </form>
    </section>
  );
}
