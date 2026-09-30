import { expect, test } from "vitest";
import type { GraphNode } from "@/shared/contracts/graph";
import { nodeKeyFormLabelOf, nodeKindLabelOf } from "./labels";

test("生成の記録が無い PID の区間は、区間の時刻を最初に記録した時刻と書く", () => {
  const keyForm = "terminal_id_process_pid_interval";
  expect(nodeKeyFormLabelOf({ keyForm, creationRecord: "present" })).toBe(
    "端末の ID と PID と、その PID が使われ始めた時刻",
  );
  expect(nodeKeyFormLabelOf({ keyForm, creationRecord: "absent" })).toBe(
    "端末の ID と PID と、その PID を最初に記録した時刻",
  );
  expect(
    nodeKeyFormLabelOf({ keyForm: "terminal_id", creationRecord: "absent" }),
  ).toBe("端末の ID");
});

const dollarName = "$ で終わる名前のアカウント";

function account(
  identity: GraphNode["identity"],
  label: GraphNode["label"] | string,
): Pick<GraphNode, "kind" | "identity" | "label"> {
  return {
    kind: "account",
    identity,
    label:
      typeof label === "string"
        ? { rawText: label, valueState: "present" }
        : label,
  };
}

test("`$` で終わる名前のアカウントを、名前の形で見分けて出す", () => {
  expect(
    nodeKindLabelOf(
      account(
        [
          { semantic: "account.domain", value: "EXAMPLE" },
          { semantic: "account.name", value: "HOST-01$" },
        ],
        "HOST-01$",
      ),
    ),
  ).toBe(dollarName);
  // SID で識別したアカウントは表示名で見分ける。
  expect(
    nodeKindLabelOf(
      account(
        [{ semantic: "account.sid", value: "S-1-5-21-1-2-3-1001" }],
        "HOST-02$",
      ),
    ),
  ).toBe(dollarName);
  // 表示名は normalized を rawText より先に読む。
  expect(
    nodeKindLabelOf(
      account([{ semantic: "account.sid", value: "S-1-5-21-1-2-3-1002" }], {
        rawText: "host-03",
        normalized: "HOST-03$",
        valueState: "present",
      }),
    ),
  ).toBe(dollarName);
  // account.name と表示名のどちらかが `$` で終われば見分ける。
  expect(
    nodeKindLabelOf(
      account(
        [
          { semantic: "account.domain", value: "EXAMPLE" },
          { semantic: "account.name", value: "HOST-04$" },
        ],
        "HOST-04",
      ),
    ),
  ).toBe(dollarName);
  expect(
    nodeKindLabelOf(
      account(
        [
          { semantic: "account.domain", value: "EXAMPLE" },
          { semantic: "account.name", value: "user05" },
        ],
        "user05$",
      ),
    ),
  ).toBe(dollarName);
  expect(
    nodeKindLabelOf(
      account(
        [
          { semantic: "account.domain", value: "EXAMPLE" },
          { semantic: "account.name", value: "user01" },
        ],
        "user01",
      ),
    ),
  ).toBe("アカウント");
  expect(
    nodeKindLabelOf({
      kind: "file",
      identity: [{ semantic: "file.path", value: "C:\\Example\\a$" }],
      label: { rawText: "C:\\Example\\a$", valueState: "present" },
    }),
  ).toBe("ファイル");
});
