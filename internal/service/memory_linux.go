package service

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

func readMemory() MemoryInfo {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return MemoryInfo{}
	}
	defer f.Close()
	var mi MemoryInfo
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		kb, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			mi.TotalBytes = kb * 1024
		case "MemAvailable:":
			mi.AvailableBytes = kb * 1024
		}
	}
	return mi
}
