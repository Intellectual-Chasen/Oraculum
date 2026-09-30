import { fileURLToPath } from "node:url";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import type { ProxyOptions } from "vite";
import { defineConfig } from "vitest/config";
import { shouldRewriteDevProxyOrigin } from "./src/shared/lib/devProxyOrigin";

/** oraculum-server の既定の待ち受け先。`backend/cmd/oraculum-server` の defaultAddr と同じ値。 */
const defaultApiTarget = "http://127.0.0.1:8080";

/**
 * 開発 server から oraculum-server へ API の要求を渡す設定。
 *
 * oraculum-server は、要求の Host と、状態を変える要求の Origin が自分の待ち受け先であることを
 * 確かめる。開発 server は Host を渡す先の値にし、Origin を書き換える要求を
 * shouldRewriteDevProxyOrigin で決める。開発 server が受け付ける Host は Vite の
 * `server.allowedHosts` が決め、`vite --host` で別の端末へ開いても、別の端末の要求の Origin は
 * 書き換えない。
 */
function apiProxy(target: string): ProxyOptions {
  const targetOrigin = new URL(target).origin;
  return {
    target,
    changeOrigin: true,
    configure: (proxy) => {
      proxy.on("proxyReq", (proxyRequest, request) => {
        if (
          shouldRewriteDevProxyOrigin({
            origin: request.headers.origin,
            host: request.headers.host,
            remoteAddress: request.socket.remoteAddress,
          })
        ) {
          proxyRequest.setHeader("origin", targetOrigin);
        }
      });
    },
  };
}

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
      // cosmos.gl が読む gl-bench は browser 欄が ES module でない file を指す。module 版を読む。
      "gl-bench": "gl-bench/dist/gl-bench.module.js",
    },
  },
  server: {
    // 画面は同じ origin だけを使う。別の origin の page に API の応答を読ませない。
    cors: false,
    // 開発 server と同じ origin で API を受け取り、画面から CORS を扱わずに済ませる。
    // 待ち受け先を変えた server へつなぐときは ORACULUM_API_TARGET を設定する。
    proxy: {
      "/api": apiProxy(process.env.ORACULUM_API_TARGET ?? defaultApiTarget),
    },
  },
  test: {
    environment: "node",
    include: ["src/**/*.test.{ts,tsx}"],
    setupFiles: ["src/testSetup.ts"],
    // 既知の制限: test 1 件の上限を既定の 5 秒から 30 秒に延ばす, CPU を他の検査と共有する CI では画面の test 1 件が手元の約 10 倍 (0.8 秒から 7.3 秒) かかり、同じ負荷を 1 core に掛けた手元でも 5 件が 5 秒を超えた, CI の runner が検査を並べて動かさなくなったときに見直す
    testTimeout: 30_000,
  },
});
