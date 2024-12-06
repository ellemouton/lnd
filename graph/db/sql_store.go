package graphdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/clock"
	"github.com/lightningnetwork/lnd/fn"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
	"github.com/lightningnetwork/lnd/sqldb"
	"github.com/lightningnetwork/lnd/sqldb/sqlc"
	"github.com/lightningnetwork/lnd/tor"
)

type GossipV2Store interface {
	AddNode(ctx context.Context, node *models.Node2) error
	GetNode(ctx context.Context, pubKey route.Vertex) (*models.Node2, error)
	HasNode(ctx context.Context, pubKey route.Vertex) (uint32, bool, error)
	LookupAlias(ctx context.Context, pubKey route.Vertex) (string, error)
	DeleteNode(ctx context.Context, pubKey route.Vertex) error

	SetSourceNode(ctx context.Context, node *models.Node2) error
	GetSourceNode(ctx context.Context) (*models.Node2, error)

	AddChannel(ctx context.Context, edge *models.Channel2) error
	GetChannelByChanID(ctx context.Context, chanID uint64) (*models.Channel2, *models.ChannelPolicy2, *models.ChannelPolicy2, error)
	GetChannelByOutpoint(ctx context.Context, outpoint wire.OutPoint) (*models.Channel2, *models.ChannelPolicy2, *models.ChannelPolicy2, error)
	DeleteChannels(ctx context.Context, chanID ...uint64) error

	UpdateChannelPolicy(ctx context.Context, policy *models.ChannelPolicy2) error
}

// SQLQueries is a subset of the sqlc.Querier interface containing all the
// graph related queries.
type SQLQueries interface {
	InsertNode(ctx context.Context, arg sqlc.InsertNodeParams) (int64, error)
	GetNode(ctx context.Context, id int64) (sqlc.Node, error)
	GetNodeByPubKey(ctx context.Context, pubKey []byte) (sqlc.Node, error)
	GetNodeIDByPubKey(ctx context.Context, pubKey []byte) (int64, error)
	UpdateNode(ctx context.Context, arg sqlc.UpdateNodeParams) error
	DeleteNode(ctx context.Context, id int64) error
	GetNodePubKeyByDBID(ctx context.Context, id int64) ([]byte, error)
	GetNodeAliasByPubKey(ctx context.Context, pubKey []byte) (sql.NullString, error)

	InsertNodeFeature(ctx context.Context, arg sqlc.InsertNodeFeatureParams) error
	GetNodeFeatures(ctx context.Context, nodeID int64) ([]sqlc.NodeFeature, error)
	DeleteNodeFeature(ctx context.Context, arg sqlc.DeleteNodeFeatureParams) error

	GetNodeAddresses(ctx context.Context, nodeID int64) ([]sqlc.NodeAddress, error)
	InsertIPV4NodeAddress(ctx context.Context, arg sqlc.InsertIPV4NodeAddressParams) error
	InsertIPV6NodeAddress(ctx context.Context, arg sqlc.InsertIPV6NodeAddressParams) error
	InsertTorV3NodeAddress(ctx context.Context, arg sqlc.InsertTorV3NodeAddressParams) error
	DeleteNodeAddress(ctx context.Context, arg sqlc.DeleteNodeAddressParams) error

	GetSourceNode(ctx context.Context) (int64, error)
	SetSourceNode(ctx context.Context, nodeID int64) error

	InsertChannel(ctx context.Context, arg sqlc.InsertChannelParams) (int64, error)
	UpdateChannel(ctx context.Context, arg sqlc.UpdateChannelParams) error
	InsertSourceChannel(ctx context.Context, arg sqlc.InsertSourceChannelParams) error
	SetSourceChannelAnnounced(ctx context.Context, arg sqlc.SetSourceChannelAnnouncedParams) (sql.Result, error)
	GetChannel(ctx context.Context, id int64) (sqlc.GetChannelRow, error)
	GetChanDBIDByChanID(ctx context.Context, channelID int64) (int64, error)
	GetChanDBIDByOutpoint(ctx context.Context, outpoint string) (int64, error)
	GetChannelFeatures(ctx context.Context, channelID int64) ([]sqlc.ChannelFeature, error)
	DeleteChannelByChanID(ctx context.Context, channelID int64) error
	InsertChannelFeature(ctx context.Context, arg sqlc.InsertChannelFeatureParams) error

	InsertChannelUpdate(ctx context.Context, arg sqlc.InsertChannelUpdateParams) (int64, error)
	GetChannelPolicy(ctx context.Context, arg sqlc.GetChannelPolicyParams) (sqlc.ChannelPolicy, error)
	UpdateChannelPolicy(ctx context.Context, arg sqlc.UpdateChannelPolicyParams) error
}

