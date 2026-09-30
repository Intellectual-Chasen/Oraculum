import { expect, test } from "vitest";
import type { EdgeKind } from "../contracts/graph";
import { edgeKindDescriptions, edgeKindLabels } from "./graphLabels";

// 推定で作るエッジの種類。ラベルが確定した関係に見えないよう、語尾に「候補」を付ける。
const inferredEdgeKinds: readonly EdgeKind[] = [
  "terminal_remote_session",
  "file_content_match",
  "cross_source_connection_match",
  "logon_session_operation",
  "argument_names_object",
  "task_registration_run",
  "linked_logon",
  "ticket_request_logon",
  "reverse_lookup_name",
  "connection_logon_match",
  "process_identity_match",
  "explicit_credential_logon",
  "unidentified_source_remote_session",
  "account_identity_match",
  "inbound_connection_match",
  "same_connection_match",
  "requested_session_logon",
  "logon_chain",
];

test.each(inferredEdgeKinds)(
  "推定のエッジの種類 %s のラベルは候補で終わる",
  (kind) => {
    expect(edgeKindLabels[kind]).toMatch(/の候補$/);
  },
);

test("逆引きの名前の補足は、ラベルに入れずに説明に持つ", () => {
  expect(edgeKindLabels.reverse_lookup_name).toBe("逆引きの名前の候補");
  expect(edgeKindDescriptions.reverse_lookup_name).toContain(
    "アドレスの持ち主",
  );
});
