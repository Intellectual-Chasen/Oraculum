import { useState } from "react";
import { InvestigationStages } from "@/features/investigationStages/InvestigationStages";
import { useInvestigationStages } from "@/features/investigationStages/useInvestigationStages";
import { SourceInventory } from "@/features/sourceInventory/SourceInventory";
import { useSourceInventory } from "@/features/sourceInventory/useSourceInventory";
import type { SourceIdentity } from "@/shared/contracts/sources";
import { ThemeToggle } from "@/shared/ui/ThemeToggle";
import { AccountBar } from "./AccountBar";
import { App } from "./App";
import { WorkspaceSession } from "./WorkspaceSession";

/** 読み込みを終えた収集元の一覧。一覧は読み込みの段階を終えた後に取得できる。 */
function LoadedSources() {
  const { state: inventory } = useSourceInventory();
  const [selectedSource, setSelectedSource] = useState<
    SourceIdentity | undefined
  >(undefined);
  return (
    <SourceInventory
      state={inventory}
      selectedSource={selectedSource}
      onSelect={setSelectedSource}
    />
  );
}

/**
 * 調査の段階の状態を持ち、段階に合う画面を出す。
 *
 * **処理の段階を終えるまで、段階の状態と段階を始める操作を出す。** グラフ・時系列・
 * レコードの応答は処理を終えた後に得られる。処理を終えた後は、ワークスペースを開いた調査の
 * 画面を出す。
 */
export function Root() {
  const stages = useInvestigationStages();
  const { state } = stages;
  if (
    state.status === "loaded" &&
    state.value.processing.state === "completed"
  ) {
    return (
      <WorkspaceSession>
        {(workspace) => <App {...workspace} />}
      </WorkspaceSession>
    );
  }
  return (
    <main className="app stages-page">
      <header className="import-page-header">
        <h1>Oraculum</h1>
        <AccountBar />
        <ThemeToggle />
      </header>
      <InvestigationStages view={stages} />
      {state.status === "loaded" &&
      state.value.loading.state === "completed" ? (
        <LoadedSources />
      ) : null}
    </main>
  );
}
