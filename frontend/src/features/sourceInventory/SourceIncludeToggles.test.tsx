// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import type { SourceRef } from "@/shared/api/graph";
import type { SourceIdentity } from "@/shared/contracts/sources";
import { SourceIncludeToggles } from "./SourceIncludeToggles";

afterEach(cleanup);

const source = (id: string, fileName: string) =>
  ({ sourceId: id, fileName }) as SourceIdentity;
const sources = [source("s1", "alpha.log"), source("s2", "beta.log")];

function renderToggles(included: SourceRef[]) {
  const onChange = vi.fn();
  render(
    <SourceIncludeToggles
      sources={sources}
      included={included}
      onChange={onChange}
    />,
  );
  return onChange;
}

test("空の選択はすべてに印を付け、1 件を外すと残りだけを選ぶ", () => {
  const onChange = renderToggles([]);
  const alpha = screen.getByRole("checkbox", { name: "alpha.log" });
  expect((alpha as HTMLInputElement).checked).toBe(true);

  fireEvent.click(alpha);

  expect(onChange).toHaveBeenCalledWith([{ id: "s2", label: "beta.log" }]);
});

test("すべてに印を戻すと空の選択にし、最後の 1 件は外せない", () => {
  const onChange = renderToggles([{ id: "s2", label: "beta.log" }]);
  const beta = screen.getByRole("checkbox", { name: "beta.log" });
  expect((beta as HTMLInputElement).disabled).toBe(true);

  fireEvent.click(screen.getByRole("checkbox", { name: "alpha.log" }));

  expect(onChange).toHaveBeenCalledWith([]);
});
