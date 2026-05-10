package network

import (
	"net/http"
	"testing"
)

func TestNewManagerPreservesProvidedClient(t *testing.T) {
	client := &http.Client{}
	manager := NewManager(client, nil)
	if manager.HTTPClient() != client {
		t.Fatal("NewManager should preserve provided client")
	}
}

func TestNewManagerCreatesClientWhenNil(t *testing.T) {
	manager := NewManager(nil, nil)
	if manager.HTTPClient() == nil {
		t.Fatal("NewManager should always expose a client")
	}
}

func TestNormalizeDomain(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "https://REST.UniProt.org/uniprotkb/search", want: "rest.uniprot.org"},
		{input: "www.ebi.ac.uk", want: "www.ebi.ac.uk"},
		{input: "", want: "unknown"},
	}
	for _, tt := range tests {
		if got := normalizeDomain(tt.input); got != tt.want {
			t.Fatalf("normalizeDomain(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
