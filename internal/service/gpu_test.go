package service

import "testing"

func TestParseNvidiaSMIOutput(t *testing.T) {
	devices, err := parseNvidiaSMIOutput("0, GPU-aaa, RTX One, 550.1, 37, 1024, 8192, 61, 125.5\n1, GPU-bbb, RTX Two, 550.1, N/A, N/A, N/A, N/A, N/A\n")
	if err != nil {
		t.Fatalf("parseNvidiaSMIOutput returned error: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("got %d devices, want 2", len(devices))
	}
	if devices[0].Index != 0 || devices[1].Index != 1 {
		t.Fatalf("unexpected device indexes: %#v", devices)
	}
	if devices[0].MemoryUsedBytes == nil || *devices[0].MemoryUsedBytes != 1024*1024*1024 {
		t.Fatalf("unexpected memory conversion: %#v", devices[0].MemoryUsedBytes)
	}
	if devices[1].UtilizationPercent != nil || devices[1].MemoryTotalBytes != nil {
		t.Fatalf("N/A fields should be omitted: %#v", devices[1])
	}
}

func TestParseNvidiaSMIOutputRetainsValidRows(t *testing.T) {
	devices, err := parseNvidiaSMIOutput("0, GPU-aaa, RTX One, 550.1, 37, 1024, 8192, 61, 125.5\nmalformed\n")
	if err == nil {
		t.Fatal("expected malformed row error")
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want one valid device", len(devices))
	}
}