// BatchedSQLQueries is a version of the SQLQueries that's capable of batched
// database operations.
type BatchedSQLQueries interface {
	SQLQueries

	sqldb.BatchedTx[SQLQueries]
}

// SQLStore represents a storage backend.
type SQLStore struct {
	db    BatchedSQLQueries
	clock clock.Clock
}

// NewSQLStore creates a new SQLStore instance given an open BatchedSQLQueries
// storage backend.
func NewSQLStore(db BatchedSQLQueries, clock clock.Clock) *SQLStore {
	return &SQLStore{
		db:    db,
		clock: clock,
	}
}

func (s *SQLStore) AddNode(ctx context.Context, node *models.Node2) error {
	var writeTxOpts SQLQueriesTxOptions
	err := s.db.ExecTx(ctx, &writeTxOpts, func(db SQLQueries) error {
		_, err := upsertNode(ctx, db, node)
		return err
	}, func() {})
	if err != nil {
		return fmt.Errorf("unable to insert node: %w", err)
	}

	return nil
}

func (s *SQLStore) DeleteNode(ctx context.Context, pubKey route.Vertex) error {
	var writeTxOpts SQLQueriesTxOptions
	err := s.db.ExecTx(ctx, &writeTxOpts, func(db SQLQueries) error {
		nodeID, err := db.GetNodeIDByPubKey(ctx, pubKey[:])
		if errors.Is(err, sql.ErrNoRows) {
			return ErrGraphNodeNotFound
		} else if err != nil {
			return fmt.Errorf("unable to fetch node: %w", err)
		}

		return db.DeleteNode(ctx, nodeID)
	}, func() {})
	if err != nil {
		return fmt.Errorf("unable to delete node: %w", err)
	}

	return nil
}

func (s *SQLStore) LookupAlias(ctx context.Context, pubKey route.Vertex) (
	string, error) {

	var (
		readTx = NewSQLQueryReadTx()
		alias  string
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		dbAlias, err := db.GetNodeAliasByPubKey(ctx, pubKey[:])
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNodeAliasNotFound
		} else if err != nil {
			return fmt.Errorf("unable to fetch node: %w", err)
		}

		if !dbAlias.Valid {
			return ErrNodeAliasNotFound
		}

		alias = dbAlias.String

		return nil

	}, func() {})
	if err != nil {
		return "", fmt.Errorf("unable to fetch node: %w", err)
	}

	return alias, nil
}

func (s *SQLStore) HasNode(ctx context.Context, pubKey route.Vertex) (uint32,
	bool, error) {

	var (
		readTx     = NewSQLQueryReadTx()
		exists     bool
		lastUpdate uint32
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		dbNode, err := db.GetNodeByPubKey(ctx, pubKey[:])
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		} else if err != nil {
			return fmt.Errorf("unable to fetch node: %w", err)
		}

		exists = true
		lastUpdate = uint32(dbNode.BlockHeight.Int32)

		return nil

	}, func() {})
	if err != nil {
		return 0, false, fmt.Errorf("unable to fetch node: %w", err)
	}

	return lastUpdate, exists, nil
}

func (s *SQLStore) GetNode(ctx context.Context, pubKey route.Vertex) (
	*models.Node2, error) {

	var (
		readTx = NewSQLQueryReadTx()
		node   *models.Node2
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		var err error
		node, err = fetchNodeByPubKey(ctx, db, pubKey)
		return err
	}, func() {})
	if err != nil {
		return nil, fmt.Errorf("unable to fetch node: %w", err)
	}

	return node, nil
}

func (s *SQLStore) SetSourceNode(ctx context.Context, node *models.Node2) error {
	var writeTxOpts SQLQueriesTxOptions
	err := s.db.ExecTx(ctx, &writeTxOpts, func(db SQLQueries) error {
		id, err := upsertNode(ctx, db, node)
		if err != nil {
			return err
		}

		return db.SetSourceNode(ctx, id)
	}, func() {})
	if err != nil {
		return fmt.Errorf("unable to set source node: %w", err)
	}

	return nil
}

func (s *SQLStore) GetSourceNode(ctx context.Context) (*models.Node2, error) {
	var (
		readTx = NewSQLQueryReadTx()
		node   *models.Node2
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		id, err := db.GetSourceNode(ctx)
		if err != nil {
			return err
		}

		node, err = fetchNodeByID(ctx, db, id)
		return err
	}, func() {})
	if err != nil {
		return nil, fmt.Errorf("unable to fetch node: %w", err)
	}

	return node, nil

}

