import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

/** class を結合する。後ろに渡した Tailwind の class が、同じ性質の前の class に勝つ。 */
export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs));
}
