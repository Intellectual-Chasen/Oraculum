/** 応答の形が、画面が読める形と異なることを表す。`decode` の実行中だけ投げ、境界で受け止める。 */
export class DecodeFailure extends Error {
  readonly path: string;

  constructor(path: string, reason: string) {
    super(`${path}: ${reason}`);
    this.name = "DecodeFailure";
    this.path = path;
  }
}

/** `unknown` の 1 値を検証して画面が扱う型へ変換する。失敗は `DecodeFailure` で投げる。 */
export type Decoder<T> = (input: unknown, path: string) => T;

/** JSON の object として読む。配列と `null` を object として扱わない。 */
export function readObject(
  input: unknown,
  path: string,
): Record<string, unknown> {
  if (typeof input !== "object" || input === null || Array.isArray(input)) {
    throw new DecodeFailure(path, "expected a JSON object");
  }
  return input as Record<string, unknown>;
}

/** 必須の文字列を読む。 */
export function requireString(
  source: Record<string, unknown>,
  key: string,
  path: string,
): string {
  const value = source[key];
  if (typeof value !== "string") {
    throw new DecodeFailure(`${path}.${key}`, "expected a string");
  }
  return value;
}

/** 省略可の文字列を読む。項目が無いときは `undefined` を返す。 */
export function optionalString(
  source: Record<string, unknown>,
  key: string,
  path: string,
): string | undefined {
  if (!(key in source)) {
    return undefined;
  }
  return requireString(source, key, path);
}

/** 必須の 10 進整数を読む。符号を持たない整数だけを受け取る。 */
export function requireCount(
  source: Record<string, unknown>,
  key: string,
  path: string,
): number {
  const value = source[key];
  if (
    typeof value !== "number" ||
    !Number.isSafeInteger(value) ||
    value < 0 ||
    Object.is(value, -0)
  ) {
    throw new DecodeFailure(
      `${path}.${key}`,
      "expected a non-negative safe integer",
    );
  }
  return value;
}

/** 省略可の 10 進整数を読む。項目が無いときは `undefined` を返す。 */
export function optionalCount(
  source: Record<string, unknown>,
  key: string,
  path: string,
): number | undefined {
  if (!(key in source)) {
    return undefined;
  }
  return requireCount(source, key, path);
}

/** 必須の真偽を読む。 */
export function requireBoolean(
  source: Record<string, unknown>,
  key: string,
  path: string,
): boolean {
  const value = source[key];
  if (typeof value !== "boolean") {
    throw new DecodeFailure(`${path}.${key}`, "expected a boolean");
  }
  return value;
}

/** 省略可の真偽を読む。項目が無いときは `undefined` を返す。 */
export function optionalBoolean(
  source: Record<string, unknown>,
  key: string,
  path: string,
): boolean | undefined {
  if (!(key in source)) {
    return undefined;
  }
  return requireBoolean(source, key, path);
}

/** 必須の列挙を読む。`values` が並べた値だけを受け取る。 */
export function requireEnum<T extends string>(
  source: Record<string, unknown>,
  key: string,
  path: string,
  values: readonly T[],
): T {
  const value = requireString(source, key, path);
  const found = values.find((candidate) => candidate === value);
  if (found === undefined) {
    throw new DecodeFailure(
      `${path}.${key}`,
      `expected one of ${values.join(" / ")}`,
    );
  }
  return found;
}

/** 省略可の列挙を読む。項目が無いときは `undefined` を返す。 */
export function optionalEnum<T extends string>(
  source: Record<string, unknown>,
  key: string,
  path: string,
  values: readonly T[],
): T | undefined {
  if (!(key in source)) {
    return undefined;
  }
  return requireEnum(source, key, path, values);
}

/** 集合の要素として文字列を読む。 */
export const decodeString: Decoder<string> = (input, path) => {
  if (typeof input !== "string") {
    throw new DecodeFailure(path, "expected a string");
  }
  return input;
};

/** 必須の集合を読む。要素数 0 も集合として受け取る。 */
export function requireArray<T>(
  source: Record<string, unknown>,
  key: string,
  path: string,
  decode: Decoder<T>,
): T[] {
  const value = source[key];
  if (!Array.isArray(value)) {
    throw new DecodeFailure(`${path}.${key}`, "expected an array");
  }
  return value.map((element, index) =>
    decode(element, `${path}.${key}[${index}]`),
  );
}

/** 省略可の配列を読む。項目が無いときは `undefined` を返す。 */
export function optionalArray<T>(
  source: Record<string, unknown>,
  key: string,
  path: string,
  decode: Decoder<T>,
): T[] | undefined {
  if (!(key in source)) {
    return undefined;
  }
  return requireArray(source, key, path, decode);
}

/** 必須の組を読む。 */
export function requireMember<T>(
  source: Record<string, unknown>,
  key: string,
  path: string,
  decode: Decoder<T>,
): T {
  return decode(source[key], `${path}.${key}`);
}

/** 省略可の組を読む。項目が無いときは `undefined` を返す。 */
export function optionalMember<T>(
  source: Record<string, unknown>,
  key: string,
  path: string,
  decode: Decoder<T>,
): T | undefined {
  if (!(key in source)) {
    return undefined;
  }
  return decode(source[key], `${path}.${key}`);
}

/** 項目が出ていることを拒む。応答が項目を出さない条件で使う。 */
export function rejectMember(
  source: Record<string, unknown>,
  key: string,
  path: string,
  reason: string,
): void {
  if (key in source) {
    throw new DecodeFailure(`${path}.${key}`, reason);
  }
}