func (s *SQLStore) AddChannel(ctx context.Context,
	edge *models.Channel2) error {

	var writeTxOpts SQLQueriesTxOptions
	err := s.db.ExecTx(ctx, &writeTxOpts, func(db SQLQueries) error {
		// First, check if this channel already exists.
		dbChanID, err := db.GetChanDBIDByChanID(
			ctx, int64(edge.ChannelID),
		)
		switch {
		// The channel does not yet exist in the DB, so we insert a
		// fresh record.
		case errors.Is(err, sql.ErrNoRows):
			err := insertChannel(ctx, db, edge)
			if err != nil {
				return fmt.Errorf("unable to insert new "+
					"node: %w", err)
			}

			return nil

		case err != nil:
			return fmt.Errorf("unable to fetch channel: %w", err)

		// The channel already exists, so we update the existing channel
		// info.
		default:
			return updateChannel(ctx, db, dbChanID, edge)
		}

	}, func() {})
	if err != nil {
		return fmt.Errorf("unable to add channel: %w", err)
	}

	return nil
}

func (s *SQLStore) GetChannelByChanID(ctx context.Context,
	chanID uint64) (*models.Channel2, *models.ChannelPolicy2,
	*models.ChannelPolicy2, error) {

	var (
		readTx  = NewSQLQueryReadTx()
		channel *models.Channel2
		p1      *models.ChannelPolicy2
		p2      *models.ChannelPolicy2
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		dbChanID, err := db.GetChanDBIDByChanID(ctx, int64(chanID))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrEdgeNotFound
		} else if err != nil {
			return fmt.Errorf("could not fetch channel DB ID "+
				"using channel ID(%d): %w", chanID, err)
		}

		channel, p1, p2, err = fetchChannel(ctx, db, dbChanID)
		if err != nil {
			return fmt.Errorf("could not fetch channel(%d): %w",
				dbChanID, err)
		}

		return nil
	}, func() {})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to fetch channel: %w", err)
	}

	return channel, p1, p2, nil
}

func (s *SQLStore) GetChannelByOutpoint(ctx context.Context,
	outpoint wire.OutPoint) (*models.Channel2, *models.ChannelPolicy2,
	*models.ChannelPolicy2, error) {

	var (
		readTx  = NewSQLQueryReadTx()
		channel *models.Channel2
		p1      *models.ChannelPolicy2
		p2      *models.ChannelPolicy2
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		dbChanID, err := db.GetChanDBIDByOutpoint(
			ctx, outpoint.String(),
		)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrEdgeNotFound
		} else if err != nil {
			return fmt.Errorf("could not fetch channel DB ID "+
				"using outpoint(%s): %w", outpoint, err)
		}

		channel, p1, p2, err = fetchChannel(ctx, db, dbChanID)

		return err
	}, func() {})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to fetch node: %w", err)
	}

	return channel, p1, p2, nil
}

func (s *SQLStore) DeleteChannels(ctx context.Context, chanIDs ...uint64) error {
	var writeTxOpts SQLQueriesTxOptions
	err := s.db.ExecTx(ctx, &writeTxOpts, func(db SQLQueries) error {
		for _, chanID := range chanIDs {
			err := db.DeleteChannelByChanID(ctx, int64(chanID))
			if err != nil {
				return fmt.Errorf("could not delete "+
					"channel %d: %w", chanID, err)
			}
		}

		return nil
	}, func() {})
	if err != nil {
		return fmt.Errorf("unable to delete channel: %w", err)
	}

	return nil
}

func (s *SQLStore) UpdateChannelPolicy(ctx context.Context,
	policy *models.ChannelPolicy2) error {

	var writeTxOpts SQLQueriesTxOptions
	err := s.db.ExecTx(ctx, &writeTxOpts, func(db SQLQueries) error {
		// First, check if a record for this policy already exists.
		dbPolicy, err := db.GetChannelPolicy(
			ctx, sqlc.GetChannelPolicyParams{
				ChannelID:  int64(policy.ChannelID),
				SecondPeer: policy.SecondPeer,
			},
		)
		switch {
		// The policy does not yet exist in the DB, so we insert a
		// fresh record.
		case errors.Is(err, sql.ErrNoRows):
			err := insertChanPolicy(ctx, db, policy)
			if err != nil {
				return fmt.Errorf("unable to insert new "+
					"policy: %w", err)
			}

			return nil

		case err != nil:
			return fmt.Errorf("unable to fetch channel "+
				"policy: %w", err)

		// The policy already exists, so we update the existing policy
		// info.
		default:
			return updateChanPolicy(ctx, db, dbPolicy.ID, policy)
		}
	}, func() {})
	if err != nil {
		return fmt.Errorf("unable to update channel policy: %w", err)
	}

	return nil
}

