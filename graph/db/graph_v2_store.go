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
	"github.com/lightningnetwork/lnd/fn/v2"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
	"github.com/lightningnetwork/lnd/sqldb"
	"github.com/lightningnetwork/lnd/sqldb/sqlc"
	"github.com/lightningnetwork/lnd/tor"
)

type dbAddressType uint8

const (
	addressTypeIPv4  dbAddressType = 0
	addressTypeIPv6  dbAddressType = 1
	addressTypeTorV3 dbAddressType = 2
)

type GossipV2Store interface {
	AddNode(ctx context.Context, node *models.Node2) error
	GetNode(ctx context.Context, pubKey route.Vertex) (*models.Node2, error)
	LookupNodeAlias(ctx context.Context, pubKey route.Vertex) (string, error)
	HasNode(ctx context.Context, pubKey route.Vertex) (uint32, bool, error)
	DeleteNode(ctx context.Context, pubKey route.Vertex) error
	SetSourceNode(ctx context.Context, node *models.Node2) error
	GetSourceNode(ctx context.Context) (*models.Node2, error)
	IsNodePublic(ctx context.Context, pubKey route.Vertex) (bool, error)
	ListNodeChannels(ctx context.Context, pubKey route.Vertex) ([]*models.Channel2, error)

	AddChannel(ctx context.Context, edge *models.Channel2) error
	UpdateAnnouncedChannel(ctx context.Context, chanID uint64, sig []byte, signedFields map[uint64][]byte) error
	GetChannelByChanID(ctx context.Context, chanID uint64) (*models.Channel2, error)
	GetChannelByOutpoint(ctx context.Context, outpoint wire.OutPoint) (*models.Channel2, error)
	DeleteChannels(ctx context.Context, chanID ...uint64) error
}

// V2Queries is a subset of the sqlc.Querier interface containing all the V2
// graph related queries.
type V2Queries interface {
	InsertNode(ctx context.Context, arg sqlc.InsertNodeParams) (int64, error)
	GetNode(ctx context.Context, id int64) (sqlc.Node, error)
	GetNodeByPubKey(ctx context.Context, pubKey []byte) (sqlc.Node, error)
	GetNodeIDByPubKey(ctx context.Context, pubKey []byte) (int64, error)
	GetNodeAliasByPubKey(ctx context.Context, pubKey []byte) (sql.NullString, error)
	UpdateNode(ctx context.Context, arg sqlc.UpdateNodeParams) error
	DeleteNode(ctx context.Context, id int64) error

	InsertNodeFeature(ctx context.Context, arg sqlc.InsertNodeFeatureParams) error
	GetNodeFeatures(ctx context.Context, nodeID int64) ([]sqlc.NodeFeature, error)
	DeleteNodeFeature(ctx context.Context, arg sqlc.DeleteNodeFeatureParams) error

	InsertNodeAddress(ctx context.Context, arg sqlc.InsertNodeAddressParams) error
	GetNodeAddresses(ctx context.Context, nodeID int64) ([]sqlc.NodeAddress, error)
	DeleteNodeAddress(ctx context.Context, arg sqlc.DeleteNodeAddressParams) error

	UpsertNodeExtraType(ctx context.Context, arg sqlc.UpsertNodeExtraTypeParams) error
	GetExtraNodeTypes(ctx context.Context, nodeID int64) ([]sqlc.NodeExtraType, error)
	DeleteExtraNodeType(ctx context.Context, arg sqlc.DeleteExtraNodeTypeParams) error

	GetSourceNode(ctx context.Context) (int64, error)
	SetSourceNode(ctx context.Context, nodeID int64) error

	InsertChannel(ctx context.Context, arg sqlc.InsertChannelParams) (int64, error)
	GetChannel(ctx context.Context, id int64) (sqlc.Channel, error)
	GetChannelByChanID(ctx context.Context, channelID int64) (sqlc.Channel, error)
	GetChannelByOutpoint(ctx context.Context, outpoint string) (sqlc.Channel, error)
	IsPublicNode(ctx context.Context, nodeID1 int64) (bool, error)
	ListNodeChannels(ctx context.Context, nodeID1 int64) ([]sqlc.Channel, error)
	AddChannelSignature(ctx context.Context, arg sqlc.AddChannelSignatureParams) error
	DeleteChannel(ctx context.Context, channelID int64) error

	InsertChannelFeature(ctx context.Context, arg sqlc.InsertChannelFeatureParams) error
	GetChannelFeatures(ctx context.Context, channelID int64) ([]sqlc.ChannelFeature, error)
	DeleteChannelFeature(ctx context.Context, arg sqlc.DeleteChannelFeatureParams) error

	UpsertChannelExtraType(ctx context.Context, arg sqlc.UpsertChannelExtraTypeParams) error
	GetExtraChannelTypes(ctx context.Context, channelID int64) ([]sqlc.ChannelExtraType, error)
	DeleteExtraChannelType(ctx context.Context, arg sqlc.DeleteExtraChannelTypeParams) error
}

