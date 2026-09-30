const counts = new Intl.NumberFormat("ja-JP");

/** 件数を 3 桁ごとに区切って書く。 */
export function formatCount(value: number): string {
  return counts.format(value);
}
