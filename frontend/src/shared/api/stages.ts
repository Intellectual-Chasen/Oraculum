import { isCaseId } from "../contracts/cases";
import {
  decodeInvestigationStages,
  type InvestigationStages,
} from "../contracts/stages";
import type { FetchFailure } from "../lib/fetchState";
import { buildFetchFailure } from "./apiFailure";
import { type ApiRequest, type ApiResult, requestJson } from "./httpClient";

const fetchFailureSummary = "段階の状態の取得";
const loadingFailureSummary = "収集元の読み込みの開始";
const processingFailureSummary = "処理の開始";

/**
 * 段階を始める要求が通信の失敗で終わったときに、次に行う操作のラベル。
 * 段階を始める要求は server の段階を進める。通信の失敗では、server が要求を受け取って
 * 段階を始めたかを画面は確かめられないため、再実行の前に段階の状態を確かめさせる。
 */
const startNetworkNextAction = "段階の状態を確認してから操作を選択";

/**
 * 段階を始める要求を、画面の持つ段階の状態が古いことを理由に退けられたときに、次に行う
 * 操作のラベル。
 */
const staleStagesNextAction = startNetworkNextAction;

/**
 * 段階を始める要求の失敗の code が、画面の持つ段階の状態が server の状態より古いことを
 * 表すかを返す。
 */
export function showsStaleStages(failure: FetchFailure): boolean {
  return (
    failure.failureCode === "stage_already_started" ||
    failure.failureCode === "stage_not_ready"
  );
}

/** 段階を始める要求の失敗に合う、次に行えることを返す。分類ごとの案内を使う失敗には `undefined` を返す。 */
function stageStartNextActionOf(failure: FetchFailure): string | undefined {
  if (failure.kind === "network") {
    return startNetworkNextAction;
  }
  if (showsStaleStages(failure)) {
    return staleStagesNextAction;
  }
  return undefined;
}

/** 段階を始める要求を送り、失敗には段階を始める要求に合う次の操作を載せる。 */
async function requestStageStart(
  request: ApiRequest<InvestigationStages>,
): Promise<ApiResult<InvestigationStages>> {
  const result = await requestJson(request);
  if (result.ok) {
    return result;
  }
  const nextAction = stageStartNextActionOf(result.failure);
  return nextAction === undefined
    ? result
    : { ok: false, failure: { ...result.failure, nextAction } };
}

/** 収集元を記録した端末の入力。3 項目とも省略できる。 */
export type LoadingTerminalDraft = {
  /** 端末の外部識別子。 */
  id?: string;
  /** 端末の表示名。 */
  hostname?: string;
  /** 端末が持つ IP アドレス。 */
  ip?: string;
};

/**
 * 読み込む収集元 1 件の入力。項目の定義元は backend の `POST /api/v0/stages/loading` の
 * handler である。空白だけの省略可の項目は、省いた項目として扱う。
 */
export type LoadingSourceDraft = {
  /** server の基準の directory からの相対 path。入力の文字列のまま送る。 */
  originPath: string;
  formatKey: string;
  /** 欄の並びの指定。 */
  formatSpec?: string;
  /** 収集元に付ける案件。 */
  caseId?: string;
  terminal?: LoadingTerminalDraft;
};

/** 前後の空白を除いた文字列を返す。空白だけの文字列は省いた項目として `undefined` を返す。 */
function presentText(value: string | undefined): string | undefined {
  const trimmed = value?.trim();
  return trimmed === undefined || trimmed === "" ? undefined : trimmed;
}

/**
 * 送る前に分けられる入力の誤りを「名前: 値」の短いラベルで返す。誤りが無いときは `undefined`
 * を返す。
 */
function draftsProblem(drafts: LoadingSourceDraft[]): string | undefined {
  if (drafts.length === 0) {
    return "収集元なし";
  }
  for (const [index, draft] of drafts.entries()) {
    // path を持たない収集元は並びの番号で、ほかは path で、誤りのある収集元を指す。
    if (draft.originPath.trim() === "") {
      return `収集元 ${index + 1}: path なし`;
    }
    if (draft.formatKey.trim() === "") {
      return `${draft.originPath}: 入力形式なし`;
    }
  }
  // 画面は案件をすべての収集元に同じ値で付ける。誤りは収集元を指さずに出す。
  const invalidCase = drafts.some((draft) => {
    const caseId = presentText(draft.caseId);
    return caseId !== undefined && !isCaseId(caseId);
  });
  return invalidCase ? "案件の形式の誤り" : undefined;
}

/** 端末の 3 項目のうち値を持つ項目だけを組にする。値を持つ項目が無いときは `undefined` を返す。 */
function terminalBody(
  terminal: LoadingTerminalDraft | undefined,
): LoadingTerminalDraft | undefined {
  const id = presentText(terminal?.id);
  const hostname = presentText(terminal?.hostname);
  const ip = presentText(terminal?.ip);
  if (id === undefined && hostname === undefined && ip === undefined) {
    return undefined;
  }
  return { id, hostname, ip };
}

/** 入力から要求の本文を組む。空白だけの省略可の項目を本文に載せない。 */
function loadingRequestBody(
  drafts: LoadingSourceDraft[],
  thenProcess: boolean,
) {
  return {
    sources: drafts.map((draft) => ({
      originPath: draft.originPath,
      formatKey: draft.formatKey,
      formatSpec: presentText(draft.formatSpec),
      caseId: presentText(draft.caseId),
      terminal: terminalBody(draft.terminal),
    })),
    thenProcess: thenProcess ? true : undefined,
  };
}

/** 調査の段階の状態と進行 (`GET /api/v0/stages`) を取得する。 */
export async function fetchStages(
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<InvestigationStages>> {
  return requestJson({
    path: "/stages",
    decode: decodeInvestigationStages,
    failureSummary: fetchFailureSummary,
    signal: options.signal,
  });
}

/**
 * 収集元の読み込みを始める (`POST /api/v0/stages/loading`)。`thenProcess` が真のときは、
 * 読み込みが完了したら server が続けて処理を始める。
 */
export async function startLoading(
  drafts: LoadingSourceDraft[],
  options: { thenProcess?: boolean; signal?: AbortSignal } = {},
): Promise<ApiResult<InvestigationStages>> {
  const problem = draftsProblem(drafts);
  if (problem !== undefined) {
    return {
      ok: false,
      failure: buildFetchFailure("request_rejected", problem),
    };
  }
  return requestStageStart({
    path: "/stages/loading",
    method: "POST",
    body: loadingRequestBody(drafts, options.thenProcess === true),
    decode: decodeInvestigationStages,
    failureSummary: loadingFailureSummary,
    signal: options.signal,
  });
}

/** 処理を始める (`POST /api/v0/stages/processing`)。本文を送らない。 */
export async function startProcessing(
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<InvestigationStages>> {
  return requestStageStart({
    path: "/stages/processing",
    method: "POST",
    decode: decodeInvestigationStages,
    failureSummary: processingFailureSummary,
    signal: options.signal,
  });
}
