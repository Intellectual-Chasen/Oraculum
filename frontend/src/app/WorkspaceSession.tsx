import {
  Check,
  Copy,
  Download,
  Eye,
  EyeOff,
  FilePlus2,
  FolderInput,
  FolderOpen,
  Pencil,
  RefreshCw,
  Share2,
  Trash2,
  Upload,
  Users,
  X,
} from "lucide-react";
import {
  Fragment,
  type ReactNode,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";
import { Dialog, DialogTrigger, Popover } from "react-aria-components";
import {
  createWorkspace,
  deleteWorkspace,
  fetchWorkspace,
  fetchWorkspaces,
  postWorkspaceChange,
  postWorkspacePresence,
  replaceWorkspace,
  subscribeWorkspaceEvents,
} from "@/shared/api/workspaces";
import { DecodeFailure } from "@/shared/contracts/decoding";
import {
  decodeWorkspaceConflict,
  type WorkspaceAccess,
  type WorkspaceConnection,
  type WorkspaceEvent,
  type WorkspaceItem,
  type WorkspacePatch,
  type WorkspaceSummary,
} from "@/shared/contracts/workspaces";
import type { FetchFailure } from "@/shared/lib/fetchState";
import { Button } from "@/shared/ui/Button";
import { ConfirmIconButton } from "@/shared/ui/ConfirmIconButton";
import { DataTable, type DataTableColumn } from "@/shared/ui/DataTable";
import { FetchFailureNotice } from "@/shared/ui/FetchFailureNotice";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { usePortalContainer } from "@/shared/ui/portalContainer";
import { RawText } from "@/shared/ui/RawText";
import { useSignedIn } from "@/shared/ui/SignedInAccount";
import { StatusDot } from "@/shared/ui/StatusDot";
import { StatusLabel } from "@/shared/ui/StatusLabel";
import { ThemeToggle } from "@/shared/ui/ThemeToggle";
import { AccountBar } from "./AccountBar";
import { shareAccessLabels, WorkspaceShares } from "./WorkspaceShares";
import { decodeWorkspaceState, type WorkspaceState } from "./workspaceState";
import {
  applyPatch,
  changedFields,
  fieldLabelList,
  fitsPresence,
  patchOf,
  type StateFields,
  selectionOf,
  type WorkspaceSelection,
  withoutNulls,
  withSelection,
} from "./workspaceSync";

/** 画面の変化から保存を始めるまでの待ち時間。 */
export const workspaceSaveDelayMs = 1000;

/** 選択の変化から、選択を他の接続へ知らせるまでの待ち時間。 */
export const presenceDelayMs = 500;

/** 他の利用者の変更を知らせる帯の表示を消すまでの時間。 */
export const changeNoticeMs = 4000;

const keepaliveBodyLimit = 64 * 1024;

/** ブラウザーは keepalive の要求の本文を 64 KiB までに限る。本文がその中に収まるかを返す。 */
export function fitsKeepalive(body: unknown): boolean {
  return new Blob([JSON.stringify(body)]).size <= keepaliveBodyLimit;
}

/** 前回開いていたワークスペースの id を置く localStorage の鍵。所有者ごとに分ける。 */
export function lastWorkspaceStorageKey(owner: string): string {
  return `oraculum.lastWorkspace.${owner}`;
}

/** 調査の画面に渡す、ワークスペースの状態と操作。 */
export type WorkspaceAppProps = {
  workspaceState?: { state: WorkspaceState; revision: number };
  onWorkspaceStateChange: (state: WorkspaceState) => void;
  workspaceBar: ReactNode;
  workspacePanel: ReactNode;
};

type Phase =
  | { kind: "loading" }
  | { kind: "failed"; failure: FetchFailure }
  | { kind: "unreadable"; summary: WorkspaceSummary }
  | {
      kind: "open";
      summary: WorkspaceSummary;
      applied?: WorkspaceState;
      /** 画面を作り直す番号。ワークスペースを開くたびに変わる。 */
      mount: number;
      /** 画面へ状態を適用する番号。開くたびと、配信や追従で状態を重ねるたびに変わる。 */
      revision: number;
    };

type SaveStatus =
  | { kind: "idle" }
  | { kind: "saving" }
  | { kind: "saved" }
  | { kind: "failed"; failure: FetchFailure }
  /** 閲覧だけの共有として開いている。downgraded は保存の途中で権限が閲覧だけに変わったことを表す。 */
  | { kind: "viewOnly"; downgraded?: boolean }
  /** 開いている共有が取り消された。 */
  | { kind: "revoked" }
  /** 開いているワークスペースを所有者が削除した。 */
  | { kind: "deleted" };

/** 保存先のワークスペース。revision は送る変更の元にする値である。 */
type Current = {
  id: string;
  name: string;
  revision: number;
  access: WorkspaceAccess;
};

/** 送った変更 1 件。送れずに再送するときも同じ clientChangeId を使う。 */
type OutgoingChange = {
  clientChangeId: string;
  baseRevision: number;
  patch: WorkspacePatch;
};

/**
 * 同じ欄の変更の競合。lost は競合した欄の自分の値、theirs は相手の値である。incoming は、
 * 送る前の自分の欄へ相手の変更が届いたことを表す。その欄は、利用者が選ぶまで送らない。
 */
type ConflictNotice = {
  who: string;
  fields: string[];
  lost: WorkspacePatch;
  theirs: WorkspacePatch;
  incoming: boolean;
};

/** 追従している接続と、追従を始める前の自分の選択と見た場所の履歴。 */
type Following = {
  connectionId: string;
  own: WorkspaceSelection;
  ownHistory: unknown;
};

const conflictValueLength = 200;

/** 利用者が読めない値を持つフィールド。競合の通知では値を出さず、名前だけを出す。 */
const valuelessFields = new Set(["dockLayout"]);

/** フィールドの値を、通知に出す長さまで詰めた文字列で返す。文字列の値は引用符を付けずに出す。 */
function summarize(patch: WorkspacePatch, field: string): string {
  if (valuelessFields.has(field)) return "";
  const value = patch[field];
  const text =
    typeof value === "string" ? value : (JSON.stringify(value) ?? "なし");
  return text.length <= conflictValueLength
    ? text
    : `${text.slice(0, conflictValueLength)}…`;
}

/** 通知の表に並べるフィールド。全体の競合では、自分の値を持つフィールドを並べる。 */
function conflictRowFields(notice: ConflictNotice): string[] {
  return notice.fields.includes("*") ? Object.keys(notice.lost) : notice.fields;
}

/**
 * keepalive の本文に収まる変更を返す。収まらなければ、件数の上限を持たない見た場所の履歴を外し、
 * 新しい clientChangeId を付けて返す。外しても収まらないか、残るフィールドが無ければ undefined を
 * 返す。外した履歴は server と揃えた状態を変えないので、送っていない値として残る。
 */
function withoutLargeHistory(
  change: OutgoingChange,
): OutgoingChange | undefined {
  if (fitsKeepalive(change)) return change;
  const { history: _history, ...patch } = change.patch;
  if (Object.keys(patch).length === 0) return undefined;
  const rest = { ...change, clientChangeId: crypto.randomUUID(), patch };
  return fitsKeepalive(rest) ? rest : undefined;
}

function summaryOf(item: WorkspaceItem): WorkspaceSummary {
  const { state: _state, shares: _shares, ...summary } = item;
  return summary;
}

/** 作った直後のワークスペースは空の object を持ち、画面へ適用するものが無い。 */
function isEmptyState(state: unknown): boolean {
  return (
    typeof state === "object" &&
    state !== null &&
    Object.keys(state).length === 0
  );
}

/**
 * 読める状態なら画面の状態として返す。読めなければ undefined を返す。画面の状態は値が
 * undefined の欄を持つため、保存するときと同じ JSON の文字列を経て読む。
 */
function readState(state: StateFields): WorkspaceState | undefined {
  try {
    return decodeWorkspaceState(JSON.parse(JSON.stringify(state)));
  } catch (cause) {
    if (cause instanceof DecodeFailure) return undefined;
    throw cause;
  }
}

/**
 * 変更の元にする revision を、受け取った変更が連続するときだけ進める。間の revision の変更を
 * まだ受け取っていなければ進めず、次の送信で server に同じ欄の競合を判定させる。
 */
function advanceBase(target: Current, revision: number) {
  if (revision === target.revision + 1) target.revision = revision;
}

const saveStatusText = {
  idle: "",
  saving: "保存中",
  saved: "保存済み",
  failed: "保存失敗",
  viewOnly: "",
  revoked: "保存停止",
  deleted: "保存停止",
} as const;

/**
 * 調査の画面を、いずれかのワークスペースを開いた状態で出す。
 *
 * 開く対象は、前回開いていたワークスペース、一覧の先頭、新しく作ったワークスペースの順に
 * 決める。画面の変化は待ち時間の後に、server と揃えた状態から変わった最上位の欄だけを送る。
 * 開いている間は配信を購読し、他の接続の変更を、自分が送っていない欄を除いて画面へ重ねる。
 * 別の接続が同じ欄を先に変更していたときは最新を画面へ適用し、反映できなかった自分の値を
 * 通知する。読めない状態を持つワークスペースは画面へ適用せず、上書きもしない。
 */
export function WorkspaceSession({
  children,
}: {
  children: (props: WorkspaceAppProps) => ReactNode;
}) {
  const signedIn = useSignedIn();
  const owner = signedIn?.account.login ?? "local";
  const storageKey = lastWorkspaceStorageKey(owner);
  const [phase, setPhase] = useState<Phase>({ kind: "loading" });
  const [status, setStatus] = useState<SaveStatus>({ kind: "idle" });
  const [list, setList] = useState<WorkspaceSummary[]>([]);
  const [actionFailure, setActionFailure] = useState<FetchFailure>();
  const [conflictNotice, setConflictNotice] = useState<ConflictNotice>();
  const [changeNotice, setChangeNotice] = useState<{
    who: string;
    fields: string;
  }>();
  const [connections, setConnections] = useState<WorkspaceConnection[]>([]);
  const [following, setFollowing] = useState<Following>();
  const [followEnded, setFollowEnded] = useState<string>();
  const listRef = useRef(list);
  const current = useRef<Current | undefined>(undefined);
  // server と揃えた状態。送った変更の成功、配信、競合の応答で更新する。
  const synced = useRef<StateFields | undefined>(undefined);
  // 開いた直後の通知で画面が書き出し直した欄の文字列。この文字列のままの欄は送らない。
  const ignored = useRef<Record<string, string>>({});
  // 配信で受け取った最後の revision。これ以下の事象は受け取り済みである。
  const streamRevision = useRef(0);
  const unsent = useRef<OutgoingChange | undefined>(undefined);
  const ownChangeIds = useRef(new Set<string>());
  // 相手の変更が届いた、自分の送っていない欄。利用者が選ぶまで送らない。
  const held = useRef(new Set<string>());
  // 画面へ適用した server と揃えた値の欄。次の通知の値を揃えた値として扱う。
  const absorbNext = useRef<string[]>([]);
  const latestLocal = useRef<WorkspaceState | undefined>(undefined);
  const skipsNext = useRef(true);
  // 閲覧だけの共有と、取り消された共有を開いている間は保存しない。
  const readOnly = useRef(false);
  // 変更を送れなかった最後の失敗。送れたら消す。
  const blocked = useRef<FetchFailure | undefined>(undefined);
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const chain = useRef<Promise<void>>(Promise.resolve());
  const mounts = useRef(0);
  const started = useRef(false);
  const connectionId = useRef("");
  const connectionsRef = useRef<WorkspaceConnection[]>([]);
  const followingRef = useRef<Following | undefined>(undefined);
  const presenceTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const sentPresence = useRef<string | undefined>(undefined);
  const noticeTimer = useRef<ReturnType<typeof setTimeout>>(undefined);

  const changeList = useCallback(
    (change: (items: WorkspaceSummary[]) => WorkspaceSummary[]) => {
      listRef.current = change(listRef.current);
      setList(listRef.current);
    },
    [],
  );
  const putInList = useCallback(
    (summary: WorkspaceSummary) =>
      changeList((items) =>
        items.some((item) => item.id === summary.id)
          ? items.map((item) => (item.id === summary.id ? summary : item))
          : [...items, summary],
      ),
    [changeList],
  );

  /** ログイン名から、接続中の利用者の表示名を探す。見つからなければログイン名を返す。 */
  const nameOf = useCallback(
    (login: string) =>
      connectionsRef.current.find((item) => item.login === login)
        ?.displayName ?? login,
    [],
  );

  /** 送る状態。追従している間は、選択と見た場所の履歴を server と揃えた値のままにする。 */
  const outgoing = useCallback((): StateFields | undefined => {
    const local = latestLocal.current;
    const base = synced.current;
    if (local === undefined || base === undefined) return undefined;
    if (followingRef.current === undefined) return local;
    const state = withSelection(local, selectionOf(base));
    return applyPatch(state, { history: base.history ?? null });
  }, []);

  /**
   * server と揃えた状態から変わった欄。画面が書き出し直しただけの欄を除く。何も送らない間は
   * 送っていない欄を持たない。
   */
  const unsentFields = useCallback(
    (base = synced.current): string[] => {
      const state = outgoing();
      if (readOnly.current || state === undefined || base === undefined)
        return [];
      // 除くのは、書き出し直した値として覚えた欄だけである。値を外した欄は JSON にすると
      // undefined になり、覚えていない欄の undefined と比べると同じになる。先に鍵の有無を見る。
      return changedFields(state, base).filter(
        (key) =>
          !Object.hasOwn(ignored.current, key) ||
          JSON.stringify(state[key]) !== ignored.current[key],
      );
    },
    [outgoing],
  );

  /**
   * 状態を画面へ適用する。追従している間は、追従している接続の選択を重ねる。読めない状態と、
   * 今の画面と同じ状態は適用しない。
   */
  const applyToScreen = useCallback((state: StateFields) => {
    const followed = followingRef.current;
    const selection =
      followed === undefined
        ? undefined
        : connectionsRef.current.find(
            (item) => item.connectionId === followed.connectionId,
          )?.selection;
    const shown = readState(
      selection === undefined ? state : withSelection(state, selection),
    );
    if (shown === undefined) return;
    if (JSON.stringify(shown) === JSON.stringify(latestLocal.current)) return;
    latestLocal.current = shown;
    // 画面は適用した値を書き出し直して知らせる。送っていない欄に数えない欄は、次の通知でも
    // 揃えた値として扱う。
    const base = synced.current ?? {};
    absorbNext.current = Object.keys(shown).filter((key) => {
      const json = JSON.stringify((shown as StateFields)[key]);
      return (
        json === JSON.stringify(base[key]) || json === ignored.current[key]
      );
    });
    mounts.current += 1;
    const revision = mounts.current;
    setPhase((open) =>
      open.kind === "open" ? { ...open, applied: shown, revision } : open,
    );
  }, []);

  const showChange = useCallback((who: string, fields: string) => {
    clearTimeout(noticeTimer.current);
    setChangeNotice({ who, fields });
    noticeTimer.current = setTimeout(
      () => setChangeNotice(undefined),
      changeNoticeMs,
    );
  }, []);

  /** 開いている共有が取り消されたか、ワークスペースが削除された。保存を止め、一覧から外す。 */
  const markGone = useCallback(
    (target: Current, kind: "revoked" | "deleted") => {
      readOnly.current = true;
      unsent.current = undefined;
      blocked.current = undefined;
      changeList((items) => items.filter((item) => item.id !== target.id));
      setStatus({ kind });
    },
    [changeList],
  );

  /**
   * 競合の応答が含む最新の状態を採用する。競合しなかった自分の送っていない欄は保ち、
   * 競合した欄の自分の値を通知に出す。競合の記録を読めなければ偽を返す。merge が真なら、同じ
   * 保存の中の前の競合の通知に重ねる。同じフィールドは最初に失った自分の値を残す。
   */
  const adoptConflict = useCallback(
    (
      target: Current,
      change: OutgoingChange,
      failure: FetchFailure,
      merge: boolean,
    ): boolean => {
      let conflict: ReturnType<typeof decodeWorkspaceConflict>;
      try {
        conflict = decodeWorkspaceConflict(failure.conflict, "conflict");
      } catch (cause) {
        if (!(cause instanceof DecodeFailure)) throw cause;
        return false;
      }
      const latest = withoutNulls(conflict.state);
      if (!isEmptyState(latest) && readState(latest) === undefined) {
        return false;
      }
      const lostFields = conflict.fields.includes("*")
        ? Object.keys(change.patch)
        : conflict.fields.filter((field) => field in change.patch);
      if (conflict.fields.includes("*")) held.current.clear();
      for (const field of conflict.fields) held.current.delete(field);
      const keep = unsentFields().filter(
        (field) => !lostFields.includes(field),
      );
      const local = outgoing() ?? {};
      synced.current = latest;
      ignored.current = {};
      target.revision = conflict.revision;
      streamRevision.current = Math.max(
        streamRevision.current,
        conflict.revision,
      );
      unsent.current = undefined;
      blocked.current = undefined;
      applyToScreen(applyPatch(latest, patchOf(local, keep)));
      const next: ConflictNotice = {
        who: nameOf(conflict.updatedBy),
        fields: conflict.fields,
        lost: patchOf(change.patch, lostFields),
        theirs: patchOf(latest, lostFields),
        incoming: false,
      };
      setConflictNotice((previous) =>
        !merge || previous === undefined || previous.incoming
          ? next
          : {
              ...next,
              fields: [
                ...previous.fields,
                ...next.fields.filter(
                  (field) => !previous.fields.includes(field),
                ),
              ],
              lost: { ...next.lost, ...previous.lost },
              theirs: { ...previous.theirs, ...next.theirs },
            },
      );
      setStatus({ kind: "idle" });
      return true;
    },
    [unsentFields, outgoing, applyToScreen, nameOf],
  );

  /** 送れる欄。利用者が選ぶのを待つ欄を除く。base は送り済みと見なす状態である。 */
  const sendableFields = useCallback(
    (base = synced.current) =>
      unsentFields(base).filter((field) => !held.current.has(field)),
    [unsentFields],
  );

  /** base との差を持つ新しい変更を作る。送る欄が無ければ undefined を返す。 */
  const newChange = useCallback(
    (target: Current, base = synced.current): OutgoingChange | undefined => {
      const fields = sendableFields(base);
      const state = outgoing();
      if (fields.length === 0 || state === undefined) return undefined;
      const change = {
        clientChangeId: crypto.randomUUID(),
        baseRevision: target.revision,
        patch: patchOf(state, fields),
      };
      // 配信は応答より先に自分の変更を届けることがある。送る前に自分の変更として覚える。
      ownChangeIds.current.add(change.clientChangeId);
      return change;
    },
    [sendableFields, outgoing],
  );

  /** 送る変更を決める。再送を待つ変更があればそれを返す。 */
  const nextChange = useCallback(
    (target: Current): OutgoingChange | undefined => {
      unsent.current ??= newChange(target);
      return unsent.current;
    },
    [newChange],
  );

  /**
   * 送っていない変更を送る。送り終えるまで次の保存を始めない。送れずに変更が残ったときは、
   * その失敗を返す。
   */
  const save = useCallback((): Promise<FetchFailure | undefined> => {
    clearTimeout(timer.current);
    chain.current = chain.current.then(async () => {
      // 競合の後に残ったフィールドを、競合が何回続いても送り続ける。2 回目からの競合は通知に重ねる。
      let conflicted = false;
      for (;;) {
        const target = current.current;
        if (target === undefined || readOnly.current) return;
        const change = nextChange(target);
        if (change === undefined) return;
        setStatus({ kind: "saving" });
        // 送る間にページを離れても、要求を最後まで送る。
        const result = await postWorkspaceChange(target.id, change, {
          keepalive: fitsKeepalive(change),
        });
        if (current.current !== target) return;
        if (result.ok) {
          unsent.current = undefined;
          blocked.current = undefined;
          synced.current = applyPatch(synced.current ?? {}, change.patch);
          for (const key of Object.keys(change.patch)) {
            delete ignored.current[key];
          }
          advanceBase(target, result.value.revision);
          setStatus({ kind: "saved" });
          continue;
        }
        const code = result.failure.failureCode;
        if (target.access !== "owner" && code === "record_not_found") {
          markGone(target, "revoked");
          return;
        }
        if (target.access !== "owner" && code === "permission_denied") {
          // 所有者が共有を閲覧だけに変えた。以後は閲覧だけの共有として扱う。
          readOnly.current = true;
          unsent.current = undefined;
          blocked.current = undefined;
          target.access = "view";
          setPhase((shown) =>
            shown.kind === "open" && shown.summary.id === target.id
              ? { ...shown, summary: { ...shown.summary, access: "view" } }
              : shown,
          );
          changeList((items) =>
            items.map((item) =>
              item.id === target.id ? { ...item, access: "view" } : item,
            ),
          );
          setStatus({ kind: "viewOnly", downgraded: true });
          return;
        }
        if (
          code === "workspace_changed" &&
          adoptConflict(target, change, result.failure, conflicted)
        ) {
          conflicted = true;
          continue;
        }
        blocked.current = result.failure;
        setStatus({ kind: "failed", failure: result.failure });
        return;
      }
    });
    return chain.current.then(() =>
      unsent.current !== undefined ? blocked.current : undefined,
    );
  }, [nextChange, markGone, changeList, adoptConflict]);

  const scheduleSave = useCallback(() => {
    clearTimeout(timer.current);
    timer.current = setTimeout(() => void save(), workspaceSaveDelayMs);
  }, [save]);

  /** 自分の選択と追従先を、待ち時間の後に他の接続へ知らせる。前回と同じなら送らない。 */
  const schedulePresence = useCallback(() => {
    if (presenceTimer.current !== undefined) return;
    presenceTimer.current = setTimeout(() => {
      presenceTimer.current = undefined;
      const target = current.current;
      const local = latestLocal.current;
      if (target === undefined || connectionId.current === "") return;
      const followed = followingRef.current;
      const selection =
        followed?.own ?? (local === undefined ? {} : selectionOf(local));
      if (!fitsPresence(selection)) return;
      const body = {
        connectionId: connectionId.current,
        selection,
        following: followed?.connectionId ?? null,
      };
      const json = JSON.stringify(body);
      if (json === sentPresence.current) return;
      sentPresence.current = json;
      void postWorkspacePresence(target.id, body);
    }, presenceDelayMs);
  }, []);

  /** 追従を終え、自分の選択に戻す。reason は終えた理由として帯に出す。 */
  const stopFollowing = useCallback(
    (reason?: string) => {
      const followed = followingRef.current;
      if (followed === undefined) return;
      followingRef.current = undefined;
      setFollowing(undefined);
      setFollowEnded(reason);
      const local = latestLocal.current;
      if (local !== undefined)
        applyToScreen(
          applyPatch(withSelection(local, followed.own), {
            history: followed.ownHistory ?? null,
          }),
        );
      schedulePresence();
    },
    [applyToScreen, schedulePresence],
  );

  const startFollowing = (target: WorkspaceConnection) => {
    const local = latestLocal.current;
    const previous = followingRef.current;
    const followed = {
      connectionId: target.connectionId,
      own: previous?.own ?? (local === undefined ? {} : selectionOf(local)),
      ownHistory: previous === undefined ? local?.history : previous.ownHistory,
    };
    followingRef.current = followed;
    setFollowing(followed);
    setFollowEnded(undefined);
    if (local !== undefined) applyToScreen(local);
    schedulePresence();
  };

  /**
   * 他の接続の変更を、server と揃えた状態へ重ね、自分が送っていない欄を除いて画面へ重ねる。
   * 自分が送っていない欄に届いた変更は、その場で競合として通知し、利用者が選ぶまで送らない。
   * 値が変わった欄を返す。
   */
  const receive = useCallback(
    (patch: WorkspacePatch, who: string): string[] => {
      const base = synced.current ?? {};
      const mine = unsentFields();
      const next = applyPatch(base, patch);
      const fields = changedFields(base, next);
      const screen = outgoing() ?? next;
      // 送信中の欄の競合は、server が 409 で判定する。
      const sending = unsent.current?.patch ?? {};
      const overlap = fields.filter(
        (field) =>
          mine.includes(field) &&
          !(field in sending) &&
          JSON.stringify(screen[field]) !== JSON.stringify(next[field]),
      );
      if (overlap.length > 0) {
        for (const field of overlap) held.current.add(field);
        setConflictNotice({
          who: nameOf(who),
          fields: overlap,
          lost: patchOf(screen, overlap),
          theirs: patchOf(next, overlap),
          incoming: true,
        });
      }
      synced.current = next;
      for (const key of fields) delete ignored.current[key];
      applyToScreen(
        applyPatch(
          screen,
          patchOf(
            next,
            fields.filter((field) => !mine.includes(field)),
          ),
        ),
      );
      return fields;
    },
    [unsentFields, outgoing, applyToScreen, nameOf],
  );

  /** snapshot と全体の change が持つ名前を、開いているワークスペースと一覧へ反映する。 */
  const receiveName = useCallback(
    (target: Current, name: string | undefined) => {
      if (name === undefined || name === target.name) return;
      target.name = name;
      setPhase((shown) =>
        shown.kind === "open" && shown.summary.id === target.id
          ? { ...shown, summary: { ...shown.summary, name } }
          : shown,
      );
      changeList((items) =>
        items.map((item) => (item.id === target.id ? { ...item, name } : item)),
      );
    },
    [changeList],
  );

  const onEvent = useCallback(
    (event: WorkspaceEvent) => {
      const target = current.current;
      if (target === undefined) return;
      switch (event.type) {
        case "snapshot": {
          streamRevision.current = event.revision;
          target.revision = Math.max(target.revision, event.revision);
          const base = synced.current ?? {};
          const latest = withoutNulls(event.state);
          receive(
            patchOf(latest, changedFields(base, latest)),
            event.updatedBy,
          );
          receiveName(target, event.name);
          return;
        }
        case "change": {
          if (event.revision <= streamRevision.current) return;
          streamRevision.current = event.revision;
          advanceBase(target, event.revision);
          if (
            event.clientChangeId !== undefined &&
            ownChangeIds.current.delete(event.clientChangeId)
          )
            return;
          const whole = event.fields.includes("*");
          const patch = whole
            ? patchOf(
                event.patch,
                Object.keys({ ...synced.current, ...event.patch }),
              )
            : event.patch;
          const fields = receive(patch, event.actor);
          if (whole) receiveName(target, event.name);
          // 自分の名前の変更は全体の変更として届く。自分の別のタブの欄の変更は帯に出す。
          if (fields.length > 0 && !(whole && event.actor === owner)) {
            showChange(nameOf(event.actor), fieldLabelList(fields));
          }
          if (unsentFields().length > 0 && !readOnly.current) scheduleSave();
          return;
        }
        case "presence": {
          connectionsRef.current = event.connections;
          setConnections(event.connections);
          const followed = followingRef.current;
          if (followed === undefined) return;
          const leader = event.connections.find(
            (item) => item.connectionId === followed.connectionId,
          );
          if (leader === undefined) {
            stopFollowing("相手の接続の切断");
            return;
          }
          const local = latestLocal.current;
          if (local !== undefined) applyToScreen(local);
          return;
        }
        case "closed":
          stopFollowing();
          markGone(target, event.reason);
          return;
      }
    },
    [
      owner,
      receive,
      receiveName,
      showChange,
      nameOf,
      unsentFields,
      scheduleSave,
      stopFollowing,
      applyToScreen,
      markGone,
    ],
  );

  const openItem = useCallback(
    (item: WorkspaceItem) => {
      clearTimeout(timer.current);
      unsent.current = undefined;
      latestLocal.current = undefined;
      blocked.current = undefined;
      skipsNext.current = true;
      ignored.current = {};
      held.current.clear();
      absorbNext.current = [];
      streamRevision.current = 0;
      followingRef.current = undefined;
      setFollowing(undefined);
      setFollowEnded(undefined);
      setConflictNotice(undefined);
      setChangeNotice(undefined);
      connectionsRef.current = [];
      setConnections([]);
      readOnly.current = item.access === "view";
      setStatus(readOnly.current ? { kind: "viewOnly" } : { kind: "idle" });
      localStorage.setItem(storageKey, item.id);
      const summary = summaryOf(item);
      putInList(summary);
      let applied: WorkspaceState | undefined;
      if (!isEmptyState(item.state)) {
        applied = readState(withoutNulls(item.state as StateFields));
        if (applied === undefined) {
          current.current = undefined;
          synced.current = undefined;
          setPhase({ kind: "unreadable", summary });
          return;
        }
      }
      current.current = {
        id: item.id,
        name: item.name,
        revision: item.revision,
        access: item.access,
      };
      synced.current = applied ?? {};
      mounts.current += 1;
      setPhase({
        kind: "open",
        summary,
        applied,
        mount: mounts.current,
        revision: mounts.current,
      });
    },
    [storageKey, putInList],
  );

  const openById = useCallback(
    async (id: string) => {
      const unsaved = await save();
      if (unsaved !== undefined) return unsaved;
      const result = await fetchWorkspace(id);
      if (!result.ok) return result.failure;
      openItem(result.value);
      return undefined;
    },
    [save, openItem],
  );

  const createAndOpen = useCallback(async () => {
    const unsaved = await save();
    if (unsaved !== undefined) return unsaved;
    const result = await createWorkspace(undefined, {});
    if (!result.ok) return result.failure;
    openItem(result.value);
    return undefined;
  }, [save, openItem]);

  /**
   * 自分のワークスペースを一覧の順で探し、先頭を開く。自分のワークスペースが無ければ作って
   * 開く。
   */
  const openFirst = useCallback(async () => {
    const first = listRef.current.find((item) => item.access === "owner");
    return first === undefined ? createAndOpen() : openById(first.id);
  }, [createAndOpen, openById]);

  useEffect(() => {
    if (started.current) return;
    started.current = true;
    void (async () => {
      const listed = await fetchWorkspaces();
      if (!listed.ok) {
        setPhase({ kind: "failed", failure: listed.failure });
        return;
      }
      changeList(() => listed.value);
      const last = localStorage.getItem(storageKey);
      let failure: FetchFailure | undefined;
      if (last !== null) {
        failure = await openById(last);
        if (failure?.failureCode === "record_not_found") {
          failure = await openFirst();
        }
      } else {
        failure = await openFirst();
      }
      if (failure !== undefined) setPhase({ kind: "failed", failure });
    })();
  }, [storageKey, changeList, openById, openFirst]);

  const openedId = phase.kind === "open" ? phase.summary.id : undefined;
  const openedMount = phase.kind === "open" ? phase.mount : undefined;
  const onEventRef = useRef(onEvent);
  onEventRef.current = onEvent;
  useEffect(() => {
    if (openedId === undefined || openedMount === undefined) return;
    connectionId.current = crypto.randomUUID();
    sentPresence.current = undefined;
    const unsubscribe = subscribeWorkspaceEvents(
      openedId,
      connectionId.current,
      (event) => onEventRef.current(event),
    );
    schedulePresence();
    return () => {
      unsubscribe();
      connectionId.current = "";
      clearTimeout(presenceTimer.current);
      presenceTimer.current = undefined;
    };
  }, [openedId, openedMount, schedulePresence]);

  useEffect(() => {
    // ページを離れる間は保存の順番を待たず、手元の最新の revision でその場で送る。
    const leave = () => {
      const target = current.current;
      if (target === undefined || readOnly.current) return;
      // 送信中か再送を待つ変更を同じ clientChangeId で送り、その後の編集を別の変更で送る。
      const pending = unsent.current;
      const rest = newChange(
        target,
        applyPatch(synced.current ?? {}, pending?.patch ?? {}),
      );
      // 同じ欄を 2 件で送ると 2 件目が競合する。そのときは送信中の変更に残りの欄を重ねて 1 件にする。
      // 中身が送信中の変更と違うので、新しい clientChangeId を付ける。送信中の変更が先に記録されて
      // いれば、同じ欄の競合として server が退け、再送としてエラーを出さずに捨てられることは無い。
      const overlaps =
        pending !== undefined &&
        rest !== undefined &&
        Object.keys(rest.patch).some((field) => field in pending.patch);
      const merged = overlaps
        ? {
            ...pending,
            clientChangeId: crypto.randomUUID(),
            patch: { ...pending.patch, ...rest.patch },
          }
        : undefined;
      if (merged !== undefined) ownChangeIds.current.add(merged.clientChangeId);
      const changes = merged !== undefined ? [merged] : [pending, rest];
      for (const change of changes) {
        const fitting =
          change === undefined ? undefined : withoutLargeHistory(change);
        if (fitting === undefined) continue;
        if (fitting !== change)
          ownChangeIds.current.add(fitting.clientChangeId);
        void postWorkspaceChange(target.id, fitting, { keepalive: true });
      }
    };
    window.addEventListener("pagehide", leave);
    return () => {
      window.removeEventListener("pagehide", leave);
      clearTimeout(timer.current);
      clearTimeout(noticeTimer.current);
    };
  }, [newChange]);

  const onWorkspaceStateChange = useCallback(
    (state: WorkspaceState) => {
      latestLocal.current = state;
      schedulePresence();
      if (skipsNext.current) {
        // 開いた直後の通知は、開いた状態を画面が書き出し直した値である。
        skipsNext.current = false;
        const base = synced.current ?? {};
        if (!isEmptyState(base)) {
          ignored.current = Object.fromEntries(
            changedFields(state, base).map((key) => [
              key,
              JSON.stringify((state as StateFields)[key]),
            ]),
          );
        }
        return;
      }
      if (absorbNext.current.length > 0) {
        for (const key of absorbNext.current) {
          ignored.current[key] = JSON.stringify((state as StateFields)[key]);
        }
        absorbNext.current = [];
      }
      if (readOnly.current) return;
      if (unsentFields().length === 0 && unsent.current === undefined) {
        clearTimeout(timer.current);
        return;
      }
      scheduleSave();
    },
    [schedulePresence, unsentFields, scheduleSave],
  );

  const createCopy = async (name: string, state: unknown) => {
    const result = await createWorkspace(`${name} のコピー`, state);
    if (!result.ok) return result.failure;
    openItem(result.value);
    return undefined;
  };

  /** 自分を所有者とする複製を「<元の名前> のコピー」の名前で、保存された状態から作り、開く。 */
  const duplicate = async (id: string) => {
    const unsaved = await save();
    if (unsaved !== undefined) return unsaved;
    const source = await fetchWorkspace(id);
    if (!source.ok) return source.failure;
    return createCopy(source.value.name, source.value.state);
  };

  /**
   * 開いている閲覧だけの共有か、取り消された共有を、画面の今の状態で複製して開く。元の
   * ワークスペースは読まない。
   */
  const copyShown = (name: string) =>
    createCopy(name, latestLocal.current ?? {});

  const run = async (action: () => Promise<FetchFailure | undefined>) => {
    setActionFailure(undefined);
    const failure = await action();
    setActionFailure(failure);
  };

  /** 名前は全体の置き換えで変える。開いているワークスペースは、先に変更を送ってから変える。 */
  const rename = async (id: string, name: string) => {
    if (current.current?.id === id) {
      const unsaved = await save();
      if (unsaved !== undefined) return unsaved;
    }
    const item = await fetchWorkspace(id);
    if (!item.ok) return item.failure;
    const result = await replaceWorkspace(id, {
      name,
      state: item.value.state,
      baseRevision: item.value.revision,
    });
    if (!result.ok) return result.failure;
    putInList(summaryOf(result.value));
    if (current.current?.id === id) {
      current.current.name = name;
      advanceBase(current.current, result.value.revision);
    }
    setPhase((shown) =>
      shown.kind !== "loading" &&
      shown.kind !== "failed" &&
      shown.summary.id === id
        ? { ...shown, summary: summaryOf(result.value) }
        : shown,
    );
    return undefined;
  };

  const remove = async (id: string) => {
    const opened =
      phase.kind === "open" || phase.kind === "unreadable"
        ? phase.summary.id === id
        : false;
    // 削除が失敗したときに変更を失わないよう、先に保存を送る。
    if (opened) await save();
    const result = await deleteWorkspace(id);
    if (!result.ok) return result.failure;
    if (opened) {
      clearTimeout(timer.current);
      unsent.current = undefined;
      current.current = undefined;
    }
    changeList((items) => items.filter((item) => item.id !== id));
    return opened ? openFirst() : undefined;
  };

  /** 競合で捨てた自分の値を画面へ戻し、送り直す。 */
  const resendLost = (notice: ConflictNotice) => {
    setConflictNotice(undefined);
    for (const field of notice.fields) held.current.delete(field);
    // 届いた変更との競合では、自分の値は画面に残っている。通知の後の編集も含めて画面の値を送る。
    const local = latestLocal.current;
    if (!notice.incoming && local !== undefined) {
      applyToScreen(applyPatch(local, notice.lost));
    }
    void save();
  };

  /** 届いた相手の値を画面へ適用し、自分の値を捨てる。 */
  const takeTheirs = (notice: ConflictNotice) => {
    setConflictNotice(undefined);
    for (const field of notice.fields) held.current.delete(field);
    const local = latestLocal.current;
    if (local !== undefined) applyToScreen(applyPatch(local, notice.theirs));
  };

  const listedId =
    phase.kind === "open" || phase.kind === "unreadable"
      ? phase.summary.id
      : undefined;
  const panel = (
    <WorkspacePanel
      list={list}
      openedId={listedId}
      failure={actionFailure}
      canShare={signedIn !== undefined}
      onOpen={(id) => run(() => openById(id))}
      onDuplicate={(id) => run(() => duplicate(id))}
      onCreate={() => run(createAndOpen)}
      onRename={(id, name) => run(() => rename(id, name))}
      onDelete={(id) => run(() => remove(id))}
    />
  );

  if (phase.kind !== "open") {
    return (
      <main className="app stages-page">
        <header className="flex items-center gap-2">
          <h1>Oraculum</h1>
          <AccountBar />
          <ThemeToggle />
        </header>
        {phase.kind === "loading" ? (
          <p role="status">ワークスペースの読み込み中</p>
        ) : phase.kind === "failed" ? (
          <FetchFailureNotice failure={phase.failure} />
        ) : (
          <div role="alert">
            <KeyValueList
              pairs={[
                {
                  name: "ワークスペース",
                  value: <RawText text={phase.summary.name} />,
                },
                {
                  name: "状態",
                  value: (
                    <StatusLabel
                      status="failed"
                      label="読み込み失敗"
                      details={
                        <KeyValueList
                          stacked
                          pairs={[
                            {
                              name: "原因",
                              value: "この画面で読み込める形式との不一致",
                            },
                            { name: "保存した状態", value: "保持" },
                            {
                              name: "次の操作",
                              value: "新規作成か別のワークスペースを開く",
                            },
                          ]}
                        />
                      }
                    />
                  ),
                },
              ]}
            />
          </div>
        )}
        {phase.kind === "loading" ? null : panel}
      </main>
    );
  }

  const gone = status.kind === "revoked" || status.kind === "deleted";
  const bar = (
    <div className="workspace-bar">
      <span>
        ワークスペース: <RawText text={phase.summary.name} />
      </span>
      <span role="status">
        {saveStatusText[status.kind] === "" ? null : (
          <StatusDot
            status={
              status.kind === "saving"
                ? "running"
                : status.kind === "saved"
                  ? "done"
                  : "failed"
            }
          >
            {saveStatusText[status.kind]}
          </StatusDot>
        )}
      </span>
      <PresenceList
        connections={connections}
        ownConnectionId={connectionId.current}
        following={following?.connectionId}
        onFollow={startFollowing}
      />
      {following === undefined ? null : (
        <div role="status" className="flex items-center gap-1">
          <KeyValueList
            pairs={[
              {
                name: "追従中",
                value: (
                  <RawText
                    text={
                      connections.find(
                        (item) => item.connectionId === following.connectionId,
                      )?.displayName ?? ""
                    }
                  />
                ),
              },
            ]}
          />
          <IconButton label="追従を解除" onPress={() => stopFollowing()}>
            <EyeOff size={14} aria-hidden="true" />
          </IconButton>
        </div>
      )}
      {followEnded === undefined ? null : (
        <p role="status">
          <StatusLabel status="idle" label="追従の終了" details={followEnded} />
        </p>
      )}
      {changeNotice === undefined ? null : (
        <div role="status">
          <KeyValueList
            pairs={[
              {
                name: "変更した利用者",
                value: <RawText text={changeNotice.who} />,
              },
              { name: "変更した項目", value: changeNotice.fields },
            ]}
          />
        </div>
      )}
      {conflictNotice === undefined ? null : (
        // 上部の帯の並びを崩さないよう、帯の下に重ねて出す。表だけを scroll させ、操作を常に見せる。
        <div
          role="alert"
          className="fixed top-(--bar-height) right-4 z-50 mt-1 flex w-[min(48rem,calc(100vw-2rem))] flex-col gap-1 rounded-md border border-line bg-surface p-3 shadow-float"
        >
          <p>
            <strong>保存の競合</strong>
          </p>
          <p>
            変更した利用者: <RawText text={conflictNotice.who} />
          </p>
          <p>自分の値の保存: {conflictNotice.incoming ? "未送信" : "未反映"}</p>
          <div className="max-h-[30vh] overflow-auto">
            <table aria-label="競合したフィールド">
              <thead>
                <tr>
                  <th scope="col">フィールド</th>
                  <th scope="col">相手の値</th>
                  <th scope="col">自分の値</th>
                </tr>
              </thead>
              <tbody>
                {conflictRowFields(conflictNotice).map((field) => (
                  <tr key={field}>
                    <th scope="row">{fieldLabelList([field])}</th>
                    <td className="break-all">
                      <RawText text={summarize(conflictNotice.theirs, field)} />
                    </td>
                    <td className="break-all">
                      <RawText text={summarize(conflictNotice.lost, field)} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="flex gap-1">
            <IconButton
              label="自分の値を再送信"
              onPress={() => resendLost(conflictNotice)}
            >
              <Upload size={14} aria-hidden="true" />
            </IconButton>
            {conflictNotice.incoming ? (
              <IconButton
                label="相手の値を採用"
                onPress={() => takeTheirs(conflictNotice)}
              >
                <Download size={14} aria-hidden="true" />
              </IconButton>
            ) : (
              <IconButton
                label="競合の通知を閉じる"
                onPress={() => setConflictNotice(undefined)}
              >
                <X size={14} aria-hidden="true" />
              </IconButton>
            )}
          </div>
        </div>
      )}
      {status.kind === "viewOnly" ? (
        <div className="flex items-center gap-1">
          {status.downgraded ? (
            <p role="alert">
              <StatusLabel
                status="failed"
                label="保存失敗"
                details="所有者による閲覧のみへの変更"
              />
            </p>
          ) : null}
          <p role="status">
            <StatusLabel
              status="idle"
              label="閲覧のみ"
              details={
                <KeyValueList
                  stacked
                  pairs={[
                    { name: "変更の保存", value: "なし" },
                    { name: "保存するには", value: "自分用に複製" },
                  ]}
                />
              }
            />
          </p>
          <IconButton
            label="自分用に複製"
            onPress={() => void run(() => copyShown(phase.summary.name))}
          >
            <Copy size={14} aria-hidden="true" />
          </IconButton>
          {actionFailure === undefined ? null : (
            <FetchFailureNotice failure={actionFailure} />
          )}
        </div>
      ) : null}
      {gone ? (
        <div role="alert" className="flex items-center gap-1">
          <StatusLabel
            status="failed"
            label={
              status.kind === "revoked" ? "共有の解除" : "所有者による削除"
            }
            details={
              <KeyValueList
                stacked
                pairs={[{ name: "変更の保存", value: "なし" }]}
              />
            }
          />
          <IconButton
            label="自分用に複製"
            onPress={() => void run(() => copyShown(phase.summary.name))}
          >
            <Copy size={14} aria-hidden="true" />
          </IconButton>
          <IconButton
            label="自分のワークスペースに移動"
            onPress={() => void run(openFirst)}
          >
            <FolderInput size={14} aria-hidden="true" />
          </IconButton>
          {actionFailure === undefined ? null : (
            <FetchFailureNotice failure={actionFailure} />
          )}
        </div>
      ) : null}
      {status.kind === "failed" ? (
        <>
          <FetchFailureNotice failure={status.failure} />
          <IconButton label="再保存" onPress={() => void save()}>
            <RefreshCw size={14} aria-hidden="true" />
          </IconButton>
        </>
      ) : null}
    </div>
  );
  return (
    <Fragment key={phase.mount}>
      {children({
        workspaceState:
          phase.applied === undefined
            ? undefined
            : { state: phase.applied, revision: phase.revision },
        onWorkspaceStateChange,
        workspaceBar: bar,
        workspacePanel: panel,
      })}
    </Fragment>
  );
}

/**
 * 同じワークスペースを開いている接続の一覧。自分の接続に「自分」を付け、同じ利用者の複数の
 * 接続を番号で分ける。他の接続には追従する操作を置き、自分に追従している接続を知らせる。
 */
function PresenceList({
  connections,
  ownConnectionId,
  following,
  onFollow,
}: {
  connections: readonly WorkspaceConnection[];
  ownConnectionId: string;
  following: string | undefined;
  onFollow: (connection: WorkspaceConnection) => void;
}) {
  // 見出しを別ウィンドウに出しても、一覧をその文書に描く。
  const portalContainer = usePortalContainer();
  if (connections.length === 0) return null;
  // 同じ利用者の複数の接続を番号で分ける。1 つだけの利用者は番号を持たない。
  const numberOf = (connection: WorkspaceConnection) => {
    const same = connections.filter((item) => item.login === connection.login);
    return same.length > 1 ? same.indexOf(connection) + 1 : undefined;
  };
  const followers = connections.filter(
    (item) =>
      item.following === ownConnectionId &&
      item.connectionId !== ownConnectionId,
  );
  const list = (
    <ul aria-label="接続中の利用者" className="flex flex-col gap-1">
      {connections.map((connection) => {
        const own = connection.connectionId === ownConnectionId;
        const number = numberOf(connection);
        const name = [connection.displayName, connection.login, number]
          .filter((part) => part !== undefined)
          .join(" ");
        return (
          <li
            key={connection.connectionId}
            aria-label={name}
            className="flex items-center justify-between gap-3"
          >
            <KeyValueList
              pairs={[
                {
                  name: "利用者",
                  value: <RawText text={connection.displayName} />,
                },
                {
                  name: "ログイン名",
                  value: <RawText text={connection.login} />,
                },
                {
                  name: "接続",
                  value: number === undefined ? undefined : `${number}`,
                },
                { name: "状態", value: own ? "自分" : undefined },
              ]}
            />
            {own || connection.connectionId === following ? null : (
              <IconButton
                label="画面に追従"
                accessibleName={`${name} の画面に追従`}
                onPress={() => onFollow(connection)}
              >
                <Eye size={14} aria-hidden="true" />
              </IconButton>
            )}
          </li>
        );
      })}
    </ul>
  );
  // 1 つだけなら自分の名前を見出しに並べる。複数なら数だけを見出しに置き、一覧は開いて見せる。
  return (
    <>
      {connections.length === 1 ? (
        list
      ) : (
        <DialogTrigger>
          <Button
            size="sm"
            variant="ghost"
            aria-label={`接続中の利用者: ${connections.length}`}
          >
            <Users size={14} aria-hidden="true" />
            {connections.length}
          </Button>
          <Popover
            UNSTABLE_portalContainer={portalContainer}
            placement="bottom end"
            className="min-w-64 rounded-md border border-line bg-surface p-3 text-sm shadow-float outline-none"
          >
            <Dialog aria-label="接続中の利用者" className="outline-none">
              {list}
            </Dialog>
          </Popover>
        </DialogTrigger>
      )}
      {followers.map((item) => (
        <div key={item.connectionId} role="status">
          <KeyValueList
            pairs={[
              {
                name: "自分に追従",
                value: <RawText text={item.displayName} />,
              },
            ]}
          />
        </div>
      ))}
    </>
  );
}

const updatedAtColumn: DataTableColumn<WorkspaceSummary> = {
  key: "updatedAt",
  header: "最終更新",
  mono: true,
  sortValue: (item) => Date.parse(item.updatedAt),
  cell: (item) => <RawText text={item.updatedAt} />,
};

const updatedByColumn: DataTableColumn<WorkspaceSummary> = {
  key: "updatedBy",
  header: "更新者",
  sortValue: (item) => item.updatedBy,
  cell: (item) => <RawText text={item.updatedBy} />,
};

function accessLabelOf(item: WorkspaceSummary): string {
  return shareAccessLabels[item.access === "edit" ? "edit" : "view"];
}

/** 開いているワークスペースは表示中の印を、ほかは開く button を出す。 */
function OpenWorkspace({
  item,
  openedId,
  onOpen,
}: {
  item: WorkspaceSummary;
  openedId: string | undefined;
  onOpen: (id: string) => void;
}) {
  return item.id === openedId ? (
    <StatusLabel status="done" label="表示中" />
  ) : (
    <IconButton
      label="ワークスペースを開く"
      accessibleName={`${item.name} を開く`}
      onPress={() => onOpen(item.id)}
    >
      <FolderOpen size={14} aria-hidden="true" />
    </IconButton>
  );
}

/**
 * 自分のワークスペースと共有されたワークスペースの一覧。自分のワークスペースは開く・作る・
 * 名前を変える・削除する・共有する操作を持ち、共有されたワークスペースは開く・複製する操作を
 * 持つ。閲覧者も使える。canShare はアカウントを持つ起動で真になる。
 */
function WorkspacePanel({
  list,
  openedId,
  failure,
  canShare,
  onOpen,
  onDuplicate,
  onCreate,
  onRename,
  onDelete,
}: {
  list: readonly WorkspaceSummary[];
  openedId: string | undefined;
  failure: FetchFailure | undefined;
  canShare: boolean;
  onOpen: (id: string) => void;
  onDuplicate: (id: string) => void;
  onCreate: () => void;
  onRename: (id: string, name: string) => void;
  onDelete: (id: string) => void;
}) {
  const [editing, setEditing] = useState<{ id: string; name: string }>();
  const [sharingId, setSharingId] = useState<string>();
  // 開いているワークスペースを先頭に置き、ほかは応答の順 (更新の新しい順) を保つ。
  const ordered = [
    ...list.filter((item) => item.id === openedId),
    ...list.filter((item) => item.id !== openedId),
  ];
  const own = ordered.filter((item) => item.access === "owner");
  const shared = ordered.filter((item) => item.access !== "owner");
  const sharing = canShare
    ? own.find((item) => item.id === sharingId)
    : undefined;
  return (
    <section aria-label="ワークスペース">
      <IconButton label="ワークスペースを新規作成" onPress={onCreate}>
        <FilePlus2 size={14} aria-hidden="true" />
      </IconButton>
      {failure === undefined ? null : <FetchFailureNotice failure={failure} />}
      <h3>自分のワークスペース</h3>
      <DataTable
        label="自分のワークスペース"
        rows={own}
        rowKey={(item) => item.id}
        rowProps={(item) => ({
          "aria-label": item.name,
          "aria-current": item.id === openedId ? "true" : undefined,
        })}
        columns={[
          {
            key: "name",
            header: "名前",
            rowHeader: true,
            sortValue: (item) => item.name,
            cell: (item) =>
              editing?.id === item.id ? (
                <form
                  className="flex items-center gap-1"
                  onSubmit={(event) => {
                    event.preventDefault();
                    setEditing(undefined);
                    onRename(item.id, editing.name.trim());
                  }}
                >
                  <input
                    aria-label="新しい名前"
                    value={editing.name}
                    maxLength={256}
                    onChange={(event) =>
                      setEditing({ id: item.id, name: event.target.value })
                    }
                  />
                  <IconButton
                    type="submit"
                    label="名前を保存"
                    isDisabled={editing.name.trim() === ""}
                    disabledReason={{
                      title: "名前なし",
                      text: "名前の入力が必要",
                    }}
                  >
                    <Check size={14} aria-hidden="true" />
                  </IconButton>
                  <IconButton
                    label="名前の変更を取消"
                    onPress={() => setEditing(undefined)}
                  >
                    <X size={14} aria-hidden="true" />
                  </IconButton>
                </form>
              ) : (
                <RawText text={item.name} />
              ),
          },
          updatedAtColumn,
          updatedByColumn,
          {
            key: "actions",
            header: "操作",
            cell: (item) =>
              editing?.id === item.id ? null : (
                <span className="inline-flex items-center gap-1">
                  <OpenWorkspace
                    item={item}
                    openedId={openedId}
                    onOpen={onOpen}
                  />
                  <IconButton
                    label="名前を変更"
                    accessibleName={`${item.name} の名前を変更`}
                    onPress={() => setEditing({ id: item.id, name: item.name })}
                  >
                    <Pencil size={14} aria-hidden="true" />
                  </IconButton>
                  <ConfirmIconButton
                    label="ワークスペースを削除"
                    accessibleName={`${item.name} を削除`}
                    details={
                      <KeyValueList
                        stacked
                        pairs={[
                          {
                            name: "ワークスペース",
                            value: <RawText text={item.name} />,
                          },
                        ]}
                      />
                    }
                    onConfirm={() => onDelete(item.id)}
                  >
                    <Trash2 size={14} aria-hidden="true" />
                  </ConfirmIconButton>
                  {canShare ? (
                    <IconButton
                      label="共有を設定"
                      accessibleName={`${item.name} の共有を設定`}
                      aria-expanded={sharingId === item.id}
                      onPress={() =>
                        setSharingId(
                          sharingId === item.id ? undefined : item.id,
                        )
                      }
                    >
                      <Share2 size={14} aria-hidden="true" />
                    </IconButton>
                  ) : null}
                </span>
              ),
          },
        ]}
      />
      {sharing === undefined ? null : (
        <WorkspaceShares
          workspaceId={sharing.id}
          workspaceName={sharing.name}
        />
      )}
      {shared.length === 0 ? null : (
        <>
          <h3>共有されたワークスペース</h3>
          <DataTable
            label="共有されたワークスペース"
            rows={shared}
            rowKey={(item) => item.id}
            rowProps={(item) => ({
              "aria-label": item.name,
              "aria-current": item.id === openedId ? "true" : undefined,
            })}
            columns={[
              {
                key: "name",
                header: "名前",
                rowHeader: true,
                sortValue: (item) => item.name,
                cell: (item) => <RawText text={item.name} />,
              },
              {
                key: "owner",
                header: "所有者",
                sortValue: (item) => item.owner,
                cell: (item) => <RawText text={item.owner} />,
              },
              {
                key: "access",
                header: "権限",
                sortValue: (item) => accessLabelOf(item),
                cell: accessLabelOf,
              },
              updatedAtColumn,
              updatedByColumn,
              {
                key: "actions",
                header: "操作",
                cell: (item) => (
                  <span className="inline-flex items-center gap-1">
                    <OpenWorkspace
                      item={item}
                      openedId={openedId}
                      onOpen={onOpen}
                    />
                    <IconButton
                      label="自分用に複製"
                      accessibleName={`${item.name} を自分用に複製`}
                      onPress={() => onDuplicate(item.id)}
                    >
                      <Copy size={14} aria-hidden="true" />
                    </IconButton>
                  </span>
                ),
              },
            ]}
          />
        </>
      )}
    </section>
  );
}
