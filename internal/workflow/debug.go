package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/appfs"
)

func appendSessionDebugLog(format string, args ...any) {
	root, err := appfs.CacheRoot()
	if err != nil {
		return
	}
	path := filepath.Join(root, "session", "debug.log")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	line := strings.TrimSpace(fmt.Sprintf(format, args...))
	if line == "" {
		return
	}
	message := time.Now().Format(time.RFC3339Nano) + " " + line + "\n"
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.WriteString(message)
}
