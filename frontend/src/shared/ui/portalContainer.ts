import { createContext, useContext } from "react";

/**
 * tooltip などの浮く要素を描く先の要素を返す関数。区画を別ウィンドウに出すと、区画の中身は
 * そのウィンドウの文書に移る。区画の中の浮く要素を同じ文書に描き、元のウィンドウに描かれて
 * 見えなくなることを防ぐ。
 */
export const PortalContainerContext = createContext<() => Element | undefined>(
  () => undefined,
);

export function usePortalContainer(): Element | undefined {
  return useContext(PortalContainerContext)();
}
