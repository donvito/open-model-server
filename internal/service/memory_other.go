//go:build !linux && !windows

package service

func readMemory() MemoryInfo { return MemoryInfo{} }
