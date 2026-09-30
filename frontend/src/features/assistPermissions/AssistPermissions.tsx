import { ShieldCheck, ShieldOff } from "lucide-react";
import { useId, useState } from "react";
import type {
  AssistPermissionAction,
  AssistProvider,
  AssistProviderPermission,
} from "@/shared/contracts/assistPermissions";
import { ConfirmIconButton } from "@/shared/ui/ConfirmIconButton";
import { DataTable, type DataTableColumn } from "@/shared/ui/DataTable";
import { FetchFailureNotice } from "@/shared/ui/FetchFailureNotice";
import { HelpPopover } from "@/shared/ui/HelpPopover";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { RawText } from "@/shared/ui/RawText";
import { AuthorField, useSignedInAccount } from "@/shared/ui/SignedInAccount";
import { StatusLabel } from "@/shared/ui/StatusLabel";
import type { AssistPermissionsView } from "./useAssistPermissions";

/** 提供者の表示名。 */
const providerLabels: Record<AssistProvider, string> = {
  claude: "Claude",
};

/** 提供者の表示名の補足。名前の横の help に出す。 */
const providerDetails: Record<AssistProvider, string> = {
  claude: "分析者本人がログインした claude CLI",
};

/** 改訂が記録した操作の表示。 */
const actionLabels: Record<AssistPermissionAction, string> = {
  grant: "許可",
  revoke: "取消",
};

/** 送信の許可が何を決めるか。見出しの help に「名前: 値」の組で出す。 */
const introPairs = [
  { name: "送信先", value: "提供者ごとの外部の AI" },
  { name: "許可なし", value: "会話の開始不可、AI による証拠の参照不可" },
  { name: "会話中の取消", value: "以後の受け渡しをサーバーが拒否" },
  { name: "記録", value: "許可と取消を改訂として保存" },
];

/**
 * 提供者ごとに、証拠を外部の AI へ送る許可を出し、取り消す表。
 *
 * **中継の有無に関わらず出す。** 許可はチームで共有するサーバーの記録であり、中継を起動して
 * いない分析者も取り消せる。**許可と取消は改訂として残り、取り消せないため、確認の dialog を
 * 経て記録する。**
 */
export function AssistPermissions({ view }: { view: AssistPermissionsView }) {
  const analystInputId = useId();
  const account = useSignedInAccount();
  const [analyst, setAnalyst] = useState("");

  const body = (() => {
    switch (view.state.status) {
      case "loading":
        return <p role="status">送信の許可の読み込み中</p>;
      case "unavailable":
        return (
          <KeyValueList
            pairs={[
              {
                name: "状態",
                value: (
                  <StatusLabel
                    status="idle"
                    label="使用不可"
                    details={
                      <KeyValueList
                        stacked
                        pairs={[
                          { name: "起動の指定", value: "--investigation" },
                        ]}
                      />
                    }
                  />
                ),
              },
            ]}
          />
        );
      case "failed":
        return <FetchFailureNotice failure={view.state.failure} />;
      case "loaded": {
        const record = (
          provider: AssistProvider,
          action: AssistPermissionAction,
        ) =>
          view.record({
            provider,
            action,
            analyst: account === undefined ? analyst : undefined,
          });
        const columns: DataTableColumn<AssistProviderPermission>[] = [
          {
            key: "provider",
            header: "提供者",
            rowHeader: true,
            cell: (permission) => (
              <>
                {providerLabels[permission.provider]}
                <HelpPopover label={providerLabels[permission.provider]}>
                  {providerDetails[permission.provider]}
                </HelpPopover>
              </>
            ),
          },
          {
            key: "state",
            header: "状態",
            cell: (permission) =>
              permission.permitted ? (
                <StatusLabel status="done" label="許可中" />
              ) : (
                <StatusLabel status="idle" label="未許可" />
              ),
          },
          {
            key: "action",
            header: "操作",
            cell: (permission) => (
              <ProviderAction
                permission={permission}
                disabled={view.recording}
                onRecord={(action) => record(permission.provider, action)}
              />
            ),
          },
        ];
        return (
          <>
            <AuthorField
              id={analystInputId}
              label="分析者の名前"
              value={analyst}
              onChange={setAnalyst}
            />
            <DataTable
              label="提供者"
              columns={columns}
              rows={view.state.providers}
              rowKey={(permission) => permission.provider}
            />
            {view.recordFailure === undefined ? null : (
              <FetchFailureNotice failure={view.recordFailure} />
            )}
            {view.state.providers.map((permission) => (
              <Revisions key={permission.provider} permission={permission} />
            ))}
          </>
        );
      }
      default: {
        const exhaustive: never = view.state;
        throw new Error(`unknown state: ${JSON.stringify(exhaustive)}`);
      }
    }
  })();

  return (
    <section aria-label="AI 支援の送信の許可">
      <div className="flex items-center gap-1">
        <h2>AI 支援の送信の許可</h2>
        <HelpPopover label="AI 支援の送信の許可">
          <KeyValueList stacked pairs={introPairs} />
        </HelpPopover>
      </div>
      {body}
    </section>
  );
}

/** 提供者 1 つの許可か取消の操作。どちらも確認の dialog を経て記録する。 */
function ProviderAction({
  permission,
  disabled,
  onRecord,
}: {
  permission: AssistProviderPermission;
  disabled: boolean;
  onRecord: (action: AssistPermissionAction) => void;
}) {
  const details = (
    <KeyValueList
      stacked
      pairs={[{ name: "提供者", value: providerLabels[permission.provider] }]}
    />
  );
  const disabledReason = { title: "記録中", text: "記録の完了を待機" };
  return permission.permitted ? (
    <ConfirmIconButton
      label="送信の許可を取消"
      details={details}
      isDisabled={disabled}
      disabledReason={disabledReason}
      onConfirm={() => onRecord("revoke")}
    >
      <ShieldOff size={14} aria-hidden="true" />
    </ConfirmIconButton>
  ) : (
    <ConfirmIconButton
      label="送信を許可"
      details={details}
      isDisabled={disabled}
      disabledReason={disabledReason}
      onConfirm={() => onRecord("grant")}
    >
      <ShieldCheck size={14} aria-hidden="true" />
    </ConfirmIconButton>
  );
}

const revisionColumns: DataTableColumn<
  AssistProviderPermission["revisions"][number]
>[] = [
  {
    key: "number",
    header: "番号",
    numeric: true,
    cell: (revision) => revision.revisionNumber,
  },
  {
    key: "action",
    header: "操作",
    cell: (revision) => actionLabels[revision.action],
  },
  {
    key: "analyst",
    header: "分析者",
    cell: (revision) => <RawText text={revision.analyst} />,
  },
  {
    key: "recordedAt",
    header: "時刻",
    mono: true,
    cell: (revision) => revision.recordedAt,
  },
];

/** 提供者 1 つの改訂の履歴。 */
function Revisions({ permission }: { permission: AssistProviderPermission }) {
  const label = `${providerLabels[permission.provider]} の改訂`;
  return (
    <section aria-label={label}>
      {permission.revisions.length === 0 ? (
        <KeyValueList pairs={[{ name: label, value: "0" }]} />
      ) : (
        <DataTable
          label={label}
          showCaption
          columns={revisionColumns}
          rows={permission.revisions}
          rowKey={(revision) => String(revision.revisionNumber)}
        />
      )}
    </section>
  );
}
