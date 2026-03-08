package metadata

import (
	"os"
	"runtime"
)

type Snapshot struct {
	Hostname string
	Platform string
	Arch     string
}

func Collect() Snapshot {
	hostname, _ := os.Hostname()
	return Snapshot{
		Hostname: hostname,
		Platform: runtime.GOOS,
		Arch:     runtime.GOARCH,
	}
}
