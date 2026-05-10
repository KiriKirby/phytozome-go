package phygoboost

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	boostheavy "github.com/KiriKirby/phytozome-go/internal/phygoboost/heavy"
	boostipc "github.com/KiriKirby/phytozome-go/internal/phygoboost/ipc"
)

const heavyEnvName = "PHYTOZOME_GO_HEAVY"

type heavyClient struct {
	mu       sync.Mutex
	cmd      *exec.Cmd
	bus      *boostipc.Bus
	stdout   io.ReadCloser
	stdin    io.WriteCloser
	waiters  map[string]chan boostipc.Message
	done     chan struct{}
	closed   bool
	nextID   atomic.Uint64
	started  bool
}

var heavyOnce sync.Once
var heavyInst *heavyClient

func heavyCoordinator() *heavyClient {
	heavyOnce.Do(func() {
		heavyInst = &heavyClient{}
	})
	return heavyInst
}

func heavyMode() bool {
	return strings.TrimSpace(os.Getenv(heavyEnvName)) == "1"
}

func runHeavyWorkerLoop(ctx context.Context) error {
	handlers := make(map[string]boostheavy.Handler, len(registry))
	for key, handler := range registry {
		h := handler
		handlers[key] = func(ctx context.Context, payload []byte) ([]byte, error) {
			return h(ctx, payload)
		}
	}
	host := boostheavy.New(handlers, func(taskCtx context.Context, msg boostipc.Message) context.Context {
		return contextWithNetworkGrantSnapshot(taskCtx, msg.Network)
	})
	return host.Serve(ctx, os.Stdin, os.Stdout)
}

func dispatchHeavyTask(ctx context.Context, taskName string, payload []byte) ([]byte, error) {
	client := heavyCoordinator()
	if err := client.ensureStarted(ctx); err != nil {
		return nil, err
	}
	id := fmt.Sprintf("task-%d", client.nextID.Add(1))
	waiter := make(chan boostipc.Message, 1)
	if err := client.registerWaiter(id, waiter); err != nil {
		return nil, err
	}
	defer client.unregisterWaiter(id)
	if err := client.send(ctx, boostipc.Message{
		ID:      id,
		Type:    "run",
		Task:    taskName,
		Payload: payload,
		Network: networkGrantSnapshotFromContext(ctx),
	}); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		_ = client.send(context.Background(), boostipc.Message{ID: id, Type: "cancel"})
		return nil, ctx.Err()
	case <-client.closedCh():
		return nil, fmt.Errorf("heavy host stopped")
	case reply := <-waiter:
		if strings.TrimSpace(reply.Error) != "" {
			return nil, fmt.Errorf("heavy task %s failed: %s", taskName, reply.Error)
		}
		return reply.Output, nil
	}
}

func (c *heavyClient) ensureStarted(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started && c.cmd != nil && c.cmd.Process != nil && c.cmd.ProcessState == nil && c.bus != nil && c.done != nil {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(context.Background(), exe)
	cmd.Env = append(os.Environ(),
		heavyEnvName+"=1",
		"PHYTOZOME_GO_HEAVY_PARENT="+strconv.Itoa(os.Getpid()),
	)
	cmd.Env = append(cmd.Env, workerEnv(workProcess)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	c.cmd = cmd
	c.bus = boostipc.New(stdout, stdin)
	c.stdout = stdout
	c.stdin = stdin
	c.done = make(chan struct{})
	c.waiters = make(map[string]chan boostipc.Message)
	c.closed = false
	c.started = true
	go c.receiveLoop()
	go c.waitLoop()
	go func() {
		<-ctx.Done()
		c.shutdown()
	}()
	return nil
}

func (c *heavyClient) shutdown() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	bus := c.bus
	cmd := c.cmd
	done := c.done
	stdin := c.stdin
	waiters := c.waiters
	c.mu.Unlock()
	if bus != nil {
		_ = bus.Send(context.Background(), boostipc.Message{Type: "shutdown"})
	}
	if stdin != nil {
		_ = stdin.Close()
	}
	if cmd != nil && cmd.Process != nil && cmd.ProcessState == nil {
		_ = cmd.Process.Kill()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(500 * time.Millisecond):
		}
	}
	c.mu.Lock()
	for id, waiter := range waiters {
		delete(c.waiters, id)
		close(waiter)
	}
	c.bus = nil
	c.cmd = nil
	c.stdout = nil
	c.stdin = nil
	c.done = nil
	c.started = false
	c.mu.Unlock()
}

func closeHeavyHost() {
	if heavyInst != nil {
		heavyInst.shutdown()
	}
	time.Sleep(10 * time.Millisecond)
}

func (c *heavyClient) send(ctx context.Context, msg boostipc.Message) error {
	c.mu.Lock()
	bus := c.bus
	closed := c.closed
	c.mu.Unlock()
	if closed || bus == nil {
		return fmt.Errorf("heavy host is not running")
	}
	return bus.Send(ctx, msg)
}

func (c *heavyClient) registerWaiter(id string, waiter chan boostipc.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fmt.Errorf("heavy host is closed")
	}
	if c.waiters == nil {
		c.waiters = make(map[string]chan boostipc.Message)
	}
	c.waiters[id] = waiter
	return nil
}

func (c *heavyClient) unregisterWaiter(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.waiters, id)
}

func (c *heavyClient) closedCh() <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return c.done
}

func (c *heavyClient) receiveLoop() {
	for {
		c.mu.Lock()
		bus := c.bus
		done := c.done
		closed := c.closed
		c.mu.Unlock()
		if closed || bus == nil || done == nil {
			return
		}
		reply, err := bus.Receive(context.Background())
		if err != nil {
			c.finish(err)
			return
		}
		c.mu.Lock()
		waiter := c.waiters[reply.ID]
		c.mu.Unlock()
		if waiter != nil {
			select {
			case waiter <- reply:
			default:
			}
		}
	}
}

func (c *heavyClient) waitLoop() {
	c.mu.Lock()
	cmd := c.cmd
	c.mu.Unlock()
	if cmd == nil {
		return
	}
	err := cmd.Wait()
	c.finish(err)
}

func (c *heavyClient) finish(_ error) {
	c.mu.Lock()
	if c.done == nil {
		c.mu.Unlock()
		return
	}
	done := c.done
	waiters := c.waiters
	c.waiters = make(map[string]chan boostipc.Message)
	c.bus = nil
	c.cmd = nil
	c.stdout = nil
	c.stdin = nil
	c.done = nil
	c.started = false
	c.closed = true
	c.mu.Unlock()
	for _, waiter := range waiters {
		close(waiter)
	}
	close(done)
}
