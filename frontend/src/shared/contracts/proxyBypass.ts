import {
  type Decoder,
  decodeString,
  optionalString,
  readObject,
  requireArray,
  requireBoolean,
  requireCount,
  requireString,
} from "./decoding";

/** `backend/api/proxy_bypass.go` の `proxyBypassDestination`。 */
export type ProxyBypassDestination = {
  address: string;
  /** port を記録しない接続では出ない。 */
  port?: string;
  recordCount: number;
};

/** `backend/api/proxy_bypass.go` の `proxyBypassClient`。 */
export type ProxyBypassClient = {
  clientIp: string;
  /** 端末の割当がこのアドレスに結んだ端末。 */
  terminals: string[];
  /** Proxy のログがこのアドレスから受けた要求の件数。 */
  proxyRequestCount: number;
  /** 他の収集元のうち、Proxy のアドレスへの接続を記録したレコードの件数。 */
  proxyConnectionCount: number;
  /** 他の収集元のうち、Proxy のアドレス以外への接続を記録したレコードの件数。 */
  directConnectionCount: number;
  /** 偽のとき、このアドレスの接続を記録する収集元が無く、接続の件数の 0 は数えた結果ではない。 */
  directConnectionCountable: boolean;
  directDestinations: ProxyBypassDestination[];
};

/** `backend/api/proxy_bypass.go` の `proxyBypassResponse`。 */
export type ProxyBypassResponse = {
  sourceId: string;
  /** 要素数 0 のときは、Proxy への接続と経由しない接続を分けていない。 */
  proxyAddresses: string[];
  /** 接続元を IP アドレスとして読めず、clients に数えなかった Proxy のログのレコードの件数。 */
  unreadableProxyRecordCount: number;
  clients: ProxyBypassClient[];
};

const decodeDestination: Decoder<ProxyBypassDestination> = (input, path) => {
  const source = readObject(input, path);
  return {
    address: requireString(source, "address", path),
    port: optionalString(source, "port", path),
    recordCount: requireCount(source, "recordCount", path),
  };
};

const decodeClient: Decoder<ProxyBypassClient> = (input, path) => {
  const source = readObject(input, path);
  return {
    clientIp: requireString(source, "clientIp", path),
    terminals: requireArray(source, "terminals", path, decodeString),
    proxyRequestCount: requireCount(source, "proxyRequestCount", path),
    proxyConnectionCount: requireCount(source, "proxyConnectionCount", path),
    directConnectionCount: requireCount(source, "directConnectionCount", path),
    directConnectionCountable: requireBoolean(
      source,
      "directConnectionCountable",
      path,
    ),
    directDestinations: requireArray(
      source,
      "directDestinations",
      path,
      decodeDestination,
    ),
  };
};

/** Proxy を経由した要求と経由しない接続の比較の応答を検証する。 */
export const decodeProxyBypassResponse: Decoder<ProxyBypassResponse> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    sourceId: requireString(source, "sourceId", path),
    proxyAddresses: requireArray(source, "proxyAddresses", path, decodeString),
    unreadableProxyRecordCount: requireCount(
      source,
      "unreadableProxyRecordCount",
      path,
    ),
    clients: requireArray(source, "clients", path, decodeClient),
  };
};