// BatchedV2Queries is a version of the V2Queries that's capable of batched
// database operations.
type BatchedV2Queries interface {
	V2Queries

	sqldb.BatchedTx[V2Queries]
}

// V2QueriesTxOptions defines the set of db txn options the V2Queries
// understands.
type V2QueriesTxOptions struct {
	// readOnly governs if a read only transaction is needed or not.
	readOnly bool
}

// ReadOnly returns true if the transaction should be read only.
//
// NOTE: This implements the TxOptions.
func (a *V2QueriesTxOptions) ReadOnly() bool {
	return a.readOnly
}

// NewV2QueryReadTx creates a new read transaction option set.
func NewV2QueryReadTx() V2QueriesTxOptions {
	return V2QueriesTxOptions{
		readOnly: true,
	}
}

type V2Store struct {
	db    BatchedV2Queries
	clock clock.Clock
}

// NewV2Store creates a new V2Store instance given an open BatchedSQLQueries
// storage backend.
func NewV2Store(db BatchedV2Queries, clock clock.Clock) *V2Store {
	return &V2Store{
		db:    db,
		clock: clock,
	}
}

func (s *V2Store) AddNode(ctx context.Context, node *models.Node2) error {
	var writeTxOpts V2QueriesTxOptions
	err := s.db.ExecTx(ctx, &writeTxOpts, func(db V2Queries) error {
		_, err := s.upsertNode(ctx, db, node)
		return err
	}, func() {})
	if err != nil {
		return fmt.Errorf("unable to insert node: %w", err)
	}

	return nil
}

func (s *V2Store) GetNode(ctx context.Context,
	pubKey route.Vertex) (*models.Node2, error) {

	var (
		readTx = NewV2QueryReadTx()
		node   *models.Node2
	)
	err := s.db.ExecTx(ctx, &readTx, func(db V2Queries) error {
		var err error
		node, err = fetchNodeByPubKey(ctx, db, pubKey)

		return err
	}, func() {})
	if err != nil {
		return nil, fmt.Errorf("unable to fetch node: %w", err)
	}

	return node, nil
}

