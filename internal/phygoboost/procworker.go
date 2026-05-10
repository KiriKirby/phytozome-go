package phygoboost

import (
	"context"
	"fmt"
	"strings"

	"github.com/goccy/go-json"
)

type Handler func(context.Context, []byte) ([]byte, error)

var registry = map[string]Handler{}

func Register(name string, handler Handler) {
	name = strings.TrimSpace(name)
	if name == "" || handler == nil {
		return
	}
	registry[name] = handler
}

func Registered(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	return registry[name] != nil
}

func InWorker() bool {
	return heavyMode()
}

func RunIfWorker(ctx context.Context) (bool, error) {
	if !InWorker() {
		return false, nil
	}
	return true, runHeavyWorkerLoop(ctx)
}

func RunTaskJSON[In any, Out any](ctx context.Context, spec TaskSpec, taskName string, input In, output *Out) error {
	payload, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("encode worker input for %s: %w", taskName, err)
	}
	out, err := RunTask(ctx, spec, taskName, payload)
	if err != nil {
		return err
	}
	if output == nil {
		return nil
	}
	if len(out) == 0 {
		var zero Out
		*output = zero
		return nil
	}
	if err := json.Unmarshal(out, output); err != nil {
		return fmt.Errorf("decode worker output for %s: %w", taskName, err)
	}
	return nil
}

func RunTask(ctx context.Context, spec TaskSpec, taskName string, input []byte) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	taskName = strings.TrimSpace(taskName)
	if taskName == "" {
		return nil, fmt.Errorf("worker task is empty")
	}
	if runningTestBinary() {
		if handler := registry[taskName]; handler != nil {
			return handler(ctx, input)
		}
	}
	request := ResourceRequest{
		LocalLevel:  spec.Level,
		LocalSlots:  spec.LocalSlots,
		Description: spec.Description,
	}
	request = missingResourceRequestForTaskSpec(ctx, spec)
	if resourceRequestIsEmpty(request) {
		return dispatchHeavyTask(ctx, taskName, input)
	}
	handle, err := DeclareResources(ctx, request)
	if err != nil {
		return nil, err
	}
	defer handle.Release()
	return dispatchHeavyTask(BindDeclaredResources(ctx, handle), taskName, input)
}
