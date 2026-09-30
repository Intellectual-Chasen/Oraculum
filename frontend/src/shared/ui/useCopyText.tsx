import { X } from "lucide-react";
import {
  type ReactNode,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";
import { IconButton } from "./IconButton";
import { KeyValueList, type KeyValuePair } from "./KeyValueList";
import { StatusLabel } from "./StatusLabel";

/** 文字列を clipboard へ書いた結果。 */
export type CopyOutcome =
  | { ok: true }
  | {
      ok: false;
      /**
       * `unavailable` は画面が clipboard を使えない (https か localhost で開いていない)、
       * `rejected` はブラウザーが書き込みを拒んだことを表す。
       */
      reason: "unavailable" | "rejected";
    };

/** clipboard へ書く手段のうち、本画面が使う操作。 */
type ClipboardWriter = { writeText: (text: string) => Promise<void> };

/**
 * 文字列を clipboard へそのまま書く。失敗を投げずに結果として返す。
 *
 * **原資料の文字列を整形せずに渡す。** 前後の空白と制御文字も含めて書く。
 */
export async function copyText(
  text: string,
  clipboard: ClipboardWriter | undefined,
): Promise<CopyOutcome> {
  if (clipboard === undefined) return { ok: false, reason: "unavailable" };
  try {
    await clipboard.writeText(text);
    return { ok: true };
  } catch {
    return { ok: false, reason: "rejected" };
  }
}

/** コピーした結果の通知。 */
type CopyNotice = { what: string; outcome: CopyOutcome };

/** コピーできたことの通知を消すまでの時間 (ms)。 */
const successNoticeMs = 4000;

/** コピーできなかった理由と、次に行う操作の「名前: 値」の組。 */
function failurePairs(
  what: string,
  reason: "unavailable" | "rejected",
): KeyValuePair[] {
  switch (reason) {
    case "unavailable":
      return [
        { name: "対象", value: what },
        { name: "理由", value: "clipboard を使えない URL" },
        { name: "操作", value: "https か localhost の URL で開く" },
      ];
    case "rejected":
      return [
        { name: "対象", value: what },
        { name: "理由", value: "ブラウザーが書き込みを拒否" },
        { name: "操作", value: "ブラウザーの権限の設定を確認" },
      ];
    default: {
      const exhaustive: never = reason;
      throw new Error(`unknown copy failure: ${String(exhaustive)}`);
    }
  }
}

/**
 * 値を clipboard へ写す操作と、その結果の通知を持つ。通知は notice を置いた場所に出す。
 *
 * **clipboard は notice を置いた要素のウィンドウのものを使う。** 別ウィンドウに出したビューから
 * 写すとき、元のウィンドウの clipboard は focus を持たない document として書き込みを拒む。
 *
 * 写せたことの通知は少し後に消す。写せなかったことの通知は、分析者が閉じるか次に写すまで残す。
 */
export function useCopyText(): {
  /** text を写し、what (何を写したか) を通知に書く。 */
  copy: (text: string, what: string) => void;
  notice: ReactNode;
} {
  const slotRef = useRef<HTMLSpanElement>(null);
  const [notice, setNotice] = useState<CopyNotice | undefined>(undefined);

  const copy = useCallback((text: string, what: string) => {
    const view = slotRef.current?.ownerDocument.defaultView ?? window;
    // 画面を https か localhost で開いていないとき、navigator.clipboard は無い。
    const clipboard: ClipboardWriter | undefined = view.navigator.clipboard;
    void copyText(text, clipboard).then((outcome) =>
      setNotice({ what, outcome }),
    );
  }, []);

  useEffect(() => {
    if (notice?.outcome.ok !== true) return;
    const timer = setTimeout(() => setNotice(undefined), successNoticeMs);
    return () => clearTimeout(timer);
  }, [notice]);

  return {
    copy,
    notice: (
      <span ref={slotRef} className="copy-notice">
        {notice === undefined ? null : notice.outcome.ok ? (
          <span role="status">
            <StatusLabel status="done" label={`コピー済み: ${notice.what}`} />
          </span>
        ) : (
          <span role="alert" className="inline-flex items-center gap-1">
            <StatusLabel
              status="failed"
              label={`コピーできません: ${notice.what}`}
              details={
                <KeyValueList
                  stacked
                  pairs={failurePairs(notice.what, notice.outcome.reason)}
                />
              }
            />
            <IconButton label="閉じる" onPress={() => setNotice(undefined)}>
              <X size={14} aria-hidden="true" />
            </IconButton>
          </span>
        )}
      </span>
    ),
  };
}
