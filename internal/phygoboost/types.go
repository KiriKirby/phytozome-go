package phygoboost

import (
	"context"
	"time"
)

type ExecLevel int

const (
	ExecUnmanaged ExecLevel = iota
	ExecManaged
)

type ManagedGrant struct {
	ID       string
	Level    ExecLevel
	Slots    int
	Acquired time.Time
}

type NetworkGrant struct {
	Domain   string
	Slots    int
	Acquired time.Time
}

type ResourceRequest struct {
	ManagedLevel ExecLevel
	ManagedSlots int
	Network      map[string]int
	Description  string
}

type TaskSpec struct {
	Level       ExecLevel
	Domain      string
	Network     map[string]int
	ForceOwnResources bool
	Description string
}

type BudgetProfile struct {
	ManagedTotal       int
	SubprocessParallel int
}

type PoolControlProfile struct {
	CPU               int
	ManagedTotal      int
	UIReservedCPU     int
	MemorySoftLimit   uint64
	MemoryHardLimit   uint64
	SystemCPUPercent  float64
	MemoryUsedPercent float64
	MaxIdleConns      int
	MaxIdlePerHost    int
	HTTPTimeout       time.Duration
	IdleConnTimeout   time.Duration
	TLSHandshake      time.Duration
	ExpectContinue    time.Duration
	MemoryCache       int64
}

type runtimeSnapshot struct {
	ProcessRSS     uint64
	SystemUsed     uint64
	SystemTotal    uint64
	SystemCPU      float64
	ChildProcesses int
	ActiveWorkers  int
	Pressure       PressureLevel
}

type ResourceHandle struct {
	managed   *ManagedGrant
	networks  []*NetworkGrant
	released  bool
	releasers []func(time.Duration, error, bool)
}

type WorkflowReporter interface {
	DeclareResources(ctx context.Context, request ResourceRequest) (*ResourceHandle, error)
}
