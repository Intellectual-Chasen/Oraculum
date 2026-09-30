import type { ReactNode } from "react";
import type { FetchState } from "../lib/fetchState";
import { FetchFailureNotice } from "./FetchFailureNotice";
import { StatusLabel } from "./StatusLabel";

type FetchStateViewProps<T> = {
  state: FetchState<T>;
  /** 読み込み中に、処理中の状態のラベルとして出す短い名詞 (「レコードの読み込み中」)。 */
  loadingDescription: string;
  /** 成功した値の表示。 */
  children: (value: T) => ReactNode;
};

/** 読み込み中・取得失敗・結果なし・成功の 4 状態を別の表示に分ける。 */
export function FetchStateView<T>({
  state,
  loadingDescription,
  children,
}: FetchStateViewProps<T>) {
  switch (state.status) {
    case "loading":
      return (
        <p role="status">
          <StatusLabel status="running" label={loadingDescription} />
        </p>
      );
    case "failed":
      return <FetchFailureNotice failure={state.failure} />;
    case "empty":
      return <p role="status">{state.description}</p>;
    case "loaded":
      return children(state.value);
    default: {
      const exhaustive: never = state;
      throw new Error(`unknown fetch state: ${JSON.stringify(exhaustive)}`);
    }
  }
}
