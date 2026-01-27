package cidr

import (
	"testing"
)

func TestNewAllocator(t *testing.T) {
	alloc, err := NewAllocator("10.244.0.0/16", 24)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alloc.Total() != 256 {
		t.Errorf("expected 256 subnets, got %d", alloc.Total())
	}
}

func TestAllocateNext(t *testing.T) {
	alloc, _ := NewAllocator("10.244.0.0/16", 24)

	cidr1, err := alloc.AllocateNext()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cidr1 != "10.244.0.0/24" {
		t.Errorf("expected 10.244.0.0/24, got %s", cidr1)
	}

	cidr2, err := alloc.AllocateNext()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cidr2 != "10.244.1.0/24" {
		t.Errorf("expected 10.244.1.0/24, got %s", cidr2)
	}
}

func TestMarkAllocated(t *testing.T) {
	alloc, _ := NewAllocator("10.244.0.0/16", 24)

	err := alloc.MarkAllocated("10.244.5.0/24")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Next allocation should skip index 5
	for i := 0; i < 5; i++ {
		alloc.AllocateNext()
	}
	cidr, _ := alloc.AllocateNext()
	if cidr != "10.244.6.0/24" {
		t.Errorf("expected 10.244.6.0/24, got %s", cidr)
	}
}

func TestRelease(t *testing.T) {
	alloc, _ := NewAllocator("10.244.0.0/24", 26)

	cidr1, _ := alloc.AllocateNext()
	alloc.Release(cidr1)

	cidr2, _ := alloc.AllocateNext()
	if cidr1 != cidr2 {
		t.Errorf("expected released CIDR to be reused")
	}
}

func TestAllocatorExhausted(t *testing.T) {
	alloc, _ := NewAllocator("10.244.0.0/24", 26) // Only 4 subnets

	for i := 0; i < 4; i++ {
		_, err := alloc.AllocateNext()
		if err != nil {
			t.Fatalf("unexpected error on allocation %d: %v", i, err)
		}
	}

	_, err := alloc.AllocateNext()
	if err == nil {
		t.Error("expected error when CIDR exhausted")
	}
}

func TestMarkAllocatedOutOfRange(t *testing.T) {
	alloc, _ := NewAllocator("10.244.0.0/16", 24)

	err := alloc.MarkAllocated("192.168.0.0/24")
	if err == nil {
		t.Error("expected error for out-of-range CIDR")
	}
}
