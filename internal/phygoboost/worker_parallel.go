package phygoboost

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-json"
)

type IndexedTask[Item any] struct {
	Index int  `json:"index"`
	Item  Item `json:"item"`
}

func RegisterIndexedJSON[Item any, Out any](name string, handler func(context.Context, int, Item) (Out, error)) {
	Register(name, func(ctx context.Context, payload []byte) ([]byte, error) {
		var input IndexedTask[Item]
		if err := json.Unmarshal(payload, &input); err != nil {
			return nil, fmt.Errorf("decode indexed worker input: %w", err)
		}
		output, err := handler(ctx, input.Index, input.Item)
		if err != nil {
			return nil, err
		}
		return json.Marshal(output)
	})
}

func ParallelTaskJSON[Item any, Out any](ctx context.Context, spec TaskSpec, taskName string, items []Item, outputs []Out) error {
	if len(items) == 0 {
		return nil
	}
	if len(outputs) < len(items) {
		return fmt.Errorf("worker output slice too small: have %d, need %d", len(outputs), len(items))
	}
	if !Registered(taskName) {
		return fmt.Errorf("worker task %q is not registered", taskName)
	}
	if runningTestBinary() {
		handler := registry[taskName]
		parallelSpec := ParallelSpec{Level: spec.Level, Domain: spec.Domain, Workers: parallelTaskWorkers(spec, len(items)), Description: spec.Description}
		if parallelSpec.Domain == "" && len(spec.Network) == 1 {
			for domain := range spec.Network {
				parallelSpec.Domain = domain
			}
		}
		return ParallelForSpec(ctx, parallelSpec, len(items), func(ctx context.Context, index int) error {
			payload, err := json.Marshal(IndexedTask[Item]{Index: index, Item: items[index]})
			if err != nil {
				return err
			}
			outPayload, err := handler(ctx, payload)
			if err != nil {
				return err
			}
			var out Out
			if err := json.Unmarshal(outPayload, &out); err != nil {
				return err
			}
			outputs[index] = out
			return nil
		})
	}
	parallelSpec := ParallelSpec{Level: spec.Level, Domain: spec.Domain, Workers: parallelTaskWorkers(spec, len(items)), Description: spec.Description}
	if parallelSpec.Domain == "" && len(spec.Network) == 1 {
		for domain := range spec.Network {
			parallelSpec.Domain = domain
		}
	}
	return ParallelForSpec(ctx, parallelSpec, len(items), func(ctx context.Context, index int) error {
		var out Out
		if err := RunTaskJSON(ctx, spec, taskName, IndexedTask[Item]{Index: index, Item: items[index]}, &out); err != nil {
			return err
		}
		outputs[index] = out
		return nil
	})
}

func parallelTaskWorkers(spec TaskSpec, total int) int {
	if total <= 0 {
		return 0
	}
	hasNetwork := strings.TrimSpace(spec.Domain) != "" || len(spec.Network) > 0
	switch {
	case hasNetwork && spec.Level == ExecHeavy:
		return minPositiveWorkerCount(NetworkProcessWorkers(total), ProcessWorkers(total))
	case hasNetwork:
		return NetworkWorkers(total)
	case spec.Level == ExecHeavy:
		return ProcessWorkers(total)
	default:
		return CPUWorkers(total)
	}
}

func minPositiveWorkerCount(values ...int) int {
	best := 0
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if best == 0 || value < best {
			best = value
		}
	}
	return best
}

func runningTestBinary() bool {
	name := strings.ToLower(filepath.Base(os.Args[0]))
	return strings.HasSuffix(name, ".test") || strings.HasSuffix(name, ".test.exe")
}

