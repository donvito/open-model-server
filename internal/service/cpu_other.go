//go:build !linux && !windows

package service

import "errors"

func readCPUSample() (cpuSample, error) {
	return cpuSample{}, errors.New("CPU utilization is unsupported on this platform")
}
