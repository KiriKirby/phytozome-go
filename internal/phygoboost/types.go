package phygoboost

import (
	"context"
	"time"
)

type ExecLevel int

const (
	ExecInline ExecLevel = iota
	ExecMain
	ExecHeavy
)

type LocalGrant struct {
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
	LocalLevel  ExecLevel
	LocalSlots  int
	Network     map[string]int
	Description string
}

type TaskSpec struct {
	Level       ExecLevel
	Domain      string
	Network     map[string]int
	LocalSlots  int
	Description string
}

type BudgetProfile struct {
	LocalMain        int
	LocalHeavy       int
	NetworkMain      int
	PrefetchMain     int
	WorkerGOMAXPROCS int
}

type ResourceHandle struct {
	local     *LocalGrant
	networks  []*NetworkGrant
	released  bool
	releasers []func(time.Duration, error, bool)
}

type WorkflowReporter interface {
	DeclareResources(ctx context.Context, request ResourceRequest) (*ResourceHandle, error)
}
