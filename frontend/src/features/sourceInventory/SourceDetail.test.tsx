// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { SourceDetail } from "./SourceDetail";

afterEach(cleanup);

test("収集元を選んでいないときは、収集元の値の組に未選択を出す", () => {
  const { container } = render(<SourceDetail row={undefined} />);
  expect(container.textContent).toBe("収集元: 未選択");
  expect(screen.queryByRole("table")).toBeNull();
});
