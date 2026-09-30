/**
 * 端末の操作 (`GET /api/v0/terminals` と続く path) の応答の JSON。
 * 名前・IP アドレス・アカウントは本 fixture が決める値である。
 */

import { timelineResponseJson } from "../timeline/timelineResponse";

const [entry] = timelineResponseJson().entries;
if (entry === undefined) throw new Error("timeline fixture has no entry");

export const terminalNodeJson = entry.terminal;
export const terminalId = entry.terminal.id;
export const recordRefJson = entry.recordRef;
export const eventTimeJson = entry.eventTime;

const accountNode = {
  ...entry.terminal,
  id: "n:account:synthetic",
  kind: "account",
  label: { rawText: "user-z", valueState: "present" },
};

export function terminalsJson() {
  return {
    terminals: [
      {
        node: terminalNodeJson,
        names: ["HOST-C", "HOST-C2"],
        operatingSystem: "Example OS 10 1234",
        recordCount: 5,
        categorizedRecordCount: 3,
      },
    ],
  };
}

function categories(overrides: Record<string, unknown>[] = []) {
  const categoryNames = [
    "remote_logon",
    "account_management",
    "program_execution",
    "task_service_registration",
    "powershell",
    "installation",
    "defense_evasion",
  ];
  return categoryNames.map((category) => ({
    category,
    recordCount: 0,
    requiredSources: ["Security"],
    presentSources: ["Security"],
    ...overrides.find((item) => item.category === category),
  }));
}

export function terminalDetailJson() {
  return {
    node: terminalNodeJson,
    operatingSystem: [
      { name: "ProductName", value: "Example OS", recordRef: recordRefJson },
    ],
    names: [
      {
        name: "HOST-C",
        first: eventTimeJson,
        last: eventTimeJson,
        recordRefs: [recordRefJson],
      },
    ],
    addresses: [
      {
        interface: "{0000-synthetic}",
        values: [
          {
            name: "DhcpIPAddress",
            value: "192.0.2.10",
            recordRef: recordRefJson,
          },
        ],
        from: eventTimeJson,
        to: eventTimeJson,
      },
    ],
    timeZone: [
      {
        name: "ActiveTimeBias",
        value: "4294966951 (0xfffffea7)",
        recordRef: recordRefJson,
      },
    ],
    remoteLogons: [
      {
        sourceIp: "198.51.100.1",
        failureCount: 1,
        successCount: 4,
        connectionCount: 0,
        attemptedAccounts: ["user-a"],
        loggedOnAccounts: ["user-b"],
      },
      {
        sourceIp: "198.51.100.2",
        failureCount: 7,
        successCount: 0,
        connectionCount: 2,
        attemptedAccounts: ["user-c"],
        loggedOnAccounts: [],
        first: eventTimeJson,
        firstRecordRef: recordRefJson,
        last: eventTimeJson,
        lastRecordRef: recordRefJson,
      },
    ],
    categories: categories([
      { category: "program_execution", recordCount: 2 },
      {
        category: "powershell",
        requiredSources: ["Microsoft-Windows-PowerShell/Operational"],
        presentSources: [],
      },
    ]),
  };
}

export function terminalEventJson(fileName: string, sequenceNumber: number) {
  return {
    recordRef: {
      ...recordRefJson,
      sequenceNumber,
      recordRawTextRef: `/api/v0/records?sequenceNumber=${sequenceNumber}`,
    },
    eventTime: eventTimeJson,
    observationKind: entry.observationKind,
    categories: ["program_execution"],
    otherEventTimes: [eventTimeJson],
    fields: [
      {
        name: "Path",
        semantic: "file.path",
        kind: "text",
        text: { rawText: fileName, valueState: "present" },
      },
      {
        name: "Hash",
        semantic: "file.sha1",
        kind: "text",
        text: { rawText: "hash-not-shown", valueState: "present" },
      },
    ],
    namedNodes: [accountNode],
    originalFileNameDiffers: fileName.endsWith("renamed.exe"),
  };
}
