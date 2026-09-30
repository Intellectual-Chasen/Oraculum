import { expect, test } from "vitest";
import {
  loadingFailureReasons,
  processingFailureReasons,
} from "@/shared/contracts/stages";
import { stageFailureReasonLabels } from "./labels";

test("段階が失敗した理由の各値に、日本語の短いラベルを持つ", () => {
  expect(stageFailureReasonLabels).toEqual({
    source_import_failed: "収集元の読み取りか取り込みの失敗",
    import_failed: "取り込みの準備の失敗",
    recording_failed: "調査への記録の失敗",
    graph_build_failed: "グラフの構築の失敗",
    interrupted: "サーバーの停止による中断",
  });
});

test("読み込みと処理の段階が使う理由の種別は、どれも表示を持つ", () => {
  for (const reason of [
    ...loadingFailureReasons,
    ...processingFailureReasons,
  ]) {
    expect(Object.keys(stageFailureReasonLabels)).toContain(reason);
  }
});
