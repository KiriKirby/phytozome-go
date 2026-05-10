package heavy

import (
	"context"
	"fmt"
	"io"
	"sync"

	boostipc "github.com/KiriKirby/phytozome-go/internal/phygoboost/ipc"
)

type Handler func(context.Context, []byte) ([]byte, error)
type TaskContextDecorator func(context.Context, boostipc.Message) context.Context

type Host struct {
	handlers         map[string]Handler
	taskCtxDecorator TaskContextDecorator
	mu               sync.Mutex
	running          map[string]context.CancelFunc
	wg               sync.WaitGroup
}

func New(handlers map[string]Handler, decorator TaskContextDecorator) *Host {
	if handlers == nil {
		handlers = map[string]Handler{}
	}
	return &Host{
		handlers:         handlers,
		taskCtxDecorator: decorator,
		running:          make(map[string]context.CancelFunc),
	}
}

func (h *Host) Serve(ctx context.Context, reader io.Reader, writer io.Writer) error {
	bus := boostipc.New(reader, writer)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		msg, err := bus.Receive(ctx)
		if err != nil {
			return err
		}
		switch msg.Type {
		case "shutdown":
			h.cancelAll()
			h.wg.Wait()
			return nil
		case "cancel":
			h.cancel(msg.ID)
			continue
		}
		handler := h.handlers[msg.Task]
		if handler == nil {
			if sendErr := bus.Send(ctx, boostipc.Message{
				ID:    msg.ID,
				Type:  "result",
				Error: fmt.Sprintf("unregistered task %q", msg.Task),
			}); sendErr != nil {
				return sendErr
			}
			continue
		}
		taskCtx, cancel := context.WithCancel(ctx)
		if h.taskCtxDecorator != nil {
			taskCtx = h.taskCtxDecorator(taskCtx, msg)
		}
		h.register(msg.ID, cancel)
		h.wg.Add(1)
		go func(task boostipc.Message, taskCtx context.Context) {
			defer h.wg.Done()
			defer h.unregister(task.ID)
			output, runErr := handler(taskCtx, task.Payload)
			reply := boostipc.Message{
				ID:     task.ID,
				Type:   "result",
				Output: output,
			}
			if runErr != nil {
				reply.Error = runErr.Error()
			}
			_ = bus.Send(context.Background(), reply)
		}(msg, taskCtx)
	}
}

func (h *Host) register(id string, cancel context.CancelFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if id == "" || cancel == nil {
		return
	}
	h.running[id] = cancel
}

func (h *Host) unregister(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.running, id)
}

func (h *Host) cancel(id string) {
	h.mu.Lock()
	cancel := h.running[id]
	h.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (h *Host) cancelAll() {
	h.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(h.running))
	for _, cancel := range h.running {
		if cancel != nil {
			cancels = append(cancels, cancel)
		}
	}
	h.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}
