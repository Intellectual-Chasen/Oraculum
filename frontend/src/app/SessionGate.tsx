import { LogIn, RefreshCw } from "lucide-react";
import { type ReactNode, useEffect, useId, useRef, useState } from "react";
import {
  onAuthenticationRequired,
  onRoleRejected,
} from "@/shared/api/httpClient";
import { fetchMyMember } from "@/shared/api/members";
import { fetchSession, login, logout } from "@/shared/api/session";
import type { MemberRole } from "@/shared/contracts/members";
import type { Session } from "@/shared/contracts/session";
import type { FetchFailure, FetchState } from "@/shared/lib/fetchState";
import { FetchFailureNotice } from "@/shared/ui/FetchFailureNotice";
import { FetchStateView } from "@/shared/ui/FetchStateView";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { type SignedIn, SignedInContext } from "@/shared/ui/SignedInAccount";
import { ThemeToggle } from "@/shared/ui/ThemeToggle";
import { AccountBar } from "./AccountBar";

type RoleState = FetchState<MemberRole> | { status: "no_role" };

/**
 * ログインした利用者の調査の役割を読み、役割を `SignedInContext` で `children` へ渡す。
 *
 * 役割を持たない利用者 (`investigation_not_found`) と役割を読めなかった利用者には、調査の画面を
 * 出さずに、理由と「もう一度確かめる」とログアウトのボタンを出す。
 *
 * 役割を次の時点で読み直す。
 * - `reloginCount` が増えたとき。役割を読めていない状態に限る。
 * - 途中の要求が `permission_denied` か `investigation_not_found` を受け取ったとき。役割を
 *   読めている状態に限る。読み直す間も `children` を出し続け、入力中の内容を保つ。
 */
function MemberRoleGate({
  signedIn,
  reloginCount,
  children,
}: {
  signedIn: Omit<SignedIn, "role">;
  /** 同じ利用者がログインし直した回数。 */
  reloginCount: number;
  children: ReactNode;
}) {
  const [state, setState] = useState<RoleState>({ status: "loading" });
  const [attempt, setAttempt] = useState(0);
  const [seenReloginCount, setSeenReloginCount] = useState(reloginCount);
  if (reloginCount !== seenReloginCount) {
    setSeenReloginCount(reloginCount);
    if (state.status !== "loaded") {
      setAttempt((count) => count + 1);
    }
  }
  const stateRef = useRef(state);
  stateRef.current = state;
  // 役割を読む要求そのものが受け取る investigation_not_found で、読み直しを繰り返さない。
  const reading = useRef(false);

  useEffect(
    () =>
      onRoleRejected(() => {
        if (!reading.current && stateRef.current.status === "loaded") {
          setAttempt((count) => count + 1);
        }
      }),
    [],
  );

  // biome-ignore lint/correctness/useExhaustiveDependencies: attempt が増えたときに役割を読み直す。
  useEffect(() => {
    const controller = new AbortController();
    reading.current = true;
    void fetchMyMember({ signal: controller.signal }).then((result) => {
      if (controller.signal.aborted) {
        return;
      }
      reading.current = false;
      if (result.ok) {
        setState({ status: "loaded", value: result.value.role });
      } else if (result.failure.failureCode === "investigation_not_found") {
        setState({ status: "no_role" });
      } else if (stateRef.current.status !== "loaded") {
        // 役割を読めている間の読み直しが一時的に失敗しても、調査の画面と入力を保つ。
        setState({ status: "failed", failure: result.failure });
      }
    });
    return () => {
      reading.current = false;
      controller.abort();
    };
  }, [attempt]);

  const retry = () => {
    setState({ status: "loading" });
    setAttempt((count) => count + 1);
  };
  if (state.status === "no_role" || state.status === "failed") {
    return (
      <main className="app stages-page">
        <header className="flex items-center gap-2">
          <h1>Oraculum</h1>
          <AccountBar account={signedIn} />
          <ThemeToggle />
        </header>
        {state.status === "no_role" ? (
          <div role="alert">
            <KeyValueList
              pairs={[
                { name: "役割", value: "なし" },
                { name: "次の操作", value: "管理者に役割の付与を依頼" },
              ]}
            />
          </div>
        ) : (
          <FetchFailureNotice failure={state.failure} />
        )}
        <IconButton label="役割を再確認" onPress={retry}>
          <RefreshCw size={14} aria-hidden="true" />
        </IconButton>
      </main>
    );
  }
  return (
    <FetchStateView state={state} loadingDescription="調査の役割の確認中">
      {(role) => (
        <SignedInContext.Provider value={{ ...signedIn, role }}>
          {children}
        </SignedInContext.Provider>
      )}
    </FetchStateView>
  );
}

/**
 * ログインの状態。
 *
 * - `login_required`: 起動時にログインしていないか、ログアウトした状態。ログインの画面だけを出す。
 * - `loaded`: セッションを読めた状態。`relogin` を持つ間は、途中の要求が
 *   `authentication_required` を受け取ったため、調査の画面の上にログインの form を重ねる。
 *   `generation` は調査の画面の key であり、別の利用者がログインし直したときに増やす。
 *   `reloginCount` は重ねた form からログインし直した回数である。
 */
type GateState =
  | { status: "loading" }
  | { status: "failed"; failure: FetchFailure }
  | { status: "login_required" }
  | {
      status: "loaded";
      session: Session;
      generation: number;
      reloginCount: number;
      relogin?: { reason: FetchFailure };
    };

