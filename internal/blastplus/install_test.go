// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

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
