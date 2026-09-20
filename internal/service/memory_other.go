//go:build !linux

package service

func readMemory() MemoryInfo { return MemoryInfo{} }