func updateChanPolicy(ctx context.Context, db SQLQueries, dbID int64,
	policy *models.ChannelPolicy2) error {

	err := db.UpdateChannelPolicy(ctx, sqlc.UpdateChannelPolicyParams{
		ID:                     dbID,
		BlockHeight:            int32(policy.BlockHeight),
		DisableFlags:           int32(policy.Flags),
		Timelock:               int32(policy.TimeLockDelta),
		FeePpm:                 int64(policy.FeeProportionalMillionths),
		BaseFeeMsat:            int64(policy.FeeBaseMSat),
		MaxHtlcMsat:            int64(policy.MaxHTLC),
		MinHtlcMsat:            int64(policy.MinHTLC),
		SerialisedAnnouncement: policy.SerialisedWireAnnouncement,
	})
	if err != nil {
		return fmt.Errorf("unable to update channel policy: %w", err)
	}

	return nil
}

func insertChanPolicy(ctx context.Context, db SQLQueries,
	policy *models.ChannelPolicy2) error {

	// First, check if the channel exists.
	dbChanID, err := db.GetChanDBIDByChanID(ctx, int64(policy.ChannelID))
	if errors.Is(err, sql.ErrNoRows) {
		return ErrEdgeNotFound
	} else if err != nil {
		return fmt.Errorf("unable to fetch channel: %w", err)
	}

	_, err = db.InsertChannelUpdate(ctx, sqlc.InsertChannelUpdateParams{
		ChannelID:              dbChanID,
		BlockHeight:            int32(policy.BlockHeight),
		SecondPeer:             policy.SecondPeer,
		Timelock:               int32(policy.TimeLockDelta),
		DisableFlags:           int32(policy.Flags),
		FeePpm:                 int64(policy.FeeProportionalMillionths),
		BaseFeeMsat:            int64(policy.FeeBaseMSat),
		MaxHtlcMsat:            int64(policy.MaxHTLC),
		MinHtlcMsat:            int64(policy.MinHTLC),
		SerialisedAnnouncement: policy.SerialisedWireAnnouncement,
	})
	if err != nil {
		return fmt.Errorf("unable to insert channel update: %w", err)
	}

	return nil
}

func fetchChannel(ctx context.Context, db SQLQueries, dbChanID int64) (
	*models.Channel2, *models.ChannelPolicy2, *models.ChannelPolicy2,
	error) {

	dbChannel, err := db.GetChannel(ctx, dbChanID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil, ErrEdgeNotFound
	} else if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to fetch channel: %w",
			err)
	}

	op, err := wire.NewOutPointFromString(dbChannel.Outpoint)
	if err != nil {
		return nil, nil, nil, err
	}

	node1Pub, err := db.GetNodePubKeyByDBID(ctx, dbChannel.NodeID1)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to fetch node(%d) "+
			"pub key: %w", dbChannel.NodeID1, err)
	}
	node1Vertex, err := route.NewVertexFromBytes(node1Pub)
	if err != nil {
		return nil, nil, nil, err
	}

	node2Pub, err := db.GetNodePubKeyByDBID(ctx, dbChannel.NodeID2)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to fetch node(%d) "+
			"pub key: %w", dbChannel.NodeID2, err)
	}
	node2Vertex, err := route.NewVertexFromBytes(node2Pub)
	if err != nil {
		return nil, nil, nil, err
	}

	features, err := getChanFeatures(ctx, db, dbChanID)
	if err != nil {
		return nil, nil, nil, err
	}

	c := &models.Channel2{
		ChannelID:                  uint64(dbChannel.ChannelID),
		Outpoint:                   *op,
		Node1Key:                   node1Vertex,
		Node2Key:                   node2Vertex,
		Capacity:                   fn.Option[btcutil.Amount]{},
		Features:                   features,
		Announced:                  dbChannel.Announced,
		SerialisedWireAnnouncement: dbChannel.SerialisedAnnouncement,
	}

	if dbChannel.Capacity.Valid {
		c.Capacity = fn.Some(btcutil.Amount(dbChannel.Capacity.Int64))
	}

	var (
		node1Policy *models.ChannelPolicy2
		node2Policy *models.ChannelPolicy2
	)
	node1DBPolicy, err := db.GetChannelPolicy(
		ctx, sqlc.GetChannelPolicyParams{
			ChannelID:  dbChannel.ChannelID,
			SecondPeer: false,
		},
	)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil, fmt.Errorf("unable to fetch node "+
			"1 policy: %w", err)
	}
	if err == nil {
		node1Policy = &models.ChannelPolicy2{
			ChannelID:                  c.ChannelID,
			BlockHeight:                uint32(node1DBPolicy.BlockHeight),
			TimeLockDelta:              uint16(node1DBPolicy.Timelock),
			MinHTLC:                    lnwire.MilliSatoshi(node1DBPolicy.MinHtlcMsat),
			MaxHTLC:                    lnwire.MilliSatoshi(node1DBPolicy.MaxHtlcMsat),
			FeeBaseMSat:                lnwire.MilliSatoshi(node1DBPolicy.BaseFeeMsat),
			FeeProportionalMillionths:  lnwire.MilliSatoshi(node1DBPolicy.FeePpm),
			SecondPeer:                 node1DBPolicy.SecondPeer,
			ToNode:                     c.Node2Key,
			Flags:                      lnwire.ChanUpdateDisableFlags(node1DBPolicy.DisableFlags),
			SerialisedWireAnnouncement: node1DBPolicy.SerialisedAnnouncement,
		}
	}

	node2DBPolicy, err := db.GetChannelPolicy(
		ctx, sqlc.GetChannelPolicyParams{
			ChannelID:  dbChannel.ChannelID,
			SecondPeer: true,
		},
	)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil, fmt.Errorf("unable to fetch node 2 "+
			"policy: %w", err)
	}
	if err == nil {
		node2Policy = &models.ChannelPolicy2{
			ChannelID:                  c.ChannelID,
			BlockHeight:                uint32(node2DBPolicy.BlockHeight),
			TimeLockDelta:              uint16(node2DBPolicy.Timelock),
			MinHTLC:                    lnwire.MilliSatoshi(node2DBPolicy.MinHtlcMsat),
			MaxHTLC:                    lnwire.MilliSatoshi(node2DBPolicy.MaxHtlcMsat),
			FeeBaseMSat:                lnwire.MilliSatoshi(node2DBPolicy.BaseFeeMsat),
			FeeProportionalMillionths:  lnwire.MilliSatoshi(node2DBPolicy.FeePpm),
			SecondPeer:                 node2DBPolicy.SecondPeer,
			ToNode:                     c.Node1Key,
			Flags:                      lnwire.ChanUpdateDisableFlags(node2DBPolicy.DisableFlags),
			SerialisedWireAnnouncement: node2DBPolicy.SerialisedAnnouncement,
		}
	}

	return c, node1Policy, node2Policy, nil
}

