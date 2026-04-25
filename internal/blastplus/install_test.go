package blastplus

import "testing"

func TestApplicationDirUsesExecutableDirWhenStable(t *testing.T) {
	oldExecutableFn := executableFn
	oldGetwdFn := getwdFn
	oldTempDirFn := tempDirFn
	t.Cleanup(func() {
		executableFn = oldExecutableFn
		getwdFn = oldGetwdFn
		tempDirFn = oldTempDirFn
	})

	executableFn = func() (string, error) { return `C:\tools\phytozome-go\phytozome-go.exe`, nil }
	getwdFn = func() (string, error) { return `C:\workspace`, nil }
	tempDirFn = func() string { return `C:\Users\me\AppData\Local\Temp` }

	dir, err := applicationDir()
	if err != nil {
		t.Fatalf("applicationDir returned error: %v", err)
	}
	if dir != `C:\tools\phytozome-go` {
		t.Fatalf("unexpected application dir: got %q", dir)
	}
}

func TestApplicationDirFallsBackToWorkingDirForTempExecutable(t *testing.T) {
	oldExecutableFn := executableFn
	oldGetwdFn := getwdFn
	oldTempDirFn := tempDirFn
	t.Cleanup(func() {
		executableFn = oldExecutableFn
		getwdFn = oldGetwdFn
		tempDirFn = oldTempDirFn
	})

	executableFn = func() (string, error) { return `C:\Users\me\AppData\Local\Temp\go-build123\phytozome-go.exe`, nil }
	getwdFn = func() (string, error) { return `C:\workspace`, nil }
	tempDirFn = func() string { return `C:\Users\me\AppData\Local\Temp` }

	dir, err := applicationDir()
	if err != nil {
		t.Fatalf("applicationDir returned error: %v", err)
	}
	if dir != `C:\workspace` {
		t.Fatalf("unexpected fallback application dir: got %q", dir)
	}
}
