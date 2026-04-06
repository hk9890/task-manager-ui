package launcher

import (
	"context"
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