func getChanFeatures(ctx context.Context, db SQLQueries,
	chanDBID int64) (*lnwire.FeatureVector, error) {

	rows, err := db.GetChannelFeatures(ctx, chanDBID)
	if err != nil {
		return nil, fmt.Errorf("unable to get channel(%d) features: %w",
			chanDBID, err)
	}

	features := lnwire.EmptyFeatureVector()
	for _, feature := range rows {
		features.Set(lnwire.FeatureBit(feature.Feature))
	}

	return features, nil
}

func insertChannel(ctx context.Context, db SQLQueries,
	edge *models.Channel2) error {

	// Get the source node so that we can check if the channel is one of
	// our own channels.
	sourceDBNodeID, err := db.GetSourceNode(ctx)
	if err != nil {
		return fmt.Errorf("unable to fetch source node: %w", err)
	}

	// Make sure that at least a "shell" entry for each node is present in
	// the nodes table.
	node1DBID, err := db.GetNodeIDByPubKey(ctx, edge.Node1Key[:])
	if errors.Is(err, sql.ErrNoRows) {
		node1DBID, err = db.InsertNode(ctx, sqlc.InsertNodeParams{
			PubKey: edge.Node1Key[:],
		})
		if err != nil {
			return fmt.Errorf("unable to insert node: %w", err)
		}
	} else if err != nil {
		return err
	}

	node2DBID, err := db.GetNodeIDByPubKey(ctx, edge.Node2Key[:])
	if errors.Is(err, sql.ErrNoRows) {
		node2DBID, err = db.InsertNode(ctx, sqlc.InsertNodeParams{
			PubKey: edge.Node2Key[:],
		})
		if err != nil {
			return fmt.Errorf("unable to insert node: %w", err)
		}
	} else if err != nil {
		return err
	}

	var capacity sql.NullInt64
	edge.Capacity.WhenSome(func(amt btcutil.Amount) {
		capacity = sql.NullInt64{
			Valid: true,
			Int64: int64(amt),
		}
	})

	dbChanID, err := db.InsertChannel(ctx, sqlc.InsertChannelParams{
		ChannelID:              int64(edge.ChannelID),
		Outpoint:               edge.Outpoint.String(),
		NodeID1:                node1DBID,
		NodeID2:                node2DBID,
		Capacity:               capacity,
		SerialisedAnnouncement: edge.SerialisedWireAnnouncement,
	})
	if err != nil {
		return fmt.Errorf("unable to insert channel: %w", err)
	}

	// If this channel belongs to the source node, add an entry in the
	// source_channels table.
	if sourceDBNodeID == node1DBID || sourceDBNodeID == node2DBID {
		err = db.InsertSourceChannel(
			ctx, sqlc.InsertSourceChannelParams{
				ChannelID: dbChanID,
				Announced: edge.Announced,
			},
		)
		if err != nil {
			return fmt.Errorf("unable to insert source "+
				"channel: %w", err)
		}
	}

	if edge.Features != nil {
		for feature := range edge.Features.Features() {
			err := db.InsertChannelFeature(
				ctx, sqlc.InsertChannelFeatureParams{
					ChannelID: dbChanID,
					Feature:   int32(feature),
				})
			if err != nil {
				return fmt.Errorf("unable to insert "+
					"channel(%d) feature(%v): %w", dbChanID,
					feature, err)
			}
		}
	}

	return nil
}

