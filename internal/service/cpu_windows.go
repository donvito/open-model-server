//go:build windows

package service

import (
	"errors"
	"syscall"
	"unsafe"
)

var getSystemTimes = syscall.NewLazyDLL("kernel32.dll").NewProc("GetSystemTimes")

type systemTimeFile struct {
	lowDateTime  uint32
	highDateTime uint32
}

func readCPUSample() (cpuSample, error) {
	var idle, kernel, user systemTimeFile
	r, _, callErr := getSystemTimes.Call(
		uintptr(unsafe.Pointer(&idle)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)),
	)
	if r == 0 {
		if callErr == nil {
			callErr = errors.New("GetSystemTimes failed")
		}
		return cpuSample{}, errors.New("CPU utilization unavailable: " + callErr.Error())
	}

	idleTicks := fileTimeValue(idle)
	total := fileTimeValue(kernel) + fileTimeValue(user)
	if total < idleTicks {
		return cpuSample{}, errors.New("CPU utilization unavailable: inconsistent GetSystemTimes counters")
	}
	return cpuSample{total: total, idle: idleTicks}, nil
}

func fileTimeValue(value systemTimeFile) uint64 {
	return uint64(value.highDateTime)<<32 | uint64(value.lowDateTime)
}
