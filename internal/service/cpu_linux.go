//go:build linux

package service

import (
	"bufio"
	"errors"
	"os"
	"strconv"
	"strings"
)

func readCPUSample() (cpuSample, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return cpuSample{}, errors.New("CPU utilization unavailable: cannot read /proc/stat")
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if !strings.HasPrefix(sc.Text(), "cpu ") {
			continue
		}
		sample, err := parseCPUStat(sc.Text())
		if err != nil {
			return cpuSample{}, err
		}
		return sample, nil
	}
	if err := sc.Err(); err != nil {
		return cpuSample{}, errors.New("CPU utilization unavailable: cannot read /proc/stat")
	}
	return cpuSample{}, errors.New("CPU utilization unavailable: aggregate CPU counters not found")
}

func parseCPUStat(line string) (cpuSample, error) {
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return cpuSample{}, errors.New("CPU utilization unavailable: malformed /proc/stat")
	}

	var sample cpuSample
	// Linux reports guest and guest_nice as subsets of user and nice. The
	// first eight counters are therefore the non-overlapping total.
	for i, field := range fields[1:] {
		if i >= 8 {
			break
		}
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return cpuSample{}, errors.New("CPU utilization unavailable: malformed /proc/stat counter")
		}
		if ^uint64(0)-sample.total < value {
			return cpuSample{}, errors.New("CPU utilization unavailable: /proc/stat counter overflow")
		}
		sample.total += value
		if i == 3 || i == 4 {
			if ^uint64(0)-sample.idle < value {
				return cpuSample{}, errors.New("CPU utilization unavailable: /proc/stat idle counter overflow")
			}
			sample.idle += value
		}
	}
	return sample, nil
}