func updateChannel(ctx context.Context, db SQLQueries, dbChanID int64,
	edge *models.Channel2) error {

	// This should only ever add a new proof to the channel (which is part
	// of the serialised announcement) and/or update the "announced" field
	// in the source_channels table. It will only ever be for our own
	// channels that we have a channel in the DB that does not yet have
	// a proof or where announced is switched from false to true.

	// We first update the source_channel table with the updated "announced"
	// value. If the channel is not in the source_channels table, we want
	// to fail the operation.
	result, err := db.SetSourceChannelAnnounced(
		ctx, sqlc.SetSourceChannelAnnouncedParams{
			ChannelID: dbChanID,
			Announced: edge.Announced,
		},
	)
	if err != nil {
		return fmt.Errorf("unable to update source channel: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("unable to update source channel: %w", err)
	}

	if n == 0 {
		return fmt.Errorf("channel updates only allowed for channels " +
			"owned by the source node")
	}

	err = db.UpdateChannel(ctx, sqlc.UpdateChannelParams{
		ID:                     dbChanID,
		SerialisedAnnouncement: edge.SerialisedWireAnnouncement,
	})
	if err != nil {
		return fmt.Errorf("unable to update channel: %w", err)
	}

	return nil
}

func fetchNodeByID(ctx context.Context, db SQLQueries, id int64) (*models.Node2,
	error) {

	dbNode, err := db.GetNode(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrGraphNodeNotFound
	} else if err != nil {
		return nil, fmt.Errorf("unable to fetch node: %w", err)
	}

	return buildNode(ctx, db, &dbNode)
}

func fetchNodeByPubKey(ctx context.Context, db SQLQueries,
	pubKey route.Vertex) (*models.Node2, error) {

	dbNode, err := db.GetNodeByPubKey(ctx, pubKey[:])
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrGraphNodeNotFound
	} else if err != nil {
		return nil, fmt.Errorf("unable to fetch node: %w", err)
	}

	return buildNode(ctx, db, &dbNode)
}

func buildNode(ctx context.Context, db SQLQueries, dbNode *sqlc.Node) (
	*models.Node2, error) {

	var pub [33]byte
	copy(pub[:], dbNode.PubKey)

	node := &models.Node2{
		PubKey:                     pub,
		Alias:                      dbNode.Alias.String,
		BlockHeight:                uint32(dbNode.BlockHeight.Int32),
		SerialisedWireAnnouncement: dbNode.SerialisedAnnouncement,
	}

	// Fetch the node's features.
	var err error
	node.Features, err = getNodeFeatures(ctx, db, dbNode.ID)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch node(%d) "+
			"features: %w", dbNode.ID, err)
	}

	// Fetch the node's addresses.
	node.Addresses, err = getNodeAddresses(ctx, db, dbNode.ID)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch node(%d) "+
			"addresses: %w", dbNode.ID, err)
	}

	return node, nil
}

func upsertNode(ctx context.Context, db SQLQueries, node *models.Node2) (int64,
	error) {

	// First, check if this node already exists.
	dbNode, err := db.GetNodeByPubKey(ctx, node.PubKey[:])
	switch {
	// The node does not yet exist in the DB, so we insert a fresh
	// record.
	case errors.Is(err, sql.ErrNoRows):
		id, err := insertNode(ctx, db, node)
		if err != nil {
			return 0, fmt.Errorf("unable to insert new "+
				"node: %w", err)
		}

		return id, nil

	case err != nil:
		return 0, fmt.Errorf("unable to fetch node: %w", err)

	// The node already exists, so we update the existing node info.
	default:
		return dbNode.ID, updateNode(ctx, db, dbNode.ID, node)
	}
}