type LoginFormProps = {
  /** ログインの form を出した理由。途中の要求が受け取った失敗を渡す。 */
  reason?: FetchFailure;
  onSignedIn: (session: Session) => void;
};

function LoginForm({ reason, onSignedIn }: LoginFormProps) {
  const fieldId = useId();
  const [loginName, setLoginName] = useState("");
  const [password, setPassword] = useState("");
  const [failure, setFailure] = useState<FetchFailure | undefined>(undefined);
  const [pending, setPending] = useState(false);
  const loginField = useRef<HTMLInputElement>(null);
  // 調査の画面に重ねて出したときは、背景を inert にして外れたフォーカスを form へ置く。
  useEffect(() => {
    if (reason !== undefined) {
      loginField.current?.focus();
    }
  }, [reason]);

  return (
    <>
      {reason === undefined ? null : <FetchFailureNotice failure={reason} />}
      <form
        className="login-form"
        onSubmit={async (event) => {
          event.preventDefault();
          setPending(true);
          const result = await login(loginName, password);
          setPending(false);
          if (result.ok) {
            onSignedIn(result.value);
            return;
          }
          setPassword("");
          setFailure(result.failure);
        }}
      >
        <fieldset>
          <legend>ログイン</legend>
          <p>
            <label htmlFor={`${fieldId}-login`}>ログイン名</label>
            <input
              ref={loginField}
              id={`${fieldId}-login`}
              autoComplete="username"
              required
              value={loginName}
              onChange={(event) => setLoginName(event.target.value)}
            />
          </p>
          <p>
            <label htmlFor={`${fieldId}-password`}>パスワード</label>
            <input
              id={`${fieldId}-password`}
              type="password"
              autoComplete="current-password"
              required
              value={password}
              onChange={(event) => setPassword(event.target.value)}
            />
          </p>
          <IconButton
            type="submit"
            variant="primary"
            label="ログイン"
            isDisabled={pending}
          >
            <LogIn size={14} aria-hidden="true" />
          </IconButton>
        </fieldset>
      </form>
      {failure === undefined ? null : <FetchFailureNotice failure={failure} />}
    </>
  );
}

/** 同じ利用者のセッションかを返す。 */
function sameAccount(previous: Session, next: Session): boolean {
  return (
    previous.authentication === "account" &&
    next.authentication === "account" &&
    previous.login === next.login
  );
}

/**
 * ログインの状態を読み、ログインが要るときはログインの form を出す。
 *
 * アカウントを持たない起動では `children` をそのまま出す。ログインしているときは、利用者と
 * 調査の役割とログアウトの操作を `SignedInContext` で `children` へ渡す。
 *
 * **途中の要求が `authentication_required` を受け取ったときは、`children` を残して上に form を
 * 重ねる。** 入力中の内容と調査の状態を保つ。重ねている間の `children` は `inert` であり、
 * キーボードで移れない。別の利用者がログインし直したときだけ `children` を作り直し、前の
 * 利用者の状態を渡さない。
 */
export function SessionGate({ children }: { children: ReactNode }) {
  const [state, setState] = useState<GateState>({ status: "loading" });

  useEffect(() => {
    const unsubscribe = onAuthenticationRequired((reason) =>
      setState((current) =>
        current.status === "loaded" &&
        current.session.authentication === "account"
          ? { ...current, relogin: { reason } }
          : current,
      ),
    );
    const controller = new AbortController();
    void fetchSession({ signal: controller.signal }).then((result) => {
      if (controller.signal.aborted) {
        return;
      }
      if (result.ok) {
        setState({
          status: "loaded",
          session: result.value,
          generation: 0,
          reloginCount: 0,
        });
      } else if (result.failure.failureCode === "authentication_required") {
        setState({ status: "login_required" });
      } else {
        setState({ status: "failed", failure: result.failure });
      }
    });
    return () => {
      unsubscribe();
      controller.abort();
    };
  }, []);

  const signIn = (session: Session) =>
    setState((current) =>
      current.status === "loaded"
        ? {
            status: "loaded",
            session,
            generation: sameAccount(current.session, session)
              ? current.generation
              : current.generation + 1,
            reloginCount: current.reloginCount + 1,
          }
        : { status: "loaded", session, generation: 0, reloginCount: 0 },
    );

  if (state.status === "login_required") {
    return (
      <main className="app stages-page">
        <header className="flex items-center gap-2">
          <h1>Oraculum</h1>
          <ThemeToggle />
        </header>
        <LoginForm onSignedIn={signIn} />
      </main>
    );
  }
  if (state.status !== "loaded") {
    return (
      <FetchStateView state={state} loadingDescription="ログインの状態の確認中">
        {() => null}
      </FetchStateView>
    );
  }
  const { session } = state;
  if (session.authentication === "none") {
    return children;
  }
  const signedIn: Omit<SignedIn, "role"> = {
    account: session,
    logout: async () => {
      const result = await logout();
      if (!result.ok) {
        return result.failure;
      }
      setState({ status: "login_required" });
      return undefined;
    },
  };
  return (
    <>
      <div
        key={state.generation}
        className="session-content"
        inert={state.relogin !== undefined}
      >
        <MemberRoleGate signedIn={signedIn} reloginCount={state.reloginCount}>
          {children}
        </MemberRoleGate>
      </div>
      {state.relogin === undefined ? null : (
        <div
          className="login-overlay"
          role="dialog"
          aria-modal="true"
          aria-label="再ログイン"
        >
          <div className="login-dialog">
            <LoginForm reason={state.relogin.reason} onSignedIn={signIn} />
          </div>
        </div>
      )}
    </>
  );
}
