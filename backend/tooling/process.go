// Package tooling は、外部 process の起動などの共通基盤を持つ。
//
// Hexagonal Layer: tooling。依存先は標準ライブラリである。adapter は os/exec を直接使わず、
// 本 package を通して外部 process を起動する。
package tooling

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// ProcessSpec は起動する外部 process の指定である。
type ProcessSpec struct {
	// Path は実行 file の path である。
	Path string
	// Args は実行 file の名前を除いた引数である。
	Args []string
	// Env は子 process に渡す環境変数の全部である。**呼び出し元の環境を引き継がない。**
	// 渡す変数は呼び出し元が選ぶ。
	Env []string
	// Dir は子 process の作業 directory である。
	Dir string
	// Stderr は子 process の標準 error の書き込み先である。nil は捨てる。
	Stderr io.Writer
}

// Process は起動した外部 process 1 つと、その標準入力と標準出力である。
type Process struct {
	cmd    *exec.Cmd
	Stdin  io.WriteCloser
	Stdout io.ReadCloser
	once   sync.Once
	waited chan struct{}
	err    error
}

// StartProcess は spec の外部 process を起動する。
//
// **ctx の取り消しで process を止めない。** 止めるのは呼び出し元の Stop である。要求 1 つの
// ctx で起動すると、browser との接続が切れた時点で応答の途中の process が止まる。
//
// **標準出力は本関数が作った pipe で渡す。** `cmd.StdoutPipe` の pipe は、終了を待つ
// `cmd.Wait` が process の終わりを見た時点で閉じる。終了を待つ goroutine は読み終わりを
// 待たないため、process がすぐに終わると、読み残した出力が「file already closed」で失われる。
// 本関数の pipe は Wait が閉じず、process が書き込み側を閉じると読み側が EOF を返す。
func StartProcess(spec ProcessSpec) (*Process, error) {
	cmd := exec.CommandContext(context.Background(), spec.Path, spec.Args...) // #nosec G204 -- 実行 file と引数は呼び出し元の adapter が固定の組から決める。
	cmd.Env = spec.Env
	if cmd.Env == nil {
		cmd.Env = []string{}
	}
	cmd.Dir = spec.Dir
	cmd.Stderr = spec.Stderr
	isolateProcessGroup(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("starting %q: %w", spec.Path, err)
	}
	stdout, stdoutWriter, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("starting %q: %w", spec.Path, err)
	}
	cmd.Stdout = stdoutWriter
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()       // best-effort: 起動できなかった process の pipe を捨てる
		_ = stdoutWriter.Close() // best-effort: 同上
		return nil, fmt.Errorf("starting %q: %w", spec.Path, err)
	}
	// 書き込み側は子 process が持つ。親の複製を閉じないと、子が終わっても読み側が EOF を返さない。
	if err := stdoutWriter.Close(); err != nil {
		return nil, errors.Join(fmt.Errorf("starting %q: %w", spec.Path, err), killProcessGroup(cmd), cmd.Wait())
	}
	process := &Process{cmd: cmd, Stdin: stdin, Stdout: stdout, waited: make(chan struct{})}
	go func() {
		process.err = cmd.Wait()
		close(process.waited)
	}()
	return process, nil
}

// Done は process が終わると閉じる channel を返す。
func (p *Process) Done() <-chan struct{} { return p.waited }

// Err は終わった process の終了の結果を返す。終わる前は nil である。
func (p *Process) Err() error {
	select {
	case <-p.waited:
		return p.err
	default:
		return nil
	}
}

// stopGrace は、標準入力を閉じた process が自分で終わるのを待つ上限である。
//
// 既知の制限: 待つ上限を 2 秒にする, 偽の CLI は標準入力の終わりで即座に終わる。実機の CLI が
// 終わるまでの時間は測っていない, 上限の後の kill で子の process の後始末が残ったときに見直す
const stopGrace = 2 * time.Second

// Stop は process と、process が起動した子の process を止め、終わるまで待つ。標準入力を閉じて
// stopGrace の間待ち、終わらなければ process の group を kill する。止めた後に標準出力の読み側を
// 閉じる。
func (p *Process) Stop() error {
	var stopErr error
	p.once.Do(func() {
		_ = p.Stdin.Close() // best-effort: 標準入力を閉じると多くの CLI は自分で終わる
		defer func() {
			_ = p.Stdout.Close() // best-effort: 止めた process の出力は読まない
		}()
		select {
		case <-p.waited:
			return
		case <-time.After(stopGrace):
		}
		stopErr = killProcessGroup(p.cmd)
		<-p.waited
	})
	var exitErr *exec.ExitError
	if stopErr != nil && !errors.As(stopErr, &exitErr) {
		return fmt.Errorf("stopping the process: %w", stopErr)
	}
	return nil
}

// LookPath は PATH から実行 file を探す。
func LookPath(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("looking up %q: %w", name, err)
	}
	return path, nil
}
