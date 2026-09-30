import {
  type DockviewApi,
  DockviewReact,
  type IDockviewHeaderActionsProps,
  type IDockviewPanelProps,
  type IDockviewReactProps,
  themeLight,
} from "dockview-react";
import {
  ExternalLink,
  Maximize2,
  Minimize2,
  PictureInPicture2,
  X,
} from "lucide-react";
import {
  createContext,
  type ReactNode,
  type RefObject,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from "react";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { PortalContainerContext } from "@/shared/ui/portalContainer";
import { StatusLabel } from "@/shared/ui/StatusLabel";

/** 配置の JSON の grid の節。size は親の並びの向きの長さである。 */
type SerializedGridNode = {
  type: "leaf" | "branch";
  data: unknown;
  size: number;
  visible?: boolean;
};

type Box = { width: number; height: number };

/**
 * 区画を最大にする前の各区画の大きさを覚え、元に戻したときにその大きさへ戻す。dockview は
 * 元に戻すときに区画を右端から表示し直し、要る幅を右端の区画から削る。大きさは配置が変わる
 * たびに、最大にしていない間だけ覚え直す。
 *
 * layout は保存する配置を返す。最大にしている間の toJSON() は、dockview が最大を解いたときの
 * 縮んだ大きさを持つので、各区画の大きさを覚えた大きさに置き換え、最大の印を除く。dispose は
 * 監視を止める。
 */
export function keepSizesAcrossMaximize(api: DockviewApi): {
  dispose: () => void;
  layout: () => unknown;
} {
  const sizes = new Map<string, Box>();
  const grid = () =>
    api.groups.filter((group) => group.api.location.type === "grid");
  const record = () => {
    if (api.hasMaximizedGroup()) return;
    sizes.clear();
    for (const group of grid()) {
      sizes.set(group.id, { width: group.api.width, height: group.api.height });
    }
  };
  record();
  const listeners = [
    api.onDidLayoutChange(record),
    api.onDidMaximizedGroupChange(({ isMaximized }) => {
      if (isMaximized) return;
      for (const group of grid()) {
        const size = sizes.get(group.id);
        if (size !== undefined) group.api.setSize(size);
      }
    }),
  ];
  // orientation は node の子を並べる向きである。覚えた大きさが無い区画を含む節は、そのままにする。
  const resize = (
    node: SerializedGridNode,
    orientation: string,
  ): Box | undefined => {
    if (node.visible === false) return undefined;
    let box: Box | undefined;
    if (node.type === "leaf") {
      box = sizes.get((node.data as { id: string }).id);
    } else {
      const inner = orientation === "HORIZONTAL" ? "VERTICAL" : "HORIZONTAL";
      const children = (node.data as SerializedGridNode[])
        .filter((child) => child.visible !== false)
        .map((child) => resize(child, inner));
      if (children.length === 0 || children.some((c) => c === undefined)) {
        return undefined;
      }
      const boxes = children as Box[];
      const sum = (key: keyof Box) =>
        boxes.reduce((total, item) => total + item[key], 0);
      const max = (key: keyof Box) =>
        Math.max(...boxes.map((item) => item[key]));
      box =
        orientation === "HORIZONTAL"
          ? { width: sum("width"), height: max("height") }
          : { width: max("width"), height: sum("height") };
    }
    if (box !== undefined) {
      node.size = orientation === "VERTICAL" ? box.width : box.height;
    }
    return box;
  };
  const layout = () => {
    const json = api.toJSON();
    if (!api.hasMaximizedGroup()) return json;
    // dockview の型は grid の最大の印 (maximizedNode) を宣言していない。
    const { maximizedNode: _maximized, ...grid } =
      json.grid as typeof json.grid & {
        maximizedNode?: unknown;
      };
    resize(grid.root as SerializedGridNode, grid.orientation);
    return { ...json, grid };
  };
  return {
    dispose: () => {
      for (const listener of listeners) listener.dispose();
    },
    layout,
  };
}

/** 配置できるビュー 1 つ。 */
export type DockPanel = {
  id: string;
  title: string;
  content: ReactNode;
  /** 隠れても中身を描き続けるビュー。入力中の値や図の配置を保つ。 */
  keepsMounted?: boolean;
  /**
   * 既定の配置と、開き直す操作に出さないビュー。保存した配置が含んでいれば、その配置のまま
   * 開く。
   */
  hidden?: boolean;
  /** このビューを含む区画に、最大にする操作を出す。 */
  maximizable?: boolean;
  /**
   * 開くときに加わる区画を持つビューの id。既定の配置には置かず、reveal で開いたときに、その
   * ビューと同じ区画にタブで重ねる。
   */
  column?: string;
};

/** reveal の要求。`onlyIfClosed` のときは、開いているビューを前面に出さない。 */
export type DockReveal = { panelId: string; onlyIfClosed?: boolean };

const PanelContents = createContext<ReadonlyMap<string, DockPanel>>(new Map());

/**
 * ビューの中身。keepsMounted でないビューは、隠れている間は描かない。隠れたビューが取得と
 * 描画を続けないようにする。
 */
function PanelBody({ api }: IDockviewPanelProps) {
  const panel = useContext(PanelContents).get(api.id);
  const [visible, setVisible] = useState(api.isVisible);
  useEffect(() => {
    const listener = api.onDidVisibilityChange((event) =>
      setVisible(event.isVisible),
    );
    setVisible(api.isVisible);
    return () => listener.dispose();
  }, [api]);
  const body = useRef<HTMLDivElement>(null);
  const getContainer = useOwnerBody(body, api);
  return (
    <div className="dock-panel" ref={body}>
      <PortalContainerContext value={getContainer}>
        {panel?.keepsMounted || visible ? panel?.content : null}
      </PortalContainerContext>
    </div>
  );
}

/**
 * 浮く要素を描く先 (要素を持つ文書の body) を返す関数。区画を別ウィンドウに出す・戻すと
 * 要素の文書が変わるので、置き場が変わるたびに読み直し、関数を替えて中の浮く要素を描き直させる。
 */
function useOwnerBody(
  element: RefObject<HTMLElement | null>,
  api: Pick<IDockviewPanelProps["api"], "onDidLocationChange">,
): () => HTMLElement | undefined {
  const [container, setContainer] = useState<HTMLElement>();
  useEffect(() => {
    const read = () => setContainer(element.current?.ownerDocument.body);
    read();
    // 置き場の知らせは、中身が新しい文書へ移る前に届くことがある。次の frame で読む。
    let frame: number | undefined;
    const listener = api.onDidLocationChange(() => {
      frame = window.requestAnimationFrame(read);
    });
    return () => {
      listener.dispose();
      if (frame !== undefined) window.cancelAnimationFrame(frame);
    };
  }, [api, element]);
  return useCallback(() => container, [container]);
}

const components = { body: PanelBody };

/**
 * 作業場所がビューの開閉と区画の移動を読み上げる文と、タブの閉じる button の名前。dockview の
 * 既定の文は英語である。キーボードで区画を動かす操作は有効にしていないため、その文は渡さない。
 */
const dockMessages: IDockviewReactProps["messages"] = {
  panelOpened: (title) => `${title} を開きました`,
  panelClosed: (title) => `${title} を閉じました`,
  groupMaximized: (title) => `${title} を最大化しました`,
  groupRestored: (title) => `${title} の大きさを復元しました`,
  groupFloated: (title) => `${title} をフロート表示にしました`,
  groupDocked: (title) => `${title} を作業場所に戻しました`,
  groupPoppedOut: (title) => `${title} を別ウィンドウに表示しました`,
  closeTab: (title) => `${title} を閉じる`,
  closeTabPlain: () => "閉じる",
};

/** 別ウィンドウを開けなかったことを作業場所へ知らせる。 */
const PopoutFailed = createContext<() => void>(() => {});

function GroupActions({
  api,
  containerApi,
  group,
  panels,
}: IDockviewHeaderActionsProps) {
  const location = api.location.type;
  const notifyPopoutFailed = useContext(PopoutFailed);
  const contents = useContext(PanelContents);
  const [maximized, setMaximized] = useState(api.isMaximized());
  useEffect(() => {
    const listener = containerApi.onDidMaximizedGroupChange(() =>
      setMaximized(api.isMaximized()),
    );
    return () => listener.dispose();
  }, [api, containerApi]);
  const maximizable = panels.some(
    (panel) => contents.get(panel.id)?.maximizable === true,
  );
  // ブラウザーがポップアップを止めると、dockview は区画を元の位置へ戻して false を返す。
  const popout = () =>
    containerApi.addPopoutGroup(group).then((opened) => {
      if (!opened) notifyPopoutFailed();
    }, notifyPopoutFailed);
  // 見出しの操作の tooltip を、別ウィンドウに出した区画ではそのウィンドウの文書に描く。
  const actions = useRef<HTMLDivElement>(null);
  const getContainer = useOwnerBody(actions, api);
  return (
    <div className="dock-actions" ref={actions}>
      <PortalContainerContext value={getContainer}>
        {location === "grid" && maximizable ? (
          <IconButton
            label={maximized ? "ビューの大きさを復元" : "ビューを最大化"}
            onPress={() => (maximized ? api.exitMaximized() : api.maximize())}
          >
            {maximized ? (
              <Minimize2 size={14} aria-hidden="true" />
            ) : (
              <Maximize2 size={14} aria-hidden="true" />
            )}
          </IconButton>
        ) : null}
        {location === "grid" ? (
          <IconButton
            label="フロート表示"
            onPress={() => containerApi.addFloatingGroup(group)}
          >
            <PictureInPicture2 size={14} aria-hidden="true" />
          </IconButton>
        ) : null}
        {location === "popout" ? null : (
          <IconButton label="別ウィンドウに表示" onPress={popout}>
            <ExternalLink size={14} aria-hidden="true" />
          </IconButton>
        )}
      </PortalContainerContext>
    </div>
  );
}

/**
 * タブの帯の端で一部だけ見えるタブを丸ごと隠す。dockview は一部だけ見えるタブも右端の一覧に
 * 載せるので、隠したタブは一覧から選べる。前面のタブは隠さず、帯を scroll して見せる。dockview は
 * 前面に出したときだけ scroll し、その後に帯の幅が変わると前面のタブが端で切れる。帯の大きさ、
 * タブの増減、scroll のたびに判定し直す。返す関数は監視を止める。
 */
export function hideClippedTabs(root: HTMLElement): () => void {
  // 大きさを測れない環境 (test の jsdom) では判定しない。
  if (typeof ResizeObserver === "undefined") return () => {};
  let frame = 0;
  const revealTimers = new Set<ReturnType<typeof setTimeout>>();
  const outside = (rect: DOMRect, bounds: DOMRect) =>
    rect.width > 0 &&
    (rect.left < bounds.left - 0.5 || rect.right > bounds.right + 0.5);
  // dockview は覚えた scroll の位置を animation frame の中で帯へ書き戻す。書き戻した後に scroll し、
  // dockview が scroll の event から新しい位置を覚えるようにする。帯以上の幅のタブは左端に合わせる
  // だけにする。右端にも合わせると、左右に合わせる scroll が交互に続く。
  const reveal = (list: HTMLElement) => {
    const bounds = list.getBoundingClientRect();
    const active = list
      .querySelector<HTMLElement>(".dv-active-tab")
      ?.getBoundingClientRect();
    if (active === undefined || !outside(active, bounds)) return;
    const alignLeft = active.width >= bounds.width || active.left < bounds.left;
    const delta = alignLeft
      ? active.left - bounds.left
      : active.right - bounds.right;
    if (Math.abs(delta) > 0.5) list.scrollLeft += delta;
  };
  const update = () => {
    frame = 0;
    for (const list of root.querySelectorAll<HTMLElement>(
      ".dv-tabs-container",
    )) {
      const bounds = list.getBoundingClientRect();
      for (const tab of list.querySelectorAll<HTMLElement>(".dv-tab")) {
        const clipped = outside(tab.getBoundingClientRect(), bounds);
        const active = tab.classList.contains("dv-active-tab");
        if (clipped && active) {
          const timer = setTimeout(() => {
            revealTimers.delete(timer);
            reveal(list);
          }, 0);
          revealTimers.add(timer);
        }
        tab.classList.toggle("dock-tab-clipped", clipped && !active);
      }
    }
  };
  const schedule = () => {
    if (frame === 0) frame = requestAnimationFrame(update);
  };
  const resize = new ResizeObserver(schedule);
  resize.observe(root);
  const mutation = new MutationObserver(() => {
    for (const list of root.querySelectorAll(".dv-tabs-container")) {
      resize.observe(list);
    }
    schedule();
  });
  mutation.observe(root, { childList: true, subtree: true });
  root.addEventListener("scroll", schedule, true);
  schedule();
  return () => {
    cancelAnimationFrame(frame);
    for (const timer of revealTimers) clearTimeout(timer);
    resize.disconnect();
    mutation.disconnect();
    root.removeEventListener("scroll", schedule, true);
  };
}

function addPanel(api: DockviewApi, panel: DockPanel) {
  return api.addPanel(panelOptions(panel));
}

/**
 * ビューを前面に出す。閉じていれば開き直す。
 *
 * **下の区画のビューは、下の区画に戻す。** 閉じたビューと、保存した配置に無いビューを開くとき、
 * 既定の配置で下に置くビューは、下に置いたほかのビューの隣に足す。下のビューが 1 つも開いて
 * いなければ、中央のビューの下に区画を作る。位置を与えないと、そのとき前面の区画 (Detail など)
 * に入る。
 */
function openPanel(
  api: DockviewApi | undefined,
  panel: DockPanel,
  panels: readonly DockPanel[],
  onlyIfClosed: boolean,
) {
  if (api === undefined) return;
  const opened = api.getPanel(panel.id);
  if (opened !== undefined) {
    if (!onlyIfClosed) opened.api.setActive();
    return;
  }
  const column =
    panel.column === undefined ? undefined : api.getPanel(panel.column);
  api
    .addPanel({
      ...panelOptions(panel),
      ...(column === undefined
        ? belowPosition(api, panel, panels)
        : {
            position: {
              referencePanel: column.id,
              direction: "within" as const,
            },
          }),
    })
    .api.setActive();
}

/** 既定の配置に置くビュー。隠したビューと、別のビューの区画に開くビューを除く。 */
function defaultPanels(panels: readonly DockPanel[]): DockPanel[] {
  return panels.filter((item) => !item.hidden && item.column === undefined);
}

/** 既定の配置で下に置くビューを開き直す位置。下のビューでなければ位置を与えない。 */
function belowPosition(
  api: DockviewApi,
  panel: DockPanel,
  panels: readonly DockPanel[],
) {
  const [, center, , ...below] = defaultPanels(panels);
  if (!below.some((item) => item.id === panel.id)) return {};
  const sibling = below.find((item) => api.getPanel(item.id) !== undefined);
  if (sibling !== undefined) {
    return {
      position: { referencePanel: sibling.id, direction: "within" as const },
    };
  }
  if (center !== undefined && api.getPanel(center.id) !== undefined) {
    return {
      position: { referencePanel: center.id, direction: "below" as const },
    };
  }
  return {};
}

/**
 * 既定の配置。先頭 3 つを左・中央・右の列に並べ、残りを中央の列の下にタブで重ねる。
 */
function addDefaultLayout(api: DockviewApi, all: readonly DockPanel[]) {
  const panels = defaultPanels(all);
  const [left, center, right, ...below] = panels;
  if (left === undefined || center === undefined || right === undefined) {
    for (const panel of panels) addPanel(api, panel);
    return;
  }
  addPanel(api, left);
  api.addPanel({
    ...panelOptions(center),
    position: { referencePanel: left.id, direction: "right" },
  });
  api.addPanel({
    ...panelOptions(right),
    position: { referencePanel: center.id, direction: "right" },
  });
  const [first, ...rest] = below;
  if (first !== undefined) {
    api.addPanel({
      ...panelOptions(first),
      position: { referencePanel: center.id, direction: "below" },
    });
    for (const panel of rest) {
      api.addPanel({
        ...panelOptions(panel),
        position: { referencePanel: first.id, direction: "within" },
        inactive: true,
      });
    }
    // inactive で足しても、同じ区画の最後のビューが前面に出ることがある。先頭を前面に戻す。
    api.getPanel(first.id)?.api.setActive();
  }
  api.getPanel(left.id)?.api.setSize({ width: 320 });
  api.getPanel(right.id)?.api.setSize({ width: 420 });
  if (first !== undefined) {
    api
      .getPanel(first.id)
      ?.api.setSize({ height: Math.round(api.height * 0.38) });
  }
}

function panelOptions(panel: DockPanel) {
  return {
    id: panel.id,
    title: panel.title,
    component: "body",
    renderer: panel.keepsMounted
      ? ("always" as const)
      : ("onlyWhenVisible" as const),
  };
}

/**
 * 開いた配置の各区画で前面にあるビューの中身を、区画に描き直す。
 *
 * dockview 8.3.1 の fromJSON は、前面に出ているときだけ描くビューを区画の前面に置いても、
 * その中身を区画に付けないことがある。付けないビューは、別のタブへ切り替えて戻すまで空の区画に
 * なる。描き方を 1 度切り替えて戻し、区画に中身を付け直させる。
 *
 * 既知の制限: dockview の不具合を、描き方の切り替えで避けている, Artifacts を前面にして保存した
 * 配置を画面で開き直し、区画が空になることを確かめた, dockview を更新したときに、この描き直しを
 * 外して同じ手順で確かめる
 */
function redrawActivePanels(api: DockviewApi): void {
  for (const group of api.groups) {
    const active = group.activePanel;
    if (active === undefined || active.api.renderer !== "onlyWhenVisible") {
      continue;
    }
    active.api.setRenderer("always");
    active.api.setRenderer("onlyWhenVisible");
  }
}

/**
 * 配置を開く。layout が undefined のときは既定の配置で開く。読めない配置と、知らないビューを
 * 含む配置は捨て、既定の配置で開いて false を返す。
 */
export function restoreLayout(
  api: DockviewApi,
  panels: readonly DockPanel[],
  layout: unknown,
): boolean {
  if (layout !== undefined) {
    try {
      // 配置の形は fromJSON が確かめ、読めなければ投げる。投げた配置は下で既定の配置に置き換える。
      api.fromJSON(layout as Parameters<DockviewApi["fromJSON"]>[0]);
      // 最大にした状態で開くと、元に戻すときに戻す大きさが分からない。最大を解いて開く。
      if (api.hasMaximizedGroup()) api.exitMaximizedGroup();
      const titles = new Map(panels.map((panel) => [panel.id, panel.title]));
      if (api.panels.every((panel) => titles.has(panel.id))) {
        // 保存した配置はタブの名前も持つ。名前を変えた後でも今の名前で出す。
        for (const panel of api.panels) {
          panel.api.setTitle(titles.get(panel.id) ?? panel.title ?? "");
        }
        redrawActivePanels(api);
        return true;
      }
    } catch {
      // 壊れた配置は下で既定の配置に置き換える。
    }
  }
  api.clear();
  addDefaultLayout(api, panels);
  return layout === undefined;
}

/**
 * ビューを分割・タブ・浮かせた区画・別ウィンドウに置ける作業場所。閉じたビューは reveal で
 * 開き直す。
 *
 * reveal を新しい値に変えると、そのビューを前面に出す。閉じていれば開き直す。resetRequest を
 * 新しい値に変えると、配置を初めの形に戻す。
 * layoutRequest を新しい値に変えると、その配置を開く。開いたときに layoutRequest が無ければ
 * 既定の配置で開く。配置が変わるたびに onLayoutChange へ `toJSON()` の値を知らせる。
 */
export function DockWorkspace({
  panels,
  reveal,
  resetRequest,
  layoutRequest,
  onLayoutChange,
}: {
  panels: readonly DockPanel[];
  reveal?: DockReveal;
  /** 配置を初めの形に戻す要求。メニューバーのように作業場所の外の操作が渡す。 */
  resetRequest?: object;
  layoutRequest?: { layout: unknown };
  onLayoutChange?: (layout: unknown) => void;
}) {
  const apiRef = useRef<DockviewApi | undefined>(undefined);
  const listenersRef = useRef<{ dispose: () => void }[]>([]);
  const panelsRef = useRef(panels);
  panelsRef.current = panels;
  const onLayoutChangeRef = useRef(onLayoutChange);
  onLayoutChangeRef.current = onLayoutChange;
  const appliedRequestRef = useRef(layoutRequest);
  const [popoutFailed, setPopoutFailed] = useState(false);
  const [layoutFailed, setLayoutFailed] = useState(false);
  const notifyPopoutFailed = useCallback(() => setPopoutFailed(true), []);
  const contents = new Map(panels.map((panel) => [panel.id, panel]));
  const keptRef = useRef<ReturnType<typeof keepSizesAcrossMaximize>>(undefined);
  // 最大にしている間も、最大にする前の大きさの配置として知らせる。
  const save = (api: DockviewApi) => {
    onLayoutChangeRef.current?.(keptRef.current?.layout() ?? api.toJSON());
  };

  const onReady = ({ api }: { api: DockviewApi }) => {
    apiRef.current = api;
    appliedRequestRef.current = layoutRequest;
    setLayoutFailed(
      !restoreLayout(api, panelsRef.current, layoutRequest?.layout),
    );
    const kept = keepSizesAcrossMaximize(api);
    keptRef.current = kept;
    save(api);
    listenersRef.current = [kept, api.onDidLayoutChange(() => save(api))];
  };
  // biome-ignore lint/correctness/useExhaustiveDependencies: 新しい組を渡したときだけ動かす。
  useEffect(() => {
    const api = apiRef.current;
    if (
      api === undefined ||
      layoutRequest === undefined ||
      layoutRequest === appliedRequestRef.current
    ) {
      return;
    }
    appliedRequestRef.current = layoutRequest;
    setLayoutFailed(
      !restoreLayout(api, panelsRef.current, layoutRequest.layout),
    );
    save(api);
  }, [layoutRequest]);
  useEffect(
    () => () => {
      for (const listener of listenersRef.current) listener.dispose();
      listenersRef.current = [];
    },
    [],
  );

  useEffect(() => {
    const panel = panelsRef.current.find((item) => item.id === reveal?.panelId);
    if (panel !== undefined) {
      openPanel(
        apiRef.current,
        panel,
        panelsRef.current,
        reveal?.onlyIfClosed === true,
      );
    }
  }, [reveal]);

  const workspaceRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const root = workspaceRef.current;
    if (root === null) return;
    return hideClippedTabs(root);
  }, []);

  const resetLayout = useCallback(() => {
    const api = apiRef.current;
    if (api === undefined) return;
    api.clear();
    addDefaultLayout(api, panelsRef.current);
  }, []);

  useEffect(() => {
    if (resetRequest !== undefined) resetLayout();
  }, [resetRequest, resetLayout]);

  const notices = [
    layoutFailed && {
      label: "保存した配置の読み込み失敗",
      details: [{ name: "表示", value: "初期の配置" }],
      close: () => setLayoutFailed(false),
    },
    popoutFailed && {
      label: "別ウィンドウの表示失敗",
      details: [
        { name: "確認", value: "ブラウザーのポップアップの設定" },
        { name: "ビュー", value: "元の位置" },
      ],
      close: () => setPopoutFailed(false),
    },
  ].filter((notice) => notice !== false);
  return (
    <div className="dock-workspace" ref={workspaceRef}>
      {notices.length === 0 ? null : (
        <div className="fixed right-4 bottom-4 z-50 flex max-w-md flex-col gap-2">
          {notices.map((notice) => (
            <div
              key={notice.label}
              role="alert"
              className="flex items-center gap-2 rounded-md border border-line bg-surface px-3 py-2 text-sm text-ink shadow-float"
            >
              <StatusLabel
                className="flex-1"
                status="failed"
                label={notice.label}
                details={<KeyValueList stacked pairs={notice.details} />}
              />
              <IconButton label="知らせを閉じる" onPress={notice.close}>
                <X size={14} aria-hidden="true" />
              </IconButton>
            </div>
          ))}
        </div>
      )}
      <PopoutFailed.Provider value={notifyPopoutFailed}>
        <PanelContents.Provider value={contents}>
          <DockviewReact
            className="dock-view"
            theme={themeLight}
            messages={dockMessages}
            popoutUrl="/popout.html"
            components={components}
            rightHeaderActionsComponent={GroupActions}
            onReady={onReady}
          />
        </PanelContents.Provider>
      </PopoutFailed.Provider>
    </div>
  );
}
