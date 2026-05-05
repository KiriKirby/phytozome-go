package report

func PlatformDisplayName(goos string) string {
	switch goos {
	case "windows":
		return "Windows"
	case "darwin":
		return "macOS"
	case "linux":
		return "Linux"
	case "freebsd":
		return "FreeBSD"
	case "openbsd":
		return "OpenBSD"
	case "netbsd":
		return "NetBSD"
	case "dragonfly":
		return "DragonFly BSD"
	case "solaris":
		return "Solaris"
	case "illumos":
		return "illumos"
	case "aix":
		return "AIX"
	case "android":
		return "Android"
	case "ios":
		return "iOS"
	case "js":
		return "JavaScript/WebAssembly"
	case "wasip1":
		return "WASI"
	default:
		if goos == "" {
			return "not available in this run"
		}
		return goos
	}
}
