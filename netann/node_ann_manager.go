package netann

import (
	"fmt"
	"sync"
	"time"

	"github.com/lightningnetwork/lnd/fn/v2"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/keychain"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
)

// NodeAnnManager owns the node's current signed announcements (v1 and v2)
// and knows how to refresh them.
type NodeAnnManager struct {
	signer keychain.MessageSignerRing
	keyLoc keychain.KeyLocator

	mu   sync.Mutex
	ann1 *lnwire.NodeAnnouncement1
	ann2 *lnwire.NodeAnnouncement2 // nil until initialized
}

// NewNodeAnnManager creates a new NodeAnnManager with the given signer and key
// locator.
func NewNodeAnnManager(signer keychain.MessageSignerRing,
	keyLoc keychain.KeyLocator) *NodeAnnManager {

	return &NodeAnnManager{
		signer: signer,
		keyLoc: keyLoc,
	}
}

// Init sets the initial announcements. It is called once from
// server.initSelfNode after both v1 and v2 are signed.
func (m *NodeAnnManager) Init(ann1 *lnwire.NodeAnnouncement1,
	ann2 *lnwire.NodeAnnouncement2) {

	m.mu.Lock()
	defer m.mu.Unlock()

	m.ann1 = ann1
	m.ann2 = ann2
}

// GetAll returns the current announcements as a slice. The v1 announcement is
// always first; the v2 announcement is appended if it has been initialized.
func (m *NodeAnnManager) GetAll() []lnwire.NodeAnnouncement {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.ann1 == nil {
		return nil
	}

	anns := []lnwire.NodeAnnouncement{m.ann1}
	if m.ann2 != nil {
		anns = append(anns, m.ann2)
	}

	return anns
}

// GetV1 returns the current v1 announcement.
func (m *NodeAnnManager) GetV1() lnwire.NodeAnnouncement {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.ann1
}

// RefreshV1 generates a fresh v1 announcement with an updated timestamp,
// applying the optional modifiers. It updates the cached ann1 and returns it.
func (m *NodeAnnManager) RefreshV1(features *lnwire.RawFeatureVector,
	modifiers ...NodeAnnModifier) (lnwire.NodeAnnouncement, error) {

	m.mu.Lock()
	defer m.mu.Unlock()

	return m.refreshV1Locked(features, modifiers...)
}

// RefreshAll refreshes the v1 announcement and, if v2 is initialized,
// re-derives and re-signs a fresh v2 from the updated v1 fields. The returned
// slice always has v1 first; v2 is appended when available.
func (m *NodeAnnManager) RefreshAll(blockHeight uint32,
	features *lnwire.RawFeatureVector,
	modifiers ...NodeAnnModifier) ([]lnwire.NodeAnnouncement, error) {

	m.mu.Lock()
	defer m.mu.Unlock()

	ann1, err := m.refreshV1Locked(features, modifiers...)
	if err != nil {
		return nil, err
	}

	anns := []lnwire.NodeAnnouncement{ann1}

	ann2, err := m.refreshV2FromV1(blockHeight)
	if err != nil {
		return nil, err
	}

	return append(anns, ann2), nil
}

// refreshV1Locked generates a new v1 announcement, updating the cache. The
// caller must hold m.mu.
func (m *NodeAnnManager) refreshV1Locked(features *lnwire.RawFeatureVector,
	modifiers ...NodeAnnModifier) (lnwire.NodeAnnouncement, error) {

	// Work on a shallow copy so the original is unchanged until signing
	// succeeds.
	newAnn1 := *m.ann1

	if features != nil {
		modifiers = append(modifiers, NodeAnnSetFeatures(features))
	}

	// Always bump the timestamp so the update propagates.
	modifiers = append(modifiers, NodeAnnSetTimestamp(time.Now()))

	if err := SignNodeAnnouncement(
		m.signer, m.keyLoc, &newAnn1, modifiers...,
	); err != nil {
		return nil, err
	}

	m.ann1 = &newAnn1

	return m.ann1, nil
}

// refreshV2FromV1 re-derives a NodeAnnouncement2 from the current ann1 fields,
// signs it, and updates the cache. The caller must hold m.mu.
func (m *NodeAnnManager) refreshV2FromV1(blockHeight uint32) (*lnwire.NodeAnnouncement2, error) {
	// Ensure block height is monotonically increasing if we already have
	// a cached v2 announcement.
	if m.ann2 != nil && blockHeight <= m.ann2.BlockHeight.Val {
		blockHeight = m.ann2.BlockHeight.Val + 1
	}

	pubKey := route.Vertex(m.ann1.NodeID)
	v2Node := models.NewV2Node(pubKey, &models.NodeV2Fields{
		LastBlockHeight: blockHeight,
		Addresses:       m.ann1.Addresses,
		Color:           fn.Some(m.ann1.RGBColor),
		Alias:           fn.Some(m.ann1.Alias.String()),
		Features:        m.ann1.Features,
	})

	ann2Unsigned, err := v2Node.NodeAnnouncement(false)
	if err != nil {
		return nil, err
	}

	if err := SignNodeAnnouncement(m.signer, m.keyLoc, ann2Unsigned); err != nil {
		return nil, err
	}

	ann2, ok := ann2Unsigned.(*lnwire.NodeAnnouncement2)
	if !ok {
		return nil, fmt.Errorf("expected *lnwire.NodeAnnouncement2, "+
			"got %T", ann2Unsigned)
	}

	m.ann2 = ann2

	return ann2, nil
}
