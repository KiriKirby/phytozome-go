package heavy

import (
	"context"
	"testing"

	boostipc "github.com/KiriKirby/phytozome-go/internal/phygoboost/ipc"
)

func TestNewPreservesTaskContextDecorator(t *testing.T) {
	called := false
	decorator := func(ctx context.Context, msg boostipc.Message) context.Context {
		called = true
		if msg.Task != "demo" {
			t.Fatalf("task = %q, want demo", msg.Task)
		}
		return context.WithValue(ctx, "decorated", true)
	}
	host := New(nil, decorator)
	if host == nil {
		t.Fatal("New returned nil host")
	}
	ctx := host.taskCtxDecorator(context.Background(), boostipc.Message{Task: "demo"})
	if !called {
		t.Fatal("task context decorator was not invoked")
	}
	if value, _ := ctx.Value("decorated").(bool); !value {
		t.Fatal("decorator result was not preserved on context")
	}
}
