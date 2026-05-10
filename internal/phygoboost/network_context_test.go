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

func TestContextWithLocalGrantBindsLocalLookup(t *testing.T) {
	grant := &LocalGrant{Level: ExecHeavy, Slots: 2}
	ctx := contextWithLocalGrant(context.Background(), grant)
	got, ok := contextLocalGrant(ctx)
	if !ok || got == nil {
		t.Fatal("expected local grant to be visible in decorated context")
	}
	if got.Level != ExecHeavy || got.Slots != 2 {
		t.Fatalf("unexpected local grant: %#v", got)
	}
}
