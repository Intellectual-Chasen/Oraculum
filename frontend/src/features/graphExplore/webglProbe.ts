/**
 * GPU で WebGL2 を描けるかを、図を作る前に確かめる。
 *
 * **ソフトウェアで描く WebGL2 を、描けないものとして扱う。** GPU の無い環境のブラウザは、
 * CPU で WebGL を描く実装 (SwiftShader など) で context を作る。図の配置の計算は CPU で
 * 描くと画面の main thread を長く止め、ほかの操作も受け付けなくなる。
 * `failIfMajorPerformanceCaveat` を付けると、ブラウザはその実装での context の作成を断る。
 * headless の chromium は SwiftShader の context を断らないため、描く実装の名前も確かめる。
 *
 * 確かめに作った context は、すぐに手放す。判定の結果は画面の読み込みごとに 1 回だけ求め、
 * 図を作り直すたびに context を作らない。
 */
export function canDrawWithGpu(): boolean {
  probed ??= probeGpu();
  return probed;
}

let probed: boolean | undefined;

function probeGpu(): boolean {
  let context: WebGL2RenderingContext | null = null;
  try {
    context = document.createElement("canvas").getContext("webgl2", {
      failIfMajorPerformanceCaveat: true,
    });
  } catch {
    return false;
  }
  if (context === null) return false;
  const info = context.getExtension("WEBGL_debug_renderer_info");
  const renderer =
    info === null
      ? ""
      : String(context.getParameter(info.UNMASKED_RENDERER_WEBGL));
  context.getExtension("WEBGL_lose_context")?.loseContext();
  return !softwareRendererPattern.test(renderer);
}

/** CPU で WebGL を描く実装の名前。 */
const softwareRendererPattern =
  /swiftshader|llvmpipe|softpipe|software|basic render driver/i;
