//go:build !linux

package agent

import "os"

func dnsWorkerUpdateFileUID(info os.FileInfo) (uint32, bool) {
	return 0, true
}
