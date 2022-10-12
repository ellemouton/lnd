package wtclient

import (
	"container/list"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/lightningnetwork/lnd/watchtower/wtdb"
)

var (
	// ErrAddressesExhausted signals that a AddressIterator has cycled
	// through all available addresses.
	ErrAddressesExhausted = errors.New("exhausted all addresses")

	// ErrAddrInUse indicates that an address is locked and cannot be
	// removed from the AddressIterator.
	ErrAddrInUse = errors.New("address in use")
)

// AddressIterator handles iteration over a list of addresses. It strictly
// disallows the list of addresses it holds to be empty. It also allows callers
// to place locks on certain addresses in order to prevent other callers from
// removing the addresses in question from the iterator.
type AddressIterator struct {
	mu             sync.Mutex
	queue          *list.List
	currentTopAddr *list.Element
	candidates     map[string]net.Addr
	lockedAddrs    map[string]bool
}

// newAddressIterator constructs a new AddressIterator.
func newAddressIterator(addrs ...net.Addr) (*AddressIterator, error) {
	if len(addrs) == 0 {
		return nil, fmt.Errorf("must have at least one address")
	}

	iter := &AddressIterator{
		queue:       list.New(),
		candidates:  make(map[string]net.Addr),
		lockedAddrs: make(map[string]bool),
	}

	for _, addr := range addrs {
		iter.queue.PushBack(addr.String())
		iter.candidates[addr.String()] = addr
	}
	iter.Reset()

	return iter, nil
}

// Reset clears the iterators state, and makes the address at the front of the
// list the next item to be returned.
func (a *AddressIterator) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.unsafeReset()
}

// unsafeReset clears the iterator state and makes the address at the front of
// the list the next item to be returned.
//
// NOTE: this method is not thread safe and so should only be called if the
// appropriate mutex is being held.
func (a *AddressIterator) unsafeReset() {
	// Reset the next candidate to the front of the linked-list.
	a.currentTopAddr = a.queue.Front()
}

// Next returns the next candidate address. This iterator will always return
// candidates in the order given when the iterator was instantiated. If no more
// candidates are available, ErrAddressesExhausted is returned.
func (a *AddressIterator) Next(lock bool) (net.Addr, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Set the next candidate to the subsequent element.
	a.currentTopAddr = a.currentTopAddr.Next()

	for a.currentTopAddr != nil {
		// Propose the address at the front of the list.
		addrID := a.currentTopAddr.Value.(string)

		// Check whether this address is still considered a candidate.
		// If it's not, we'll proceed to the next.
		addr, ok := a.candidates[addrID]
		if !ok {
			nextCandidate := a.currentTopAddr.Next()
			a.queue.Remove(a.currentTopAddr)
			a.currentTopAddr = nextCandidate
			continue
		}

		if lock {
			a.lockedAddrs[addr.String()] = true
		}

		return addr, nil
	}

	return nil, ErrAddressesExhausted
}

// Peek returns the current top of the address list. If the end of the list has
// been reached then the iterator is reset and the first item in the list is
// returned. Since the AddressIterator will never have an empty address list,
// this function will never return a nil value.
func (a *AddressIterator) Peek(lock bool) net.Addr {
	a.mu.Lock()
	defer a.mu.Unlock()

	for {
		// If currentTopAddr is nil, it means we have reached the end of
		// the list, so we reset it here. The iterator always has at
		// least one address, so we can be sure that currentTopAddr will
		// be non-nil after calling reset here.
		if a.currentTopAddr == nil {
			a.unsafeReset()
		}

		addrID := a.currentTopAddr.Value.(string)
		addr, ok := a.candidates[addrID]
		if !ok {
			nextCandidate := a.currentTopAddr.Next()
			a.queue.Remove(a.currentTopAddr)
			a.currentTopAddr = nextCandidate
			continue
		}

		if lock {
			a.lockedAddrs[addr.String()] = true
		}

		return addr
	}
}

// ReleaseLock releases the lock held on the given address.
func (a *AddressIterator) ReleaseLock(addr net.Addr) {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.lockedAddrs, addr.String())
}

// Add adds a new address to the iterator.
func (a *AddressIterator) Add(addr net.Addr) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.candidates[addr.String()]; ok {
		return
	}

	a.queue.PushBack(addr.String())
	a.candidates[addr.String()] = addr

	// If we've reached the end of our queue, then this candidate
	// will become the next.
	if a.currentTopAddr == nil {
		a.currentTopAddr = a.queue.Back()
	}
}

// Remove removes an existing address from the iterator. It disallows the
// address from being removed if it is the last address in the iterator or if
// there is currently a lock on the address.
func (a *AddressIterator) Remove(addr net.Addr) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.candidates[addr.String()]; !ok {
		return nil
	}

	if len(a.candidates) == 1 {
		return wtdb.ErrLastTowerAddr
	}

	if a.lockedAddrs[addr.String()] {
		return ErrAddrInUse
	}

	delete(a.candidates, addr.String())
	return nil
}

// HasLocked returns true if the AddressIterator has any locked addresses.
func (a *AddressIterator) HasLocked() bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	return len(a.lockedAddrs) > 0
}