func getNodeFeatures(ctx context.Context, db SQLQueries,
	nodeID int64) (*lnwire.FeatureVector, error) {

	rows, err := db.GetNodeFeatures(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("unable to get node(%d) features: %w",
			nodeID, err)
	}

	features := lnwire.EmptyFeatureVector()
	for _, feature := range rows {
		features.Set(lnwire.FeatureBit(feature.Feature))
	}

	return features, nil
}

func getNodeAddresses(ctx context.Context, db SQLQueries, nodeID int64) (
	[]net.Addr, error) {

	addrs, err := db.GetNodeAddresses(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("unable to get node(%d) addresses: %w",
			nodeID, err)
	}

	addresses := make([]net.Addr, 0, len(addrs))
	for _, addr := range addrs {
		switch dbAddressType(addr.AddressType) {
		case addressTypeIPv4, addressTypeIPv6:
			tcp, err := net.ResolveTCPAddr("tcp", addr.Address)
			if err != nil {
				return nil, err
			}
			addresses = append(addresses, tcp)

		case addressTypeTorV3:
			service, portStr, err := net.SplitHostPort(addr.Address)
			if err != nil {
				return nil, fmt.Errorf("unable to split tor "+
					"v3 address: %v", addr.Address)
			}

			port, err := strconv.Atoi(portStr)
			if err != nil {
				return nil, err
			}

			addresses = append(addresses, &tor.OnionAddr{
				OnionService: service,
				Port:         port,
			})

		default:
			return nil, fmt.Errorf("unknown address type: %v",
				addr.AddressType)
		}
	}

	return addresses, nil
}

func insertNode(ctx context.Context, db SQLQueries, node *models.Node2) (int64,
	error) {

	var alias sql.NullString
	if node.Alias != "" {
		alias = sql.NullString{
			Valid:  true,
			String: node.Alias,
		}
	}

	nodeParams := sqlc.InsertNodeParams{
		PubKey: node.PubKey[:],
		BlockHeight: sql.NullInt32{
			Valid: true,
			Int32: int32(node.BlockHeight),
		},
		Alias:                  alias,
		SerialisedAnnouncement: node.SerialisedWireAnnouncement,
	}

	// Insert the node.
	nodeID, err := db.InsertNode(ctx, nodeParams)
	if err != nil {
		return 0, err
	}

	err = upsertNodeFeatures(ctx, db, nodeID, node.Features)
	if err != nil {
		return 0, err
	}

	err = upsertNodeAddresses(ctx, db, nodeID, node.Addresses)
	if err != nil {
		return 0, err
	}

	return nodeID, nil
}

func updateNode(ctx context.Context, db SQLQueries, id int64,
	node *models.Node2) error {

	// Update the record in the "nodes" table.
	var alias sql.NullString
	if node.Alias != "" {
		alias = sql.NullString{
			Valid:  true,
			String: node.Alias,
		}
	}

	nodeParams := sqlc.UpdateNodeParams{
		ID: id,
		BlockHeight: sql.NullInt32{
			Valid: true,
			Int32: int32(node.BlockHeight),
		},
		Alias:                  alias,
		SerialisedAnnouncement: node.SerialisedWireAnnouncement,
	}

	err := db.UpdateNode(ctx, nodeParams)
	if err != nil {
		return err
	}

	err = upsertNodeFeatures(ctx, db, id, node.Features)
	if err != nil {
		return err
	}

	return upsertNodeAddresses(ctx, db, id, node.Addresses)
}

type dbAddressType uint8

const (
	addressTypeIPv4  dbAddressType = 0
	addressTypeIPv6  dbAddressType = 1
	addressTypeTorV3 dbAddressType = 2
)

