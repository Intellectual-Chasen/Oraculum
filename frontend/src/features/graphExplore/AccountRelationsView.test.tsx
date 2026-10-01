// @vitest-environment jsdom
import { fireEvent, render, screen } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import { decodeAccountRelationsResponse } from "@/shared/contracts/accountRelations";
import {
  accountNodeDetailResponseJson,
  recordNode,
} from "@/testdata/graph/graphResponse";
import {
  hostALogSha256,
  hostALogSourceId,
} from "@/testdata/sources/sourcesResponse";
import {
  AccountRelationsDetail,
  AccountRelationsFigure,
} from "./AccountRelationsView";

test("相手の識別鍵と根拠の役割を表示し、50 件の続きを要求できる", () => {
  const origin = accountNodeDetailResponseJson().node;
  const counterpart = {
    ...origin,
    id: "n:account:counterpart",
    identity: [{ semantic: "account.name", value: "domuser" }],
    label: { rawText: "domuser", valueState: "present" },
  };
  const key = {
    counterpartId: counterpart.id,
    originRole: "record_target_account" as const,
    otherRole: "record_subject_account" as const,
  };
  const record = {
    nodeId: recordNode("matched").id,
    originEdgeId: "e:target",
    otherEdgeId: "e:subject",
    summary: {
      recordRef: {
        sourceId: hostALogSourceId,
        sourceContentSha256: hostALogSha256,
        sourceFileName: "Security.evtx",
        positionKind: "sequence_number",
        sequenceNumber: 1,
        recordRawTextRef: "record-1",
      },
      windowsEvent: true,
      eventCategory: "Microsoft-Windows-Security-Auditing",
      eventAction: "4769",
      eventRecordId: "27598",
    },
    sigmaRulePaths: [],
  };
  const response = decodeAccountRelationsResponse(
    {
      origin,
      groups: [
        {
          ...key,
          counterpart,
          recordCount: 51,
          unknownTimeCount: 0,
          events: [
            {
              category: "Microsoft-Windows-Security-Auditing",
              action: "4769",
              count: 51,
            },
          ],
          sources: [{ sourceId: hostALogSourceId, count: 51 }],
          sigmaRulePaths: [],
        },
      ],
      records: [record],
      selectedRecordCount: 51,
      nextOffset: 50,
      periodUnjudgedRecordCount: 0,
    },
    "response",
  );
  const onSelectRecord = vi.fn();
  const onLoadMore = vi.fn();
  render(
    <>
      <AccountRelationsFigure
        state={{ status: "loaded", value: response }}
        selected={key}
      />
      <AccountRelationsDetail
        state={{ status: "loaded", value: response }}
        selected={key}
        onSelect={() => {}}
        onSelectRecord={onSelectRecord}
        onSelectNode={() => {}}
        onLoadMore={onLoadMore}
        loadingMore={false}
      />
    </>,
  );
  expect(
    screen.getByRole("img", { name: /個別レコードと両アカウント/ }),
  ).toBeInTheDocument();
  expect(screen.getByText(/個別レコード: 1 \/ 51 件表示/)).toBeInTheDocument();
  expect(screen.getByText(/未表示: 50 件/)).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: /レコード 27598/ }));
  expect(onSelectRecord).toHaveBeenCalledWith(
    response.records?.[0].summary.recordRef,
  );
  fireEvent.click(
    screen.getByRole("button", { name: "続きを読み込む（次の 50 件）" }),
  );
  expect(onLoadMore).toHaveBeenCalledOnce();
});
