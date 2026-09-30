/**
 * 開発 server が API の要求の Origin を渡す先の origin へ書き換えるかを返す。
 *
 * 書き換えるのは、同じ端末の browser が開発 server 自身の origin から送った要求だけである。
 * 別の origin の page の要求と、別の端末から届いた要求は Origin をそのまま渡し、
 * oraculum-server が退ける。
 */
export function shouldRewriteDevProxyOrigin(request: {
  origin: string | undefined;
  host: string | undefined;
  remoteAddress: string | undefined;
}): boolean {
  return (
    request.origin !== undefined &&
    request.host !== undefined &&
    request.origin === `http://${request.host}` &&
    isLoopbackAddress(request.remoteAddress)
  );
}

function isLoopbackAddress(address: string | undefined): boolean {
  return (
    address === "::1" ||
    address?.startsWith("127.") === true ||
    address?.startsWith("::ffff:127.") === true
  );
}
