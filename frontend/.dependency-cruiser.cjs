// 承認済みの配置を検査する。型だけの依存も省略しない。
//
// **rule の severity は error にする。** depcruise の終了コードは error の件数であり、
// warn の rule に違反した依存は exit 0 で通る。
//
// 既知の制限: 解析したモジュールが 0 件でも exit 0 になる,
// 件数の下限を渡す option が depcruise 18.2.0 に無く、実 repo では 76 モジュールと
// 272 件の依存を解析している (2026-09-18),
// 0 件を失敗にする option が入ったとき見直す。
module.exports = {
  forbidden: [
    {
      name: "no-circular",
      severity: "error",
      from: {},
      to: { circular: true },
    },
    {
      name: "ui-must-not-depend-on-api",
      severity: "error",
      from: { path: "^src/shared/ui/" },
      to: { path: "^src/shared/api/" },
    },
    {
      name: "api-must-not-depend-on-ui",
      severity: "error",
      from: { path: "^src/shared/api/" },
      to: { path: "^src/shared/ui/" },
    },
    {
      name: "lib-must-not-depend-on-api",
      severity: "error",
      from: { path: "^src/shared/lib/" },
      to: { path: "^src/shared/api/" },
    },
    {
      name: "no-unresolved",
      severity: "error",
      from: {},
      to: { couldNotResolve: true },
    },
    {
      name: "shared-must-not-depend-on-upper-layers",
      severity: "error",
      from: { path: "^src/shared/" },
      to: { path: "^src/(app|features)/" },
    },
    {
      name: "features-must-not-depend-on-app",
      severity: "error",
      from: { path: "^src/features/" },
      to: { path: "^src/app/" },
    },
    {
      name: "contracts-must-be-a-leaf",
      severity: "error",
      comment:
        "src/shared/contracts/ は葉である。通信・汎用 UI・純粋処理に依存しない。",
      from: { path: "^src/shared/contracts/" },
      to: { path: "^src/shared/(api|ui|lib)/" },
    },
    {
      name: "no-sibling-feature-imports",
      severity: "error",
      from: { path: "^src/features/([^/]+)/" },
      to: { path: "^src/features/", pathNot: "^src/features/$1/" },
    },
  ],
  options: {
    doNotFollow: { path: "node_modules" },
    tsPreCompilationDeps: true,
    tsConfig: { fileName: "tsconfig.json" },
    enhancedResolveOptions: {
      extensions: [".ts", ".tsx", ".js", ".jsx", ".json"],
    },
  },
};
