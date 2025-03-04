// SPDX-License-Identifier: Apache-2.0
// Copyright 2018-2019 Authors of Cilium

package testidentity

import (
	"context"
	"net"

	"github.com/cilium/cilium/pkg/identity"
	"github.com/cilium/cilium/pkg/identity/cache"
	"github.com/cilium/cilium/pkg/labels"
)

type IdentityAllocatorOwnerMock struct{}

func (i *IdentityAllocatorOwnerMock) UpdateIdentities(added, deleted cache.IdentityCache) {}

func (i *IdentityAllocatorOwnerMock) GetNodeSuffix() string {
	return "foo"
}

// MockIdentityAllocator is used as a mock identity allocator for unit tests.
type MockIdentityAllocator struct {
	cache.IdentityCache

	currentID        int // Regular identities
	localID          int // CIDR identities
	idRefCount       map[int]int
	ipToIdentity     map[string]int
	idToIdentity     map[int]*identity.Identity
	labelsToIdentity map[string]int // labels are sorted as a key
}

// NewMockIdentityAllocator returns a new mock identity allocator to be used
// for unit testing purposes. It can be used as a drop-in for "real" identity
// allocation in a testing context.
func NewMockIdentityAllocator(c cache.IdentityCache) *MockIdentityAllocator {
	if c == nil {
		c = cache.IdentityCache{}
	}
	return &MockIdentityAllocator{
		IdentityCache: c,

		currentID:        1000,
		localID:          int(identity.LocalIdentityFlag),
		ipToIdentity:     make(map[string]int),
		idRefCount:       make(map[int]int),
		idToIdentity:     make(map[int]*identity.Identity),
		labelsToIdentity: make(map[string]int),
	}
}

// WaitForInitialGlobalIdentities does nothing.
func (f *MockIdentityAllocator) WaitForInitialGlobalIdentities(context.Context) error {
	return nil
}

// IsLocalIdentityAllocatorInitialized returns true.
func (f *MockIdentityAllocator) IsLocalIdentityAllocatorInitialized() bool {
	return true
}

// GetIdentities returns the identities from the identity cache.
func (f *MockIdentityAllocator) GetIdentities() cache.IdentitiesModel {
	result := cache.IdentitiesModel{}
	return result.FromIdentityCache(f.IdentityCache)
}

// AllocateIdentity allocates a fake identity. It is meant to generally mock
// the canonical identity allocator logic.
func (f *MockIdentityAllocator) AllocateIdentity(_ context.Context, lbls labels.Labels, _ bool) (*identity.Identity, bool, error) {
	if reservedIdentity := identity.LookupReservedIdentityByLabels(lbls); reservedIdentity != nil {
		return reservedIdentity, false, nil
	}

	var id int
	if identity.RequiresGlobalIdentity(lbls) {
		id = f.currentID
		f.currentID++
	} else {
		id = f.localID
		f.localID++
	}

	f.IdentityCache[identity.NumericIdentity(id)] = lbls.LabelArray()
	f.labelsToIdentity[lbls.String()] = id

	realID := &identity.Identity{
		ID:     identity.NumericIdentity(id),
		Labels: lbls,
	}
	f.idToIdentity[id] = realID
	f.idRefCount[id]++

	return realID, true, nil
}

// Release releases a fake identity. It is meant to generally mock the
// canonical identity release logic.
func (f *MockIdentityAllocator) Release(_ context.Context, id *identity.Identity, _ bool) (released bool, err error) {
	count, ok := f.idRefCount[int(id.ID)]
	if !ok {
		return false, nil
	}
	if count == 1 {
		delete(f.idRefCount, int(id.ID))
	} else {
		count--
		f.idRefCount[int(id.ID)] = count
		return false, nil
	}
	delete(f.idToIdentity, int(id.ID))
	delete(f.IdentityCache, id.ID)
	return true, nil
}

// ReleaseSlice wraps Release for slices.
func (f *MockIdentityAllocator) ReleaseSlice(ctx context.Context, _ cache.IdentityAllocatorOwner, identities []*identity.Identity) error {
	for _, id := range identities {
		if _, err := f.Release(ctx, id, false); err != nil {
			return err
		}
	}
	return nil
}

// LookupIdentity looks up the labels in the mock identity store.
func (f *MockIdentityAllocator) LookupIdentity(ctx context.Context, lbls labels.Labels) *identity.Identity {
	if reservedIdentity := identity.LookupReservedIdentityByLabels(lbls); reservedIdentity != nil {
		return reservedIdentity
	}
	return f.idToIdentity[f.labelsToIdentity[lbls.String()]]
}

// LookupIdentityByID returns the identity corresponding to the id if the
// identity is a reserved identity. Otherwise, returns nil.
func (f *MockIdentityAllocator) LookupIdentityByID(ctx context.Context, id identity.NumericIdentity) *identity.Identity {
	if identity := identity.LookupReservedIdentity(id); identity != nil {
		return identity
	}
	return f.idToIdentity[int(id)]
}

// AllocateCIDRsForIPs allocates CIDR identities for the given IPs. It is meant
// to generally mock the CIDR identity allocator logic.
func (f *MockIdentityAllocator) AllocateCIDRsForIPs(IPs []net.IP, _ map[string]*identity.Identity) ([]*identity.Identity, error) {
	result := make([]*identity.Identity, 0, len(IPs))
	for _, ip := range IPs {
		id, ok := f.ipToIdentity[ip.String()]
		if !ok {
			id = f.localID
			f.ipToIdentity[ip.String()] = id
			f.idRefCount[id]++
			f.localID = id + 1
		}
		cidrLabels := append([]string{}, ip.String())
		result = append(result, &identity.Identity{
			ID:        identity.NumericIdentity(id),
			CIDRLabel: labels.NewLabelsFromModel(cidrLabels),
		})
	}
	return result, nil
}

func (f *MockIdentityAllocator) ReleaseCIDRIdentitiesByID(ctx context.Context, identities []identity.NumericIdentity) {
}

// GetIdentityCache returns the identity cache.
func (f *MockIdentityAllocator) GetIdentityCache() cache.IdentityCache {
	return f.IdentityCache
}
