import { type ReactNode, useState } from "react";

/**
 * 開いた後にだけ中身を描く `<details>`。閉じた `<details>` の中身も DOM に描かれるため、
 * 数万件の要素を持つ応答で、開いていない一覧を描かない。一度開いた後は、閉じても描いたままにする。
 */
export function LazyDetails({
  summary,
  children,
}: {
  summary: ReactNode;
  children: () => ReactNode;
}) {
  const [opened, setOpened] = useState(false);
  return (
    <details
      onToggle={(event) => {
        if (event.currentTarget.open) setOpened(true);
      }}
    >
      <summary>{summary}</summary>
      {opened ? children() : null}
    </details>
  );
}
