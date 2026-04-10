package launcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExecProcessRunnerStartsProcess(t *testing.T) {
	t.Parallel()

	runner := NewExecProcessRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := runner.Run(ctx, "sleep", []string{"0.01"}, "", nil); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

func TestExecProcessRunnerPreservesParentEnvWhenLauncherEnvSet(t *testing.T) {
	t.Setenv("BWB_PARENT_ENV", "present")

	runner := NewExecProcessRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tmpDir := t.TempDir()
	okFile := filepath.Join(tmpDir, "ok")
	failFile := filepath.Join(tmpDir, "fail")

	cmd := "if [ \"$BWB_PARENT_ENV\" = \"present\" ] && [ \"$BWB_LAUNCHER_ENV\" = \"set\" ]; then touch \"$1\"; else touch \"$2\"; fi"
	args := []string{"-c", cmd, "sh", okFile, failFile}

	err := runner.Run(ctx, "sh", args, "", []string{"BWB_LAUNCHER_ENV=set"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(okFile); err == nil {
			return
		}
		if _, err := os.Stat(failFile); err == nil {
			t.Fatal("launcher environment replaced parent environment")
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for process output files in %s", tmpDir)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
