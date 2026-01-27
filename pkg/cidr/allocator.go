package cidr

import (
	"errors"
	"fmt"
	"net"
	"sync"
)

var (
	ErrCIDRExhausted  = errors.New("CIDR range exhausted")
	ErrCIDROutOfRange = errors.New("CIDR out of cluster range")
	ErrInvalidCIDR    = errors.New("invalid CIDR format")
)

type Allocator struct {
	mu            sync.Mutex
	clusterCIDR   *net.IPNet
	maskSize      int
	total         int
	allocated     []bool
	nextCandidate int
}

func NewAllocator(clusterCIDR string, nodeMaskSize int) (*Allocator, error) {
	_, ipnet, err := net.ParseCIDR(clusterCIDR)
	if err != nil {
		return nil, fmt.Errorf("invalid cluster CIDR: %w", err)
	}

	clusterMaskSize, _ := ipnet.Mask.Size()
	if nodeMaskSize <= clusterMaskSize {
		return nil, fmt.Errorf("node mask size (%d) must be larger than cluster mask size (%d)", nodeMaskSize, clusterMaskSize)
	}

	total := 1 << (nodeMaskSize - clusterMaskSize)

	return &Allocator{
		clusterCIDR:   ipnet,
		maskSize:      nodeMaskSize,
		total:         total,
		allocated:     make([]bool, total),
		nextCandidate: 0,
	}, nil
}

func (a *Allocator) Total() int {
	return a.total
}

func (a *Allocator) AllocateNext() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for i := 0; i < a.total; i++ {
		idx := (a.nextCandidate + i) % a.total
		if !a.allocated[idx] {
			a.allocated[idx] = true
			a.nextCandidate = (idx + 1) % a.total
			return a.indexToCIDR(idx), nil
		}
	}
	return "", ErrCIDRExhausted
}

func (a *Allocator) MarkAllocated(cidr string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	idx, err := a.cidrToIndex(cidr)
	if err != nil {
		return err
	}
	a.allocated[idx] = true
	return nil
}

func (a *Allocator) Release(cidr string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	idx, err := a.cidrToIndex(cidr)
	if err != nil {
		return err
	}
	a.allocated[idx] = false
	// Reset nextCandidate to allow immediate reuse of released CIDR
	if idx < a.nextCandidate {
		a.nextCandidate = idx
	}
	return nil
}

func (a *Allocator) IsAllocated(cidr string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	idx, err := a.cidrToIndex(cidr)
	if err != nil {
		return false
	}
	return a.allocated[idx]
}

func (a *Allocator) indexToCIDR(idx int) string {
	ip := make(net.IP, len(a.clusterCIDR.IP))
	copy(ip, a.clusterCIDR.IP)

	bitsToShift := 32 - a.maskSize
	offset := idx << bitsToShift

	ipInt := ipToUint32(ip)
	ipInt += uint32(offset)
	resultIP := uint32ToIP(ipInt)

	return fmt.Sprintf("%s/%d", resultIP.String(), a.maskSize)
}

func (a *Allocator) cidrToIndex(cidr string) (int, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return 0, ErrInvalidCIDR
	}

	maskSize, _ := ipnet.Mask.Size()
	if maskSize != a.maskSize {
		return 0, ErrCIDROutOfRange
	}

	if !a.clusterCIDR.Contains(ip) {
		return 0, ErrCIDROutOfRange
	}

	clusterIP := ipToUint32(a.clusterCIDR.IP)
	nodeIP := ipToUint32(ip.To4())
	bitsToShift := 32 - a.maskSize
	idx := int((nodeIP - clusterIP) >> bitsToShift)

	if idx < 0 || idx >= a.total {
		return 0, ErrCIDROutOfRange
	}

	return idx, nil
}

func ipToUint32(ip net.IP) uint32 {
	ip = ip.To4()
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

func uint32ToIP(n uint32) net.IP {
	return net.IPv4(byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
}
