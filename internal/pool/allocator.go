// Package pool provides IP address allocation using a bitmap for O(1) operations.
package pool

import (
	"fmt"
	"net"
	"sync"

	"github.com/athena-dhcpd/athena-dhcpd/pkg/dhcpv4"
)

// Pool represents an IP address pool with bitmap allocation.
type Pool struct {
	Name       string
	Start      net.IP
	End        net.IP
	Network    *net.IPNet
	startU     uint32
	endU       uint32
	size       uint32
	bitmap     []uint64 // 1 bit per IP: 1=allocated, 0=free
	allocated  uint32
	mu         sync.Mutex

	// Pool matching criteria
	MatchCircuitID   string
	MatchRemoteID    string
	MatchVendorClass string
	LeaseTime        string
}

// NewPool creates a new IP pool from a range within a network.
func NewPool(name string, start, end net.IP, network *net.IPNet) (*Pool, error) {
	startU := dhcpv4.IPToUint32(start.To4())
	endU := dhcpv4.IPToUint32(end.To4())

	if endU < startU {
		return nil, fmt.Errorf("pool %s: end %s is before start %s", name, end, start)
	}

	if !network.Contains(start) {
		return nil, fmt.Errorf("pool %s: start %s not in network %s", name, start, network)
	}
	if !network.Contains(end) {
		return nil, fmt.Errorf("pool %s: end %s not in network %s", name, end, network)
	}

	size := endU - startU + 1
	bitmapSize := (size + 63) / 64

	return &Pool{
		Name:    name,
		Start:   start.To4(),
		End:     end.To4(),
		Network: network,
		startU:  startU,
		endU:    endU,
		size:    size,
		bitmap:  make([]uint64, bitmapSize),
	}, nil
}

// Size returns the total number of IPs in the pool.
func (p *Pool) Size() uint32 {
	return p.size
}

// Allocated returns the number of allocated IPs.
func (p *Pool) Allocated() uint32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.allocated
}

// Available returns the number of free IPs.
func (p *Pool) Available() uint32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.size - p.allocated
}

// Utilization returns the pool utilization as a percentage.
func (p *Pool) Utilization() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.size == 0 {
		return 0
	}
	return float64(p.allocated) / float64(p.size) * 100
}

// ipToOffset converts an IP to a bitmap offset.
func (p *Pool) ipToOffset(ip net.IP) (uint32, bool) {
	u := dhcpv4.IPToUint32(ip.To4())
	if u < p.startU || u > p.endU {
		return 0, false
	}
	return u - p.startU, true
}

// offsetToIP converts a bitmap offset to an IP.
func (p *Pool) offsetToIP(offset uint32) net.IP {
	return dhcpv4.Uint32ToIP(p.startU + offset)
}

// isSet returns true if the bit at offset is set (allocated).
func (p *Pool) isSet(offset uint32) bool {
	word := offset / 64
	bit := offset % 64
	return p.bitmap[word]&(1<<bit) != 0
}

// set marks an IP as allocated.
func (p *Pool) set(offset uint32) {
	word := offset / 64
	bit := offset % 64
	if p.bitmap[word]&(1<<bit) == 0 {
		p.bitmap[word] |= 1 << bit
		p.allocated++
	}
}

// clear marks an IP as free.
func (p *Pool) clear(offset uint32) {
	word := offset / 64
	bit := offset % 64
	if p.bitmap[word]&(1<<bit) != 0 {
		p.bitmap[word] &^= 1 << bit
		p.allocated--
	}
}

// Allocate finds the next free IP in the pool. Returns nil if pool is full.
// Uses bitmap scanning — NOT a linear IP scan.
func (p *Pool) Allocate() net.IP {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.allocated >= p.size {
		return nil
	}

	// Scan bitmap words for a word with at least one free bit
	for w := uint32(0); w < uint32(len(p.bitmap)); w++ {
		if p.bitmap[w] == ^uint64(0) {
			continue // All bits set in this word
		}
		// Find first zero bit in this word
		word := p.bitmap[w]
		for bit := uint32(0); bit < 64; bit++ {
			if word&(1<<bit) == 0 {
				offset := w*64 + bit
				if offset >= p.size {
					return nil // Past end of pool
				}
				p.set(offset)
				return p.offsetToIP(offset)
			}
		}
	}

	return nil
}

// AllocateSpecific tries to allocate a specific IP. Returns false if already allocated or out of range.
func (p *Pool) AllocateSpecific(ip net.IP) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	offset, ok := p.ipToOffset(ip)
	if !ok {
		return false
	}
	if p.isSet(offset) {
		return false
	}
	p.set(offset)
	return true
}

// Release frees a previously allocated IP.
func (p *Pool) Release(ip net.IP) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	offset, ok := p.ipToOffset(ip)
	if !ok {
		return false
	}
	if !p.isSet(offset) {
		return false
	}
	p.clear(offset)
	return true
}

// Contains checks if an IP is within this pool's range.
func (p *Pool) Contains(ip net.IP) bool {
	u := dhcpv4.IPToUint32(ip.To4())
	return u >= p.startU && u <= p.endU
}

// IsAllocated checks if a specific IP is allocated.
func (p *Pool) IsAllocated(ip net.IP) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	offset, ok := p.ipToOffset(ip)
	if !ok {
		return false
	}
	return p.isSet(offset)
}

// AllocateN returns up to n free IPs without marking them as allocated.
// Useful for parallel conflict probing.
func (p *Pool) AllocateN(n int) []net.IP {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.allocated >= p.size {
		return nil
	}

	var ips []net.IP
	for w := uint32(0); w < uint32(len(p.bitmap)) && len(ips) < n; w++ {
		if p.bitmap[w] == ^uint64(0) {
			continue
		}
		word := p.bitmap[w]
		for bit := uint32(0); bit < 64 && len(ips) < n; bit++ {
			if word&(1<<bit) == 0 {
				offset := w*64 + bit
				if offset >= p.size {
					return ips
				}
				ips = append(ips, p.offsetToIP(offset))
			}
		}
	}

	return ips
}

// String returns a human-readable pool description.
func (p *Pool) String() string {
	return fmt.Sprintf("%s (%s-%s, %d/%d used)", p.Name, p.Start, p.End, p.allocated, p.size)
}

// RangeString returns the pool range as "start-end".
func (p *Pool) RangeString() string {
	return fmt.Sprintf("%s-%s", p.Start, p.End)
}
