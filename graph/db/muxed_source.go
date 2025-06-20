package graphdb

import (
	"bytes"
	"errors"
	"sync"

	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
)

type muxedSource struct {
	local  GraphSource
	remote GraphSource

	// getLocalSource can be used to fetch the local nodes pub key.
	getLocalSource func() (route.Vertex, error)

	// srcPub is a cached version of the local nodes own pub key bytes. The
	// mu mutex should be used when accessing this field.
	srcPub *route.Vertex
	mu     sync.Mutex
}

var _ GraphSource = (*muxedSource)(nil)

func NewMuxedSource(local, remote GraphSource,
	getLocalSource func() (route.Vertex, error)) GraphSource {

	return &muxedSource{
		remote:         remote,
		local:          local,
		getLocalSource: getLocalSource,
	}
}

func (m *muxedSource) ForEachChannelCacheable(cb func(*models.CachedEdgeInfo,
	*models.CachedEdgePolicy, *models.CachedEdgePolicy) error) error {

	srcPub, err := m.selfNodePub()
	if err != nil {
		return err
	}

	ourChans := make(map[uint64]bool)
	err = m.local.ForEachNodeChannel(
		srcPub, func(info *models.ChannelEdgeInfo,
			policy, policy2 *models.ChannelEdgePolicy) error {

			ourChans[info.ChannelID] = true

			var p1, p2 *models.CachedEdgePolicy
			if policy != nil {
				p1 = models.NewCachedPolicy(policy)
			}
			if policy2 != nil {
				p2 = models.NewCachedPolicy(policy2)
			}

			return cb(models.NewCachedEdge(info), p1, p2)
		},
	)
	if err != nil {
		return err
	}

	return m.remote.ForEachChannelCacheable(
		func(info *models.CachedEdgeInfo,
			policy *models.CachedEdgePolicy,
			policy2 *models.CachedEdgePolicy) error {

			if ourChans[info.ChannelID] {
				return nil
			}

			return cb(info, policy, policy2)
		},
	)
}

func (m *muxedSource) ForEachNodeCacheable(cb func(route.Vertex,
	*lnwire.FeatureVector) error) error {

	srcPub, err := m.selfNodePub()
	if err != nil {
		return err
	}

	handled := make(map[route.Vertex]struct{})
	err = m.remote.ForEachNodeCacheable(func(node route.Vertex,
		features *lnwire.FeatureVector) error {

		// We leave the handling of the local node to the local source.
		if bytes.Equal(srcPub[:], node[:]) {
			return nil
		}

		handled[node] = struct{}{}

		return cb(node, features)
	})
	if err != nil {
		return err
	}

	return m.local.ForEachNodeCacheable(func(node route.Vertex,
		features *lnwire.FeatureVector) error {

		if _, ok := handled[node]; ok {
			return nil
		}

		return cb(node, features)
	})
}

func (m *muxedSource) ForEachChannel(cb func(*models.ChannelEdgeInfo,
	*models.ChannelEdgePolicy, *models.ChannelEdgePolicy) error) error {

	srcPub, err := m.selfNodePub()
	if err != nil {
		return err
	}

	ourChans := make(map[uint64]bool)
	err = m.local.ForEachNodeChannel(
		srcPub, func(info *models.ChannelEdgeInfo,
			policy, policy2 *models.ChannelEdgePolicy) error {

			ourChans[info.ChannelID] = true

			return cb(info, policy, policy2)
		},
	)
	if err != nil {
		return err
	}

	return m.remote.ForEachChannel(func(info *models.ChannelEdgeInfo,
		policy *models.ChannelEdgePolicy,
		policy2 *models.ChannelEdgePolicy) error {

		if ourChans[info.ChannelID] {
			return nil
		}

		return cb(info, policy, policy2)
	})
}

func (m *muxedSource) ForEachNodeChannel(nodePub route.Vertex,
	cb func(*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy) error) error {

	srcPub, err := m.selfNodePub()
	if err != nil {
		return err
	}

	if bytes.Equal(srcPub[:], nodePub[:]) {
		return m.local.ForEachNodeChannel(nodePub, cb)
	}

	// First query our own db since we may have chan info that our remote
	// does not know of (regarding our selves or our channel peers).
	handledChans := make(map[uint64]bool)
	err = m.local.ForEachNodeChannel(nodePub, func(
		info *models.ChannelEdgeInfo, policy *models.ChannelEdgePolicy,
		policy2 *models.ChannelEdgePolicy) error {

		handledChans[info.ChannelID] = true

		return cb(info, policy, policy2)
	})
	if err != nil {
		return err
	}

	return m.remote.ForEachNodeChannel(
		nodePub, func(info *models.ChannelEdgeInfo,
			p1 *models.ChannelEdgePolicy,
			p2 *models.ChannelEdgePolicy) error {

			if handledChans[info.ChannelID] {
				return nil
			}

			return cb(info, p1, p2)
		},
	)
}

