package phygoboost

import (
	"context"
	"testing"
)

func TestNetworkGrantSnapshotFromContextClonesActiveDomains(t *testing.T) {
	ctx := contextWithNetworkGrantSnapshot(context.Background(), map[string]int{
		"rest.uniprot.org": 2,
		"":                 3,
		"www.ebi.ac.uk":    1,
	})
	got := networkGrantSnapshotFromContext(ctx)
	if len(got) != 2 {
		t.Fatalf("snapshot len = %d, want 2", len(got))
	}
	if got["rest.uniprot.org"] != 2 {
		t.Fatalf("uniprot snapshot = %d, want 2", got["rest.uniprot.org"])
	}
	if got["www.ebi.ac.uk"] != 1 {
		t.Fatalf("interpro snapshot = %d, want 1", got["www.ebi.ac.uk"])
	}
	got["rest.uniprot.org"] = 99
	if !contextHasNetworkGrant(ctx, "rest.uniprot.org") {
		t.Fatal("mutating snapshot should not affect original context grants")
	}
}

func TestContextWithNetworkGrantSnapshotBindsGrantLookup(t *testing.T) {
	ctx := contextWithNetworkGrantSnapshot(context.Background(), map[string]int{
		"phytozome-next.jgi.doe.gov": 1,
	})
	if !contextHasNetworkGrant(ctx, "phytozome-next.jgi.doe.gov") {
		t.Fatal("expected phytozome grant to be visible in decorated context")
	}
	if contextHasNetworkGrant(ctx, "rest.uniprot.org") {
		t.Fatal("unexpected grant for unrelated domain")
	}
}

func TestContextWithManagedGrantBindsManagedLookup(t *testing.T) {
	grant := &ManagedGrant{Level: ExecManaged, Slots: 1}
	ctx := contextWithManagedGrant(context.Background(), grant)
	got, ok := contextManagedGrant(ctx)
	if !ok || got == nil {
		t.Fatal("expected managed grant to be visible in decorated context")
	}
	if got.Level != ExecManaged || got.Slots != 1 {
		t.Fatalf("unexpected managed grant: %#v", got)
	}
}

func TestResourceRequestForPureNetworkTaskAddsImplicitManagedSlot(t *testing.T) {
	request := resourceRequestForTaskSpec(TaskSpec{
		Level:       ExecManaged,
		Domain:      "rest.uniprot.org",
		Description: "lookup uniprot entry",
	})
	if request.ManagedSlots != 1 {
		t.Fatalf("ManagedSlots = %d, want 1 for pure managed network task", request.ManagedSlots)
	}
	if got := request.Network["rest.uniprot.org"]; got != 1 {
		t.Fatalf("network slots = %d, want 1", got)
	}
}

func TestResourceRequestForPureManagedTaskAddsImplicitManagedSlot(t *testing.T) {
	request := resourceRequestForTaskSpec(TaskSpec{
		Level:       ExecManaged,
		Description: "managed coordination task",
	})
	if request.ManagedSlots != 1 {
		t.Fatalf("ManagedSlots = %d, want 1 for managed task", request.ManagedSlots)
	}
	if len(request.Network) != 0 {
		t.Fatalf("Network = %#v, want none", request.Network)
	}
}
