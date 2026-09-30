import { Component, type ReactNode } from "react";
import { toVisibleRawText } from "@/shared/lib/rawText";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { StatusLabel } from "@/shared/ui/StatusLabel";

type SubgraphCanvasBoundaryProps = { children: ReactNode };
type SubgraphCanvasBoundaryState = {
  hasFailed: boolean;
  webglUnavailable: boolean;
};

/** 図の WebGL2 の context を作れなかったことを表す例外。 */
export class WebglUnavailableError extends Error {
  constructor() {
    super("the WebGL2 context could not be created");
  }
}

/**
 * 図の描画で起きた例外を受け止める。
 *
 * WebGL を使えない環境と、1 件のノードに起因する例外で、ノードの一覧と根拠の表示まで
 * 消えることを避ける。図を描けなかったことだけを画面に書き、例外の内容を画面に出さない。
 */
export class SubgraphCanvasBoundary extends Component<
  SubgraphCanvasBoundaryProps,
  SubgraphCanvasBoundaryState
> {
  state: SubgraphCanvasBoundaryState = {
    hasFailed: false,
    webglUnavailable: false,
  };

  static getDerivedStateFromError(error: unknown): SubgraphCanvasBoundaryState {
    return {
      hasFailed: true,
      webglUnavailable: error instanceof WebglUnavailableError,
    };
  }

  componentDidCatch(error: unknown) {
    // 例外の文字列には原資料由来の値が入りうる。制御文字と書式文字を可視の符号へ
    // 置き換えてから記録し、1 件の記録が複数行に割れる形を避ける。
    const reason = error instanceof Error ? error.message : String(error);
    console.error("rendering the subgraph failed", toVisibleRawText(reason));
  }

  render() {
    if (this.state.hasFailed) {
      return (
        <div role="alert">
          <StatusLabel
            status="failed"
            label={
              this.state.webglUnavailable ? "GPU の WebGL2 なし" : "描画に失敗"
            }
            details={
              <KeyValueList
                stacked
                pairs={[
                  {
                    name: "次の操作",
                    value: this.state.webglUnavailable
                      ? "ハードウェア アクセラレーションの有効化"
                      : "描画できる環境の確認",
                  },
                  { name: "ノードの選択", value: "Nodes のビュー" },
                ]}
              />
            }
          />
        </div>
      );
    }
    return this.props.children;
  }
}
