package graphdb

import (
	"bytes"
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/channeldb"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
)

type muxedSource struct {
	remote Source
	local  Source

	// getLocalSource can be used to fetch the local nodes pub key.
	getLocalSource func() (route.Vertex, error)

	// srcPub is a cached version of the local nodes own pub key bytes. The
	// mu mutex should be used when accessing this field.
	srcPub *route.Vertex
	mu     sync.Mutex
}

func NewMuxedSource(remote, local Source, getLocalSource func() (route.Vertex, error)) Source {
	return &muxedSource{
		remote:         remote,
		local:          local,
		getLocalSource: getLocalSource,
	}
}

func (m *muxedSource) AddrsForNode(nodePub *btcec.PublicKey) (bool, []net.Addr, error) {
	// Check both the local and remote sources and merge the results.
	return channeldb.NewMultiAddrSource(
		m.local, m.remote,
	).AddrsForNode(nodePub)
}

func (m *muxedSource) ForEachChannel(cb func(*models.ChannelEdgeInfo, *models.ChannelEdgePolicy, *models.ChannelEdgePolicy) error) error {
	srcPub, err := m.selfNodePub()
	if err != nil {
		return err
	}

	ourChans := make(map[uint64]bool)
	err = m.local.ForEachNodeChannel(context.TODO(), srcPub, func(
		info *models.ChannelEdgeInfo, policy *models.ChannelEdgePolicy,
		policy2 *models.ChannelEdgePolicy) error {

		ourChans[info.ChannelID] = true

		return cb(info, policy, policy2)
	})
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

func (m *muxedSource) FetchChannelEdgesByOutpoint(point *wire.OutPoint) (*models.ChannelEdgeInfo, *models.ChannelEdgePolicy, *models.ChannelEdgePolicy, error) {
	srcPub, err := m.selfNodePub()
	if err != nil {
		return nil, nil, nil, err
	}

	// We query the local source first in case this channel belongs to the
	// local node.
	localInfo, localP1, localP2, localErr := m.local.FetchChannelEdgesByOutpoint( //nolint:lll
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
	remoteInfo, remoteP1, remoteP2, err := m.remote.FetchChannelEdgesByOutpoint( //nolint:lll
		point,
	)
	if err == nil || !errors.Is(err, ErrEdgeNotFound) {
		return remoteInfo, remoteP1, remoteP2, err
	}

	// If the remote source does not have the channel, we return the local
	// information if it was found.
	return localInfo, localP1, localP2, localErr
}

func (g *muxedSource) HasLightningNode(nodePub [33]byte) (time.Time, bool, error) {
	srcPub, err := g.selfNodePub()
	if err != nil {
		return time.Time{}, false, err
	}

	// If this is the local node, then it will have the most up-to-date
	// info and so there is no reason to query the remote node.
	if bytes.Equal(srcPub[:], nodePub[:]) {
		return g.local.HasLightningNode(nodePub)
	}

	// Else, we query the remote node first since it is the most likely to
	// have the most up-to-date timestamp.
	timeStamp, remoteHas, err := g.remote.HasLightningNode(nodePub)
	if err != nil {
		return timeStamp, false, err
	}
	if remoteHas {
		return timeStamp, true, nil
	}

	// Fall back to querying the local node.
	return g.local.HasLightningNode(nodePub)
}

func (g muxedSource) FetchLightningNode(nodePub route.Vertex) (*models.LightningNode, error) {
	srcPub, err := g.selfNodePub()
	if err != nil {
		return nil, err
	}

	// Special case if the node in question is the local node.
	if bytes.Equal(srcPub[:], nodePub[:]) {
		return g.local.FetchLightningNode(nodePub)
	}

	node, err := g.remote.FetchLightningNode(nodePub)
	if err == nil || !errors.Is(err, ErrGraphNodeNotFound) {
		return node, err
	}

	return g.local.FetchLightningNode(nodePub)
}

func (g *muxedSource) ForEachNode(cb func(*models.LightningNode) error) error {
	srcPub, err := g.selfNodePub()
	if err != nil {
		return err
	}

	handled := make(map[route.Vertex]struct{})
	err = g.remote.ForEachNode(func(node *models.LightningNode) error {
		// We leave the handling of the local node to the local source.
		if bytes.Equal(srcPub[:], node.PubKeyBytes[:]) {
			return nil
		}

		handled[node.PubKeyBytes] = struct{}{}

		return cb(node)
	})
	if err != nil {
		return err
	}

	return g.local.ForEachNode(
		func(node *models.LightningNode) error {
			if _, ok := handled[node.PubKeyBytes]; ok {
				return nil
			}

			return cb(node)
		},
	)
}

func (g *muxedSource) ForEachNodeChannel(ctx context.Context, nodePub route.Vertex, cb func(*models.ChannelEdgeInfo, *models.ChannelEdgePolicy, *models.ChannelEdgePolicy) error) error {
	srcPub, err := g.selfNodePub()
	if err != nil {
		return err
	}

	if bytes.Equal(srcPub[:], nodePub[:]) {
		return g.local.ForEachNodeChannel(ctx, nodePub, cb)
	}

	// First query our own db since we may have chan info that our remote
	// does not know of (regarding our selves or our channel peers).
	handledChans := make(map[uint64]bool)
	err = g.local.ForEachNodeChannel(ctx, nodePub, func(
		info *models.ChannelEdgeInfo, policy *models.ChannelEdgePolicy,
		policy2 *models.ChannelEdgePolicy) error {

		handledChans[info.ChannelID] = true

		return cb(info, policy, policy2)
	})
	if err != nil {
		return err
	}

	return g.remote.ForEachNodeChannel(
		ctx, nodePub, func(info *models.ChannelEdgeInfo,
			p1 *models.ChannelEdgePolicy,
			p2 *models.ChannelEdgePolicy) error {

			if handledChans[info.ChannelID] {
				return nil
			}

			return cb(info, p1, p2)
		},
	)
}

func (m muxedSource) IsPublicNode(ctx context.Context, pubKey [33]byte) (bool, error) {
	isPublic, err := m.local.IsPublicNode(ctx, pubKey)
	if err == nil && isPublic {
		return isPublic, err
	}
	if err != nil && !errors.Is(err, ErrGraphNodeNotFound) {
		return false, err
	}

	return m.remote.IsPublicNode(ctx, pubKey)
}

func (g muxedSource) FetchChannelEdgesByID(ctx context.Context, chanID uint64) (*models.ChannelEdgeInfo, *models.ChannelEdgePolicy, *models.ChannelEdgePolicy, error) {
	srcPub, err := g.selfNodePub()
	if err != nil {
		return nil, nil, nil, err
	}

	// We query the local source first in case this channel belongs to the
	// local node.
	localInfo, localP1, localP2, localErr := g.local.FetchChannelEdgesByID(
		ctx, chanID,
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
	remoteInfo, remoteP1, remoteP2, err := g.remote.FetchChannelEdgesByID(
		ctx, chanID,
	)
	if err == nil || !errors.Is(err, ErrEdgeNotFound) {
		return remoteInfo, remoteP1, remoteP2, err
	}

	// If the remote source does not have the channel, we return the local
	// information if it was found.
	return localInfo, localP1, localP2, localErr
}

func (m *muxedSource) NumZombies() (uint64, error) {
	return m.remote.NumZombies()
}

// FetchNodeFeatures returns the features of a given node. If no features are
// known for the node, an empty feature vector is returned. If the node in
// question is the local node, then only the local source is queried. Otherwise,
// the remote is queried first and only if it returns an empty feature vector
// is the local source queried.
//
// NOTE: this is part of the GraphSource interface.
func (g *muxedSource) FetchNodeFeatures(ctx context.Context, node route.Vertex) (*lnwire.FeatureVector, error) {
	srcPub, err := g.selfNodePub()
	if err != nil {
		return nil, err
	}

	// If we are the source node, we know our own features, so just use
	// local DB.
	if bytes.Equal(srcPub[:], node[:]) {
		return g.local.FetchNodeFeatures(ctx, node)
	}

	// Otherwise, first query the remote source since it will have the most
	// up-to-date information. If it returns an empty feature vector, then
	// we also consult the local source.
	feats, err := g.remote.FetchNodeFeatures(ctx, node)
	if err != nil {
		return nil, err
	}

	if !feats.IsEmpty() {
		return feats, nil
	}

	return g.local.FetchNodeFeatures(ctx, node)
}

func (m *muxedSource) ForEachNodeCached(cb func(node route.Vertex, chans map[uint64]*models.DirectedChannel) error) error {
	srcPub, err := m.selfNodePub()
	if err != nil {
		return err
	}

	handled := make(map[route.Vertex]struct{})
	err = m.remote.ForEachNodeCached(func(node route.Vertex, chans map[uint64]*models.DirectedChannel) error {
		// We leave the handling of the local node to the local source.
		if bytes.Equal(srcPub[:], node[:]) {
			return nil
		}

		handled[node] = struct{}{}

		return cb(node, chans)
	})
	if err != nil {
		return err
	}

	return m.local.ForEachNodeCached(
		func(node route.Vertex, chans map[uint64]*models.DirectedChannel) error {
			if _, ok := handled[node]; ok {
				return nil
			}

			return cb(node, chans)
		},
	)
}

func (m *muxedSource) NewRoutingGraphSession() (RoutingGraph, func() error, error) {
	local, cleanup, err := m.local.NewRoutingGraphSession()
	if err != nil {
		return nil, nil, err
	}

	remote := m.remote.NewRoutingGraph()

	return &muxedSession{
		m:             m,
		localSession:  local,
		remoteSession: remote,
	}, cleanup, nil
}

func (m *muxedSource) NewRoutingGraph() RoutingGraph {
	return &muxedSession{
		m:             m,
		localSession:  m.local.NewRoutingGraph(),
		remoteSession: m.remote.NewRoutingGraph(),
	}
}

func (m *muxedSource) ForEachNodeWithTx(ctx context.Context, cb func(NodeTx) error) error {
	panic("implement me")
}

type muxNodeTx struct {
	m          *muxedSource
	localNode  NodeTx
	remoteNode NodeTx
}

type muxedSession struct {
	m             *muxedSource
	localSession  RoutingGraph
	remoteSession RoutingGraph
}

func (g *muxedSession) ForEachNodeChannel(ctx context.Context, nodePub route.Vertex, cb func(channel *models.DirectedChannel) error) error {
	srcPub, err := g.m.selfNodePub()
	if err != nil {
		return err
	}

	if bytes.Equal(srcPub[:], nodePub[:]) {
		return g.localSession.ForEachNodeChannel(ctx, nodePub, cb)
	}

	// First query our own db since we may have chan info that our remote
	// does not know of (regarding our selves or our channel peers).
	handledChans := make(map[uint64]bool)
	err = g.localSession.ForEachNodeChannel(ctx, nodePub,
		func(channel *models.DirectedChannel) error {
			handledChans[channel.ChannelID] = true

			return cb(channel)
		})
	if err != nil {
		return err
	}

	return g.remoteSession.ForEachNodeChannel(
		ctx, nodePub, func(channel *models.DirectedChannel) error {
			if handledChans[channel.ChannelID] {
				return nil
			}

			return cb(channel)
		},
	)
}

func (m *muxedSession) FetchNodeFeatures(ctx context.Context, node route.Vertex) (*lnwire.FeatureVector, error) {
	srcPub, err := m.m.selfNodePub()
	if err != nil {
		return nil, err
	}

	// If we are the source node, we know our own features, so just use
	// local DB.
	if bytes.Equal(srcPub[:], node[:]) {
		return m.localSession.FetchNodeFeatures(ctx, node)
	}

	// Otherwise, first query the remote source since it will have the most
	// up-to-date information. If it returns an empty feature vector, then
	// we also consult the local source.
	feats, err := m.remoteSession.FetchNodeFeatures(ctx, node)
	if err != nil {
		return nil, err
	}

	if !feats.IsEmpty() {
		return feats, nil
	}

	return m.localSession.FetchNodeFeatures(ctx, node)
}

var _ RoutingGraph = (*muxedSession)(nil)

// selfNodePub fetches the local nodes pub key. It first checks the cached value
// and if non exists, it queries the database.
func (m *muxedSource) selfNodePub() (route.Vertex, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.srcPub != nil {
		return *m.srcPub, nil
	}

	pub, err := m.getLocalSource()
	if err != nil {
		return route.Vertex{}, err
	}

	m.srcPub = &pub

	return *m.srcPub, nil
}

var _ Source = (*muxedSource)(nil)
