import { expect, test } from "vitest";
import { type LoadingFormRow, loadingDraftOf } from "./loadingRows";

const row: LoadingFormRow = {
  originPath: "logs/proxy.log",
  formatCandidates: ["squid_logformat"],
  formatKey: "squid_logformat",
  formatSpec: "combined",
  terminalId: "",
  terminalHostname: "proxy.example.test",
  terminalIp: "",
};

test("固定形式へ切り替えると以前の独自書式を送信せず、再編集用の入力を保持する", () => {
  for (const formatKey of [
    "squid_combined",
    "apache_access_combined",
    "infotrace_mark_ii",
  ]) {
    const switched = { ...row, formatKey };
    expect(loadingDraftOf(switched, "").formatSpec).toBeUndefined();
    expect(switched.formatSpec).toBe("combined");
  }
  expect(loadingDraftOf(row, "").formatSpec).toBe(row.formatSpec);
});
