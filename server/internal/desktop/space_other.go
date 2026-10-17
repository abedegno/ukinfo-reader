//go:build !darwin && !linux

package desktop

import "fmt"

func AvailableBytes(path string) (uint64, error) {
	return 0, fmt.Errorf("automatic local disk checks are not supported on this platform")
}