func upsertNodeAddresses(ctx context.Context, db SQLQueries, nodeID int64,
	addresses []net.Addr) error {

	// Get any existing addresses for the node.
	existingAddresses, err := db.GetNodeAddresses(ctx, nodeID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	// Copy the nodes latest set of addresses.
	newAddresses := map[dbAddressType]map[string]struct{}{
		addressTypeIPv4:  {},
		addressTypeIPv6:  {},
		addressTypeTorV3: {},
	}
	addAddr := func(t dbAddressType, addr net.Addr) {
		newAddresses[t][addr.String()] = struct{}{}
	}

	delAddr := func(t dbAddressType, addr string) bool {
		if _, ok := newAddresses[t][addr]; ok {
			delete(newAddresses[t], addr)
			return true
		}
		return false
	}

	for _, address := range addresses {
		switch addr := address.(type) {
		case *net.TCPAddr:
			switch len(addr.IP) {
			case net.IPv4len:
				addAddr(addressTypeIPv4, addr)
			case net.IPv6len:
				addAddr(addressTypeIPv6, addr)
			default:
				return fmt.Errorf("unhandled IP address: %v",
					addr)
			}

		case *tor.OnionAddr:
			if len(addr.OnionService) != tor.V3Len {
				return fmt.Errorf("invalid length for a tor " +
					"v3 address")
			}
			addAddr(addressTypeTorV3, addr)

		default:
			return fmt.Errorf("unhandled address type: %T", addr)
		}
	}

	// For any current address that already exists in the DB, remove it from
	// the in-memory map. For any existing address that does not exist in
	// the in-memory map, delete it from the database.
	for _, addr := range existingAddresses {
		// The address is still present, so there are no updates to be
		// made. We remove it from the in-memory map to avoid inserting
		// it again later.
		if delAddr(dbAddressType(addr.AddressType), addr.Address) {
			continue
		}

		// The address is no longer present, so we remove it from the
		// database.
		err := db.DeleteNodeAddress(ctx, sqlc.DeleteNodeAddressParams{
			NodeID:  nodeID,
			Address: addr.Address,
		})
		if err != nil {
			return fmt.Errorf("unable to delete node(%d) "+
				"address(%v): %w", nodeID, addr.Address, err)
		}
	}

	// Any remaining entries in newAddresses are new addresses that need to
	// be added to the database for the first time.
	for addrType, addrSet := range newAddresses {
		for addr := range addrSet {
			switch addrType {
			case addressTypeIPv4:
				err := db.InsertIPV4NodeAddress(
					ctx, sqlc.InsertIPV4NodeAddressParams{
						NodeID:  nodeID,
						Address: addr,
					},
				)
				if err != nil {
					return fmt.Errorf("unable to insert "+
						"node(%d) IPv4 address(%v): %w",
						nodeID, addr, err)
				}

			case addressTypeIPv6:
				err := db.InsertIPV6NodeAddress(
					ctx, sqlc.InsertIPV6NodeAddressParams{
						NodeID:  nodeID,
						Address: addr,
					},
				)
				if err != nil {
					return fmt.Errorf("unable to insert "+
						"node(%d) IPv6 address(%v): %w",
						nodeID, addr, err)
				}

			case addressTypeTorV3:
				err := db.InsertTorV3NodeAddress(
					ctx, sqlc.InsertTorV3NodeAddressParams{
						NodeID:  nodeID,
						Address: addr,
					},
				)
				if err != nil {
					return fmt.Errorf("unable to insert "+
						"node(%d) tor V3 "+
						"address(%v): %w", nodeID, addr,
						err)
				}
			}
		}
	}

	return nil
}

func upsertNodeFeatures(ctx context.Context, db SQLQueries, nodeID int64,
	features *lnwire.FeatureVector) error {

	// Get any existing features for the node.
	existingFeatures, err := db.GetNodeFeatures(ctx, nodeID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	// Copy the nodes latest set of feature bits.
	newFeatures := make(map[int32]struct{})
	if features != nil {
		for feature := range features.Features() {
			newFeatures[int32(feature)] = struct{}{}
		}
	}

	// For any current feature that already exists in the DB, remove it from
	// the in-memory map. For any existing feature that does not exist in
	// the in-memory map, delete it from the database.
	for _, feature := range existingFeatures {
		// The feature is still present, so there are no updates to be
		// made.
		if _, ok := newFeatures[feature.Feature]; ok {
			delete(newFeatures, feature.Feature)
			continue
		}

		// The feature is no longer present, so we remove it from the
		// database.
		err := db.DeleteNodeFeature(ctx, sqlc.DeleteNodeFeatureParams{
			NodeID:  nodeID,
			Feature: feature.Feature,
		})
		if err != nil {
			return fmt.Errorf("unable to delete node(%d) "+
				"feature(%v): %w", nodeID, feature.Feature, err)
		}
	}

	// Any remaining entries in newFeatures are new features that need to be
	// added to the database for the first time.
	for feature := range newFeatures {
		err := db.InsertNodeFeature(ctx, sqlc.InsertNodeFeatureParams{
			NodeID:  nodeID,
			Feature: feature,
		})
		if err != nil {
			return fmt.Errorf("unable to insert node(%d) "+
				"feature(%v): %w", nodeID, feature, err)
		}
	}

	return nil
}

// SQLQueriesTxOptions defines the set of db txn options the SQLQueries
// understands.
type SQLQueriesTxOptions struct {
	// readOnly governs if a read only transaction is needed or not.
	readOnly bool
}

// ReadOnly returns true if the transaction should be read only.
//
// NOTE: This implements the TxOptions.
func (a *SQLQueriesTxOptions) ReadOnly() bool {
	return a.readOnly
}

// NewSQLQueryReadTx creates a new read transaction option set.
func NewSQLQueryReadTx() SQLQueriesTxOptions {
	return SQLQueriesTxOptions{
		readOnly: true,
	}
}