func (m *muxedSource) ForEachNode(cb func(tx NodeRTx) error) error {
	srcPub, err := m.selfNodePub()
	if err != nil {
		return err
	}

	handled := make(map[route.Vertex]struct{})
	err = m.remote.ForEachNode(func(tx NodeRTx) error {
		// We leave the handling of the local node to the local source.
		if bytes.Equal(srcPub[:], tx.Node().PubKeyBytes[:]) {
			return nil
		}

		handled[tx.Node().PubKeyBytes] = struct{}{}

		return cb(tx)
	})
	if err != nil {
		return err
	}

	return m.local.ForEachNode(func(tx NodeRTx) error {
		if _, ok := handled[tx.Node().PubKeyBytes]; ok {
			return nil
		}

		return cb(tx)
	})
}

func (m *muxedSource) FetchChannelEdgesByID(chanID uint64) (
	*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
	*models.ChannelEdgePolicy, error) {

	srcPub, err := m.selfNodePub()
	if err != nil {
		return nil, nil, nil, err
	}

	// We query the local source first in case this channel belongs to the
	// local node.
	localInfo, localP1, localP2, localErr := m.local.FetchChannelEdgesByID(
		chanID,
	)
	if localErr != nil && !errors.Is(localErr, ErrEdgeNotFound) {
		return nil, nil, nil, localErr
	}

	// If the local node is one of the channel owners, then it will have
	// up-to-date information regarding this channel.
	if !errors.Is(localErr, ErrEdgeNotFound) &&
		(bytes.Equal(localInfo.NodeKey1Bytes[:], srcPub[:]) ||
			bytes.Equal(localInfo.NodeKey2Bytes[:], srcPub[:])) {

		return localInfo, localP1, localP2, nil
	}

	// Otherwise, we query the remote source since it is the most likely to
	// have the most up-to-date information.
	remoteInfo, remoteP1, remoteP2, err := m.remote.FetchChannelEdgesByID(
		chanID,
	)
	if err == nil || !errors.Is(err, ErrEdgeNotFound) {
		return remoteInfo, remoteP1, remoteP2, err
	}

	// If the remote source does not have the channel, we return the local
	// information if it was found.
	return localInfo, localP1, localP2, localErr
}

func (m *muxedSource) FetchChannelEdgesByOutpoint(point *wire.OutPoint) (
	*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
	*models.ChannelEdgePolicy, error) {

	srcPub, err := m.selfNodePub()
	if err != nil {
		return nil, nil, nil, err
	}

	// We query the local source first in case this channel belongs to the
	// local node.
	localInfo, localP1, localP2, localErr := m.local.FetchChannelEdgesByOutpoint( //nolint:ll
		point,
	)
	if localErr != nil && !errors.Is(localErr, ErrEdgeNotFound) {
		return nil, nil, nil, localErr
	}

	// If the local node is one of the channel owners, then it will have
	// up-to-date information regarding this channel.
	if !errors.Is(localErr, ErrEdgeNotFound) &&
		(bytes.Equal(localInfo.NodeKey1Bytes[:], srcPub[:]) ||
			bytes.Equal(localInfo.NodeKey2Bytes[:], srcPub[:])) {

		return localInfo, localP1, localP2, nil
	}

	// Otherwise, we query the remote source since it is the most likely to
	// have the most up-to-date information.
	remoteInfo, remoteP1, remoteP2, err := m.remote.FetchChannelEdgesByOutpoint( //nolint:ll
		point,
	)
	if err == nil || !errors.Is(err, ErrEdgeNotFound) {
		return remoteInfo, remoteP1, remoteP2, err
	}

	// If the remote source does not have the channel, we return the local
	// information if it was found.
	return localInfo, localP1, localP2, localErr
}

// selfNodePub fetches the local nodes pub key. It first checks the cached value
// and if non exists, it queries the database.
func (m *muxedSource) selfNodePub() (route.Vertex, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.srcPub != nil {
		return *m.srcPub, nil
	}

	pub, err := m.getLocalSource()
	if errors.Is(err, ErrSourceNodeNotSet) { // on first init, it wont be set yet.
		return route.Vertex{}, nil
	} else if err != nil {
		return route.Vertex{}, err
	}

	m.srcPub = &pub

	return *m.srcPub, nil
}
