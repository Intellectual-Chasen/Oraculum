package tooling_test

import (
	"bufio"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/tooling"
)

// 子 process は渡した環境変数だけを持ち、呼び出し元の環境を引き継がない。
func TestStartProcessPassesOnlyTheGivenEnvironment(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is not on PATH; the test needs a POSIX shell and runs on the CI runners")
	}
	t.Setenv("ORACULUM_TOOLING_SECRET", "synthetic")
	process, err := tooling.StartProcess(tooling.ProcessSpec{
		Path: shell, Args: []string{"-c", "echo \"given=$GIVEN secret=$ORACULUM_TOOLING_SECRET\""},
		Env: []string{"GIVEN=value"},
	})
	if err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(process.Stdout)
	if err != nil {
		t.Fatal(err)
	}
	<-process.Done()
	if got := strings.TrimSpace(string(output)); got != "given=value secret=" {
		t.Errorf("output = %q, want only the given variable", got)
	}
	if err := process.Err(); err != nil {
		t.Errorf("exit = %v", err)
	}
}

// Stop は標準入力を読み続ける process を止め、終わるまで待つ。
func TestStopEndsARunningProcess(t *testing.T) {
	cat, err := exec.LookPath("cat")
	if err != nil {
		t.Skip("cat is not on PATH; the test needs a POSIX environment and runs on the CI runners")
	}
	process, err := tooling.StartProcess(tooling.ProcessSpec{Path: cat})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(process.Stdin, "line\n"); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(process.Stdout).ReadString('\n')
	if err != nil || line != "line\n" {
		t.Fatalf("echoed %q, %v", line, err)
	}
	stopped := make(chan error, 1)
	go func() { stopped <- process.Stop() }()
	select {
	case err := <-stopped:
		if err != nil {
			t.Errorf("Stop = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Stop did not end the process")
	}
	select {
	case <-process.Done():
	default:
		t.Error("the process is not done after Stop")
	}
}

// 標準入力の終わりで終わらない process は、猶予の後に止める。
func TestStopKillsAProcessThatIgnoresTheEndOfItsInput(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is not on PATH; the test needs a POSIX shell and runs on the CI runners")
	}
	process, err := tooling.StartProcess(tooling.ProcessSpec{
		Path: shell, Args: []string{"-c", "while :; do sleep 1; done"},
	})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	stopped := make(chan error, 1)
	go func() { stopped <- process.Stop() }()
	select {
	case err := <-stopped:
		if err != nil {
			t.Errorf("Stop = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Stop did not end the process")
	}
	if elapsed := time.Since(started); elapsed < time.Second {
		t.Errorf("Stop returned after %v, want it to wait for the process first", elapsed)
	}
}
