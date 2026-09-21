//go:build windows

package service

import (
	"syscall"
	"unsafe"
)

var globalMemoryStatusEx = syscall.NewLazyDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

type memoryStatusEx struct {
	dwLength                uint32
	dwMemoryLoad            uint32
	ullTotalPhys            uint64
	ullAvailPhys            uint64
	ullTotalPageFile        uint64
	ullAvailPageFile        uint64
	ullTotalVirtual         uint64
	ullAvailVirtual         uint64
	ullAvailExtendedVirtual uint64
}

func readMemory() MemoryInfo {
	status := memoryStatusEx{dwLength: uint32(unsafe.Sizeof(memoryStatusEx{}))}
	r, _, _ := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&status)))
	if r == 0 {
		return MemoryInfo{}
	}
	return MemoryInfo{
		TotalBytes:     status.ullTotalPhys,
		AvailableBytes: status.ullAvailPhys,
	}
}
