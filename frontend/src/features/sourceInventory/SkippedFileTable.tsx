import type { SkippedFile } from "@/shared/contracts/sources";
import { MissingValue } from "@/shared/ui/MissingValue";
import { RawText } from "@/shared/ui/RawText";
import { skippedFileReasonLabels } from "./labels";

/**
 * 収集の directory にあり、取り込まなかったファイルを理由とともに出す。ファイルが無いときは
 * 何も出さない。検出した形式の列は、形式を持つファイルが 1 件以上あるときだけ置く。
 */
export function SkippedFileTable({ files }: { files: readonly SkippedFile[] }) {
  if (files.length === 0) {
    return null;
  }
  const showsKind = files.some((file) => file.detectedKind !== undefined);
  return (
    <table>
      <caption>取り込まなかったファイル</caption>
      <thead>
        <tr>
          <th scope="col">ファイル</th>
          <th scope="col">理由</th>
          {showsKind ? <th scope="col">検出した形式</th> : null}
        </tr>
      </thead>
      <tbody>
        {files.map((file) => (
          <tr key={file.originPath}>
            <td className="wrapping-cell">
              <RawText text={file.originPath} />
            </td>
            <td>{skippedFileReasonLabels[file.reason]}</td>
            {showsKind ? (
              <td>
                {file.detectedKind ?? <MissingValue description="形式なし" />}
              </td>
            ) : null}
          </tr>
        ))}
      </tbody>
    </table>
  );
}