func (s *V2Store) LookupNodeAlias(ctx context.Context, pubKey route.Vertex) (
	string, error) {

	var (
		readTx = NewV2QueryReadTx()
		alias  string
	)
	err := s.db.ExecTx(ctx, &readTx, func(db V2Queries) error {
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

func (s *V2Store) HasNode(ctx context.Context, pubKey route.Vertex) (uint32,
	bool, error) {

	var (
		readTx     = NewV2QueryReadTx()
		exists     bool
		lastUpdate uint32
	)
	err := s.db.ExecTx(ctx, &readTx, func(db V2Queries) error {
		node, err := db.GetNodeByPubKey(ctx, pubKey[:])
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		} else if err != nil {
			return fmt.Errorf("unable to fetch node: %w", err)
		}

		exists = true
		lastUpdate = uint32(node.BlockHeight)

		return nil

	}, func() {})
	if err != nil {
		return 0, false, fmt.Errorf("unable to fetch node: %w", err)
	}

	return lastUpdate, exists, nil
}

func (s *V2Store) DeleteNode(ctx context.Context, pubKey route.Vertex) error {
	var writeTxOpts V2QueriesTxOptions
	err := s.db.ExecTx(ctx, &writeTxOpts, func(db V2Queries) error {
		id, err := db.GetNodeIDByPubKey(ctx, pubKey[:])
		if errors.Is(err, sql.ErrNoRows) {
			return ErrGraphNodeNotFound
		} else if err != nil {
			return fmt.Errorf("unable to fetch node: %w", err)
		}

		return db.DeleteNode(ctx, id)
	}, func() {})
	if err != nil {
		return fmt.Errorf("unable to delete node: %w", err)
	}

	return nil
}

func (s *V2Store) SetSourceNode(ctx context.Context, node *models.Node2) error {
	var writeTxOpts V2QueriesTxOptions
	err := s.db.ExecTx(ctx, &writeTxOpts, func(db V2Queries) error {
		id, err := s.upsertNode(ctx, db, node)
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

func (s *V2Store) GetSourceNode(ctx context.Context) (*models.Node2, error) {
	var (
		readTx = NewV2QueryReadTx()
		node   *models.Node2
	)
	err := s.db.ExecTx(ctx, &readTx, func(db V2Queries) error {
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

func (s *V2Store) AddChannel(ctx context.Context, edge *models.Channel2) error {
	var writeTxOpts V2QueriesTxOptions
	err := s.db.ExecTx(ctx, &writeTxOpts, func(db V2Queries) error {
		// Make sure that this channel does not already exist in the
		// database.
		_, err := db.GetChannelByChanID(ctx, int64(edge.ChannelID))
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil {
			return ErrEdgeAlreadyExist
		}

		return s.insertChannel(ctx, db, edge)
	}, func() {})
	if err != nil {
		return fmt.Errorf("unable to add channel: %w", err)
	}

	return nil
}

func (s *V2Store) ListNodeChannels(ctx context.Context,
	pubKey route.Vertex) ([]*models.Channel2, error) {

	var (
		readTx = NewV2QueryReadTx()
		edges  []*models.Channel2
	)
	err := s.db.ExecTx(ctx, &readTx, func(db V2Queries) error {
		node, err := db.GetNodeByPubKey(ctx, pubKey[:])
		if errors.Is(err, sql.ErrNoRows) {
			return ErrGraphNodeNotFound
		} else if err != nil {
			return fmt.Errorf("unable to fetch node: %w", err)
		}

		dbEdges, err := db.ListNodeChannels(ctx, node.ID)
		if err != nil {
			return fmt.Errorf("unable to fetch node channels: %w",
				err)
		}

		for _, dbEdge := range dbEdges {
			edge, err := buildChannel(ctx, db, dbEdge)
			if err != nil {
				return fmt.Errorf("unable to fetch "+
					"channel(%d): %w", dbEdge.ChannelID,
					err)
			}

			edges = append(edges, edge)
		}

		return nil
	}, func() {
		edges = nil
	})
	if err != nil {
		return nil, fmt.Errorf("unable to fetch node channels: %w", err)
	}

	return edges, nil
}

func (s *V2Store) UpdateAnnouncedChannel(ctx context.Context, chanID uint64,
	sig []byte, signedFields map[uint64][]byte) error {

	var writeTxOpts V2QueriesTxOptions
	err := s.db.ExecTx(ctx, &writeTxOpts, func(db V2Queries) error {
		// Make sure that this channel does already exist in the
		// database.
		channel, err := db.GetChannelByChanID(ctx, int64(chanID))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrEdgeNotFound
		} else if err != nil {
			return err
		}

		if len(channel.Signature) != 0 {
			return fmt.Errorf("channel(%d) already has an "+
				"announcement signature", chanID)
		}

		err = db.AddChannelSignature(
			ctx, sqlc.AddChannelSignatureParams{
				ID:        channel.ID,
				Signature: sig,
			},
		)
		if err != nil {
			return fmt.Errorf("unable to add channel signature: %w",
				err)
		}

		return upsertChannelExtraSignedFields(
			ctx, db, channel.ID, signedFields,
		)
	}, func() {})
	if err != nil {
		return fmt.Errorf("unable to add channel: %w", err)
	}

	return nil
}

func (s *V2Store) GetChannelByChanID(ctx context.Context, chanID uint64) (
	*models.Channel2, error) {

	var (
		readTx  = NewV2QueryReadTx()
		channel *models.Channel2
	)
	err := s.db.ExecTx(ctx, &readTx, func(db V2Queries) error {
		dbChan, err := db.GetChannelByChanID(ctx, int64(chanID))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrEdgeNotFound
		} else if err != nil {
			return fmt.Errorf("could not fetch channel "+
				"using channel ID(%d): %w", chanID, err)
		}

		channel, err = buildChannel(ctx, db, dbChan)
		if err != nil {
			return fmt.Errorf("could not fetch channel(%d): %w",
				chanID, err)
		}

		return nil
	}, func() {})
	if err != nil {
		return nil, fmt.Errorf("unable to fetch channel: %w", err)
	}

	return channel, nil
}

func (s *V2Store) GetChannelByOutpoint(ctx context.Context,
	outpoint wire.OutPoint) (*models.Channel2, error) {

	var (
		readTx  = NewV2QueryReadTx()
		channel *models.Channel2
	)
	err := s.db.ExecTx(ctx, &readTx, func(db V2Queries) error {
		dbChan, err := db.GetChannelByOutpoint(ctx, outpoint.String())
		if errors.Is(err, sql.ErrNoRows) {
			return ErrEdgeNotFound
		} else if err != nil {
			return fmt.Errorf("could not fetch channel DB ID "+
				"using outpoint(%s): %w", outpoint, err)
		}

		channel, err = buildChannel(ctx, db, dbChan)
		if err != nil {
			return fmt.Errorf("could not fetch channel(%s): %w",
				outpoint, err)
		}

		return err
	}, func() {})
	if err != nil {
		return nil, fmt.Errorf("unable to fetch node: %w", err)
	}

	return channel, nil
}

func maybeCreateShellNode(ctx context.Context, db V2Queries,
	pubKey route.Vertex) (int64, error) {

	nodeID, err := db.GetNodeIDByPubKey(ctx, pubKey[:])
	// The node exists. Return the ID.
	if err == nil {
		return nodeID, nil
	}

	// Some other error occurred, return it.
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}

	// Otherwise, the node does not exist, so we create a shell entry for
	// it.
	return db.InsertNode(ctx, sqlc.InsertNodeParams{
		PubKey: pubKey[:],
	})
}

func (s *V2Store) insertChannel(ctx context.Context, db V2Queries,
	edge *models.Channel2) error {

	// Make sure that at least a "shell" entry for each node is present in
	// the nodes table.
	node1DBID, err := maybeCreateShellNode(ctx, db, edge.Node1Key)
	if err != nil {
		return fmt.Errorf("unable to create shell node: %w", err)
	}

	node2DBID, err := maybeCreateShellNode(ctx, db, edge.Node2Key)
	if err != nil {
		return fmt.Errorf("unable to create shell node: %w", err)
	}

	var signature []byte
	edge.Signature.WhenSome(func(s []byte) {
		signature = s
	})

	dbChanID, err := db.InsertChannel(ctx, sqlc.InsertChannelParams{
		ChannelID: int64(edge.ChannelID),
		Outpoint:  edge.Outpoint.String(),
		NodeID1:   node1DBID,
		NodeID2:   node2DBID,
		Capacity:  int64(edge.Capacity),
		Signature: signature,
		CreatedAt: s.clock.Now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("unable to insert channel: %w", err)
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

	if err := upsertChannelExtraSignedFields(
		ctx, db, dbChanID, edge.ExtraSignedFields,
	); err != nil {
		return fmt.Errorf("unable to insert channel extra "+
			"types: %w", err)
	}

	return nil
}

func (s *V2Store) DeleteChannels(ctx context.Context, chanIDs ...uint64) error {
	var writeTxOpts V2QueriesTxOptions
	err := s.db.ExecTx(ctx, &writeTxOpts, func(db V2Queries) error {
		for _, chanID := range chanIDs {
			err := db.DeleteChannel(ctx, int64(chanID))
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

func (s *V2Store) IsNodePublic(ctx context.Context, pubKey route.Vertex) (bool,
	error) {

	var (
		readTx   = NewV2QueryReadTx()
		isPublic bool
	)
	err := s.db.ExecTx(ctx, &readTx, func(db V2Queries) error {
		nodeID, err := db.GetNodeIDByPubKey(ctx, pubKey[:])
		if err != nil {
			return fmt.Errorf("unable to fetch node ID: %w", err)
		}

		isPublic, err = db.IsPublicNode(ctx, nodeID)
		if err != nil {
			return fmt.Errorf("unable to fetch node public "+
				"status: %w", err)
		}

		return nil
	}, func() {})
	if err != nil {
		return false, err
	}

	return isPublic, nil
}

func fetchNodeByID(ctx context.Context, db V2Queries, id int64) (*models.Node2,
	error) {

	dbNode, err := db.GetNode(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrGraphNodeNotFound
	} else if err != nil {
		return nil, fmt.Errorf("unable to fetch node: %w", err)
	}

	return buildNode(ctx, db, &dbNode)
}

func fetchNodeByPubKey(ctx context.Context, db V2Queries,
	pubKey route.Vertex) (*models.Node2, error) {

	dbNode, err := db.GetNodeByPubKey(ctx, pubKey[:])
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrGraphNodeNotFound
	} else if err != nil {
		return nil, fmt.Errorf("unable to fetch node: %w", err)
	}

	return buildNode(ctx, db, &dbNode)
}

func buildNode(ctx context.Context, db V2Queries, dbNode *sqlc.Node) (
	*models.Node2, error) {

	var pub [33]byte
	copy(pub[:], dbNode.PubKey)

	var alias fn.Option[string]
	if dbNode.Alias.Valid {
		alias = fn.Some(dbNode.Alias.String)
	}

	var sig fn.Option[[]byte]
	if dbNode.Signature != nil {
		sig = fn.Some(dbNode.Signature)
	}

	node := &models.Node2{
		PubKey:      pub,
		Alias:       alias,
		BlockHeight: uint32(dbNode.BlockHeight),
		Signature:   sig,
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

	// Fetch the node's extra signed fields.
	node.ExtraSignedFields, err = getNodeExtraSignedFields(
		ctx, db, dbNode.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch node(%d) "+
			"extra signed fields: %w", dbNode.ID, err)

	}

	return node, nil
}

func (s *V2Store) upsertNode(ctx context.Context, db V2Queries,
	node *models.Node2) (int64, error) {

	// First, check if this node already exists.
	dbNode, err := db.GetNodeByPubKey(ctx, node.PubKey[:])
	switch {
	// The node does not yet exist in the DB, so we insert a fresh
	// record.
	case errors.Is(err, sql.ErrNoRows):
		id, err := s.insertNode(ctx, db, node)
		if err != nil {
			return 0, fmt.Errorf("unable to insert new "+
				"node: %w", err)
		}

		return id, nil

	case err != nil:
		return 0, fmt.Errorf("unable to fetch node: %w", err)

	// The node already exists, so we update the existing node info.
	default:
		return dbNode.ID, s.updateNode(ctx, db, dbNode.ID, node)
	}
}

func (s *V2Store) insertNode(ctx context.Context, db V2Queries,
	node *models.Node2) (int64, error) {

	var alias sql.NullString
	node.Alias.WhenSome(func(s string) {
		alias = sql.NullString{
			Valid:  true,
			String: s,
		}
	})

	var sig []byte
	node.Signature.WhenSome(func(s []byte) {
		sig = s
	})

	nodeParams := sqlc.InsertNodeParams{
		PubKey:      node.PubKey[:],
		BlockHeight: int64(node.BlockHeight),
		Alias:       alias,
		Signature:   sig,
		CreatedAt:   s.clock.Now().UTC(),
		UpdatedAt:   s.clock.Now().UTC(),
	}

	// Insert the node.
	nodeID, err := db.InsertNode(ctx, nodeParams)
	if err != nil {
		return 0, fmt.Errorf("inserting node: %w", err)
	}

	err = upsertNodeFeatures(ctx, db, nodeID, node.Features)
	if err != nil {
		return 0, fmt.Errorf("inserting node features: %w", err)
	}

	err = upsertNodeAddresses(ctx, db, nodeID, node.Addresses)
	if err != nil {
		return 0, fmt.Errorf("inserting node addresses: %w", err)
	}

	err = upsertNodeExtraSignedFields(
		ctx, db, nodeID, node.ExtraSignedFields,
	)
	if err != nil {
		return 0, fmt.Errorf("inserting node extra TLVs: %w", err)
	}

	return nodeID, nil
}

func (s *V2Store) updateNode(ctx context.Context, db V2Queries, id int64,
	node *models.Node2) error {

	var alias sql.NullString
	node.Alias.WhenSome(func(s string) {
		alias = sql.NullString{
			Valid:  true,
			String: s,
		}
	})

	var sig []byte
	node.Signature.WhenSome(func(s []byte) {
		sig = s
	})

	nodeParams := sqlc.UpdateNodeParams{
		ID:          id,
		BlockHeight: int64(node.BlockHeight),
		Alias:       alias,
		Signature:   sig,
		UpdatedAt:   s.clock.Now().UTC(),
	}

	err := db.UpdateNode(ctx, nodeParams)
	if err != nil {
		return err
	}

	err = upsertNodeFeatures(ctx, db, id, node.Features)
	if err != nil {
		return fmt.Errorf("inserting node features: %w", err)
	}

	err = upsertNodeAddresses(ctx, db, id, node.Addresses)
	if err != nil {
		return fmt.Errorf("inserting node addresses: %w", err)
	}

	err = upsertNodeExtraSignedFields(
		ctx, db, id, node.ExtraSignedFields,
	)
	if err != nil {
		return fmt.Errorf("upserting node extra TLVs: %w", err)
	}

	return nil
}
func upsertNodeFeatures(ctx context.Context, db V2Queries, nodeID int64,
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

func getNodeFeatures(ctx context.Context, db V2Queries,
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

func upsertNodeAddresses(ctx context.Context, db V2Queries, nodeID int64,
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
		if delAddr(dbAddressType(addr.Type), addr.Address) {
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
			err := db.InsertNodeAddress(
				ctx, sqlc.InsertNodeAddressParams{
					NodeID:  nodeID,
					Type:    int16(addrType),
					Address: addr,
				},
			)
			if err != nil {
				return fmt.Errorf("unable to insert "+
					"node(%d) address(%v): %w", nodeID,
					addr, err)
			}
		}
	}

	return nil
}

func getNodeAddresses(ctx context.Context, db V2Queries, nodeID int64) (
	[]net.Addr, error) {

	addrs, err := db.GetNodeAddresses(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("unable to get node(%d) addresses: %w",
			nodeID, err)
	}

	addresses := make([]net.Addr, 0, len(addrs))
	for _, addr := range addrs {
		switch dbAddressType(addr.Type) {
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
				addr.Type)
		}
	}

	return addresses, nil
}

func getNodeExtraSignedFields(ctx context.Context, db V2Queries,
	nodeID int64) (map[uint64][]byte, error) {

	fields, err := db.GetExtraNodeTypes(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("unable to get node(%d) extra "+
			"signed fields: %w", nodeID, err)
	}

	extraFields := make(map[uint64][]byte)
	for _, field := range fields {
		extraFields[uint64(field.Type)] = field.Value
	}

	return extraFields, nil
}

func upsertNodeExtraSignedFields(ctx context.Context, db V2Queries,
	nodeID int64, extraFields map[uint64][]byte) error {

	// Get any existing extra signed fields for the node.
	existingFields, err := db.GetExtraNodeTypes(ctx, nodeID)
	if err != nil {
		return err
	}

	// Make a lookup map of the existing field types so that we can use it
	// to keep track of any fields we should delete.
	m := make(map[uint64]bool)
	for _, field := range existingFields {
		m[uint64(field.Type)] = true
	}

	// For all the new fields, we'll upsert them and remove them from the
	// map of existing fields.
	for tlvType, value := range extraFields {
		err = db.UpsertNodeExtraType(
			ctx, sqlc.UpsertNodeExtraTypeParams{
				NodeID: nodeID,
				Type:   int64(tlvType),
				Value:  value,
			},
		)
		if err != nil {
			return fmt.Errorf("unable to upsert node(%d) extra "+
				"signed field(%v): %w", nodeID, tlvType, err)
		}

		// Remove the field from the map of existing fields if it was
		// present.
		delete(m, tlvType)
	}

	// For all the fields that are left in the map of existing fields, we'll
	// delete them as they are no longer present in the new set of fields.
	for tlvType := range m {
		err = db.DeleteExtraNodeType(
			ctx, sqlc.DeleteExtraNodeTypeParams{
				NodeID: nodeID,
				Type:   int64(tlvType),
			},
		)
		if err != nil {
			return fmt.Errorf("unable to delete node(%d) extra "+
				"signed field(%v): %w", nodeID, tlvType, err)
		}
	}

	return nil
}

func upsertChannelExtraSignedFields(ctx context.Context, db V2Queries,
	channelID int64, extraFields map[uint64][]byte) error {

	// Get any existing extra signed fields for the channel.
	existingFields, err := db.GetExtraChannelTypes(ctx, channelID)
	if err != nil {
		return err
	}

	// Make a lookup map of the existing field types so that we can use it
	// to keep track of any fields we should delete.
	m := make(map[uint64]bool)
	for _, field := range existingFields {
		m[uint64(field.Type)] = true
	}

	// For all the new fields, we'll upsert them and remove them from the
	// map of existing fields.
	for tlvType, value := range extraFields {
		err = db.UpsertChannelExtraType(
			ctx, sqlc.UpsertChannelExtraTypeParams{
				ChannelID: channelID,
				Type:      int64(tlvType),
				Value:     value,
			},
		)
		if err != nil {
			return fmt.Errorf("unable to upsert channel(%d) extra "+
				"signed field(%v): %w", channelID, tlvType, err)
		}

		// Remove the field from the map of existing fields if it was
		// present.
		delete(m, tlvType)
	}

	// For all the fields that are left in the map of existing fields, we'll
	// delete them as they are no longer present in the new set of fields.
	for tlvType := range m {
		err = db.DeleteExtraChannelType(
			ctx, sqlc.DeleteExtraChannelTypeParams{
				ChannelID: channelID,
				Type:      int64(tlvType),
			},
		)
		if err != nil {
			return fmt.Errorf("unable to delete channel(%d) extra "+
				"signed field(%v): %w", channelID, tlvType, err)
		}
	}

	return nil
}

func buildChannel(ctx context.Context, db V2Queries,
	dbChan sqlc.Channel) (*models.Channel2, error) {

	op, err := wire.NewOutPointFromString(dbChan.Outpoint)
	if err != nil {
		return nil, err
	}

	node1, err := db.GetNode(ctx, dbChan.NodeID1)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch node(%d) pub key: %w",
			dbChan.NodeID1, err)
	}
	node1Vertex, err := route.NewVertexFromBytes(node1.PubKey)
	if err != nil {
		return nil, err
	}

	node2, err := db.GetNode(ctx, dbChan.NodeID2)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch node(%d) pub key: %w",
			dbChan.NodeID2, err)
	}
	node2Vertex, err := route.NewVertexFromBytes(node2.PubKey)
	if err != nil {
		return nil, err
	}

	features, err := getChanFeatures(ctx, db, dbChan.ID)
	if err != nil {
		return nil, err
	}

	extraTypes, err := getChannelExtraSignedFields(ctx, db, dbChan.ID)
	if err != nil {
		return nil, err
	}

	var sig fn.Option[[]byte]
	if dbChan.Signature != nil {
		sig = fn.Some(dbChan.Signature)
	}

	return &models.Channel2{
		ChannelID:         uint64(dbChan.ChannelID),
		Outpoint:          *op,
		Node1Key:          node1Vertex,
		Node2Key:          node2Vertex,
		Capacity:          btcutil.Amount(dbChan.Capacity),
		Features:          features,
		ExtraSignedFields: extraTypes,
		Signature:         sig,
	}, nil
}

func getChanFeatures(ctx context.Context, db V2Queries,
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

func getChannelExtraSignedFields(ctx context.Context, db V2Queries,
	nodeID int64) (map[uint64][]byte, error) {

	fields, err := db.GetExtraChannelTypes(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("unable to get channel(%d) extra "+
			"signed fields: %w", nodeID, err)
	}

	extraFields := make(map[uint64][]byte)
	for _, field := range fields {
		extraFields[uint64(field.Type)] = field.Value
	}

	return extraFields, nil
}
