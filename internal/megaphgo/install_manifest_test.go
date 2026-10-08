package megaphgo

import (
	"strings"
	"testing"
)

func TestLoadRuntimeReleaseManifestReturnsErrorsInsteadOfPanicking(t *testing.T) {
	original := runtimeReleaseManifestJSON
	t.Cleanup(func() { runtimeReleaseManifestJSON = original })

	for _, data := range [][]byte{
		[]byte("{"),
		[]byte(`{"assets":{"windows-amd64":"runtime.zip"}}`),
		[]byte(`{"release_tag":"v1","assets":{}}`),
		[]byte(`{"release_tag":"v1","assets":{"windows-amd64":""}}`),
	} {
		runtimeReleaseManifestJSON = data
		if _, err := loadRuntimeReleaseManifest(); err == nil {
			t.Fatalf("loadRuntimeReleaseManifest(%q) returned nil error", strings.TrimSpace(string(data)))
		}
	}
}
