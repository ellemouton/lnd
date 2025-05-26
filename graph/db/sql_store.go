package graphdb

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/aliasmgr"
	"github.com/lightningnetwork/lnd/batch"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
	"github.com/lightningnetwork/lnd/sqldb"
	"github.com/lightningnetwork/lnd/sqldb/sqlc"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/lightningnetwork/lnd/tor"
)

// ProtocolVersion is an enum that defines the gossip protocol version of a
// message.
type ProtocolVersion uint8

const (
	// ProtocolV1 is the gossip protocol version defined in BOLT #7.
	ProtocolV1 ProtocolVersion = 1
)

// String returns a string representation of the protocol version.
func (v ProtocolVersion) String() string {
	return fmt.Sprintf("V%d", v)
}

// SQLQueries is a subset of the sqlc.Querier interface that can be used to
// execute queries against the SQL graph tables.
//
//nolint:ll,interfacebloat
type SQLQueries interface {
	/*
		Node queries.
	*/
	UpsertNode(ctx context.Context, arg sqlc.UpsertNodeParams) (int64, error)
	GetNode(ctx context.Context, id int64) (sqlc.Node, error)
	GetNodeByPubKey(ctx context.Context, arg sqlc.GetNodeByPubKeyParams) (sqlc.Node, error)
	GetNodesByLastUpdateRange(ctx context.Context, arg sqlc.GetNodesByLastUpdateRangeParams) ([]sqlc.Node, error)
	ListNodeIDsAndPubKeys(ctx context.Context, version int16) ([]sqlc.ListNodeIDsAndPubKeysRow, error)
	ListNodes(ctx context.Context, version int16) ([]sqlc.ListNodesRow, error)
	IsPublicV1Node(ctx context.Context, pubKey []byte) (bool, error)
	GetUnconnectedNodes(ctx context.Context) ([]sqlc.GetUnconnectedNodesRow, error)
	DeleteNodeByPubKey(ctx context.Context, arg sqlc.DeleteNodeByPubKeyParams) (sql.Result, error)
	DeleteNode(ctx context.Context, id int64) error

	GetExtraNodeTypes(ctx context.Context, nodeID int64) ([]sqlc.NodeExtraType, error)
	UpsertNodeExtraType(ctx context.Context, arg sqlc.UpsertNodeExtraTypeParams) error
	DeleteExtraNodeType(ctx context.Context, arg sqlc.DeleteExtraNodeTypeParams) error

	InsertNodeAddress(ctx context.Context, arg sqlc.InsertNodeAddressParams) error
	GetNodeAddressesByPubKey(ctx context.Context, arg sqlc.GetNodeAddressesByPubKeyParams) ([]sqlc.GetNodeAddressesByPubKeyRow, error)
	DeleteNodeAddresses(ctx context.Context, nodeID int64) error

	InsertNodeFeature(ctx context.Context, arg sqlc.InsertNodeFeatureParams) error
	GetNodeFeatures(ctx context.Context, nodeID int64) ([]sqlc.NodeFeature, error)
	GetNodeFeaturesByPubKey(ctx context.Context, arg sqlc.GetNodeFeaturesByPubKeyParams) ([]int32, error)
	DeleteNodeFeature(ctx context.Context, arg sqlc.DeleteNodeFeatureParams) error

	/*
		Source node queries.
	*/
	AddSourceNode(ctx context.Context, nodeID int64) error
	GetSourceNodesByVersion(ctx context.Context, version int16) ([]sqlc.GetSourceNodesByVersionRow, error)
	GetSourceNodes(ctx context.Context) ([]sqlc.GetSourceNodesRow, error)

	/*
		Channel queries.
	*/
	CreateChannel(ctx context.Context, arg sqlc.CreateChannelParams) (int64, error)
	AddV1ChannelProof(ctx context.Context, arg sqlc.AddV1ChannelProofParams) error
	GetChannelAndNodesBySCID(ctx context.Context, arg sqlc.GetChannelAndNodesBySCIDParams) (sqlc.GetChannelAndNodesBySCIDRow, error)
	GetChannelBySCID(ctx context.Context, arg sqlc.GetChannelBySCIDParams) (sqlc.Channel, error)
	GetChannelByOutpoint(ctx context.Context, arg sqlc.GetChannelByOutpointParams) (sqlc.Channel, error)
	ListChannelsByNodeID(ctx context.Context, arg sqlc.ListChannelsByNodeIDParams) ([]sqlc.Channel, error)
	ListAllChannels(ctx context.Context, version int16) ([]sqlc.Channel, error)
	HighestSCID(ctx context.Context, version int16) ([]byte, error)
	GetSCIDByOutpoint(ctx context.Context, arg sqlc.GetSCIDByOutpointParams) ([]byte, error)
	GetPublicV1ChannelsBySCID(ctx context.Context, arg sqlc.GetPublicV1ChannelsBySCIDParams) ([]sqlc.Channel, error)
	GetChannelsByPolicyLastUpdateRange(ctx context.Context, arg sqlc.GetChannelsByPolicyLastUpdateRangeParams) ([]sqlc.Channel, error)
	GetChannelsBySCIDRange(ctx context.Context, arg sqlc.GetChannelsBySCIDRangeParams) ([]sqlc.Channel, error)
	DeleteChannel(ctx context.Context, id int64) error

	CreateChannelExtraType(ctx context.Context, arg sqlc.CreateChannelExtraTypeParams) error
	GetExtraChannelTypes(ctx context.Context, channelID int64) ([]sqlc.ChannelExtraType, error)
	InsertChannelFeature(ctx context.Context, arg sqlc.InsertChannelFeatureParams) error
	GetChannelFeatures(ctx context.Context, channelID int64) ([]sqlc.ChannelFeature, error)

	/*
		Channel Policy table queries.
	*/
	UpsertEdgePolicy(ctx context.Context, arg sqlc.UpsertEdgePolicyParams) (int64, error)
	GetChannelPolicyByChannelAndNode(ctx context.Context, arg sqlc.GetChannelPolicyByChannelAndNodeParams) (sqlc.ChannelPolicy, error)
	GetV1DisabledSCIDs(ctx context.Context) ([][]byte, error)

	UpsertChanPolicyExtraType(ctx context.Context, arg sqlc.UpsertChanPolicyExtraTypeParams) error
	GetChannelPolicyExtraTypes(ctx context.Context, channelPolicyID int64) ([]sqlc.ChannelPolicyExtraType, error)
	DeleteChannelPolicyExtraType(ctx context.Context, arg sqlc.DeleteChannelPolicyExtraTypeParams) error

	UpsertZombieChannel(ctx context.Context, arg sqlc.UpsertZombieChannelParams) error
	GetZombieChannel(ctx context.Context, arg sqlc.GetZombieChannelParams) (sqlc.ZombieChannel, error)
	CountZombieChannels(ctx context.Context, version int16) (int64, error)
	IsZombieChannel(ctx context.Context, arg sqlc.IsZombieChannelParams) (bool, error)
	DeleteZombieChannel(ctx context.Context, arg sqlc.DeleteZombieChannelParams) error

	/*
		Prune log table queries.
	*/
	UpsertPruneLogEntry(ctx context.Context, arg sqlc.UpsertPruneLogEntryParams) error
	GetPruneTip(ctx context.Context) (sqlc.PruneLog, error)
	DeletePruneLogEntriesInRange(ctx context.Context, arg sqlc.DeletePruneLogEntriesInRangeParams) error

	/*
		Closed SCID table queries.
	*/
	InsertClosedChannel(ctx context.Context, scid []byte) error
	IsClosedChannel(ctx context.Context, scid []byte) (bool, error)
}

// BatchedSQLQueries is a version of SQLQueries that's capable of batched
// database operations.
type BatchedSQLQueries interface {
	SQLQueries
	sqldb.BatchedTx[SQLQueries]
}

// SQLStore is an implementation of the V1Store interface that uses a SQL
// database as the backend.
//
// NOTE: currently, this temporarily embeds the KVStore struct so that we can
// implement the V1Store interface incrementally. For any method not
// implemented,  things will fall back to the KVStore. This is ONLY the case
// for the time being while this struct is purely used in unit tests only.
type SQLStore struct {
	cfg *SQLStoreConfig
	db  BatchedSQLQueries

	// cacheMu guards all caches (rejectCache and chanCache). If
	// this mutex will be acquired at the same time as the DB mutex then
	// the cacheMu MUST be acquired first to prevent deadlock.
	cacheMu     sync.RWMutex
	rejectCache *rejectCache
	chanCache   *channelCache

	chanScheduler batch.Scheduler[SQLQueries]
	nodeScheduler batch.Scheduler[SQLQueries]

	srcNodeID  int64
	srcNodePub route.Vertex
	srcNodeMu  sync.Mutex

	// Temporary fall-back to the KVStore so that we can implement the
	// interface incrementally.
	*KVStore
}

// A compile-time assertion to ensure that SQLStore implements the V1Store
// interface.
var _ V1Store = (*SQLStore)(nil)

// SQLStoreConfig holds the configuration for the SQLStore.
type SQLStoreConfig struct {
	// ChainHash is the genesis hash for the chain that all the gossip
	// messages in this store are aimed at.
	ChainHash chainhash.Hash
}

// NewSQLStore creates a new SQLStore instance given an open BatchedSQLQueries
// storage backend.
func NewSQLStore(cfg *SQLStoreConfig, db BatchedSQLQueries, kvStore *KVStore,
	options ...StoreOptionModifier) (*SQLStore, error) {

	opts := DefaultOptions()
	for _, o := range options {
		o(opts)
	}

	if opts.NoMigration {
		return nil, fmt.Errorf("the NoMigration option is not yet " +
			"supported for SQL stores")
	}

	s := &SQLStore{
		cfg:         cfg,
		db:          db,
		KVStore:     kvStore,
		rejectCache: newRejectCache(opts.RejectCacheSize),
		chanCache:   newChannelCache(opts.ChannelCacheSize),
	}

	s.chanScheduler = batch.NewTimeScheduler(
		db, &s.cacheMu, opts.BatchCommitInterval,
	)
	s.nodeScheduler = batch.NewTimeScheduler(
		db, nil, opts.BatchCommitInterval,
	)

	return s, nil
}

// AddLightningNode adds a vertex/node to the graph database. If the node is not
// in the database from before, this will add a new, unconnected one to the
// graph. If it is present from before, this will update that node's
// information.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) AddLightningNode(node *models.LightningNode,
	opts ...batch.SchedulerOption) error {

	ctx := context.TODO()

	r := &batch.Request[SQLQueries]{
		Opts: batch.NewSchedulerOptions(opts...),
		Do: func(queries SQLQueries) error {
			_, err := upsertNode(ctx, queries, node)
			return err
		},
	}

	return s.nodeScheduler.Execute(ctx, r)
}

// FetchLightningNode attempts to look up a target node by its identity public
// key. If the node isn't found in the database, then ErrGraphNodeNotFound is
// returned.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) FetchLightningNode(pubKey route.Vertex) (
	*models.LightningNode, error) {

	ctx := context.TODO()

	var node *models.LightningNode
	err := s.db.ExecTx(ctx, sqldb.ReadTxOpt(), func(db SQLQueries) error {
		var err error
		_, node, err = getNodeByPubKey(ctx, db, pubKey)

		return err
	}, sqldb.NoOpReset)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch node: %w", err)
	}

	return node, nil
}

// HasLightningNode determines if the graph has a vertex identified by the
// target node identity public key. If the node exists in the database, a
// timestamp of when the data for the node was lasted updated is returned along
// with a true boolean. Otherwise, an empty time.Time is returned with a false
// boolean.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) HasLightningNode(pubKey [33]byte) (time.Time, bool,
	error) {

	ctx := context.TODO()

	var (
		exists     bool
		lastUpdate time.Time
	)
	err := s.db.ExecTx(ctx, sqldb.ReadTxOpt(), func(db SQLQueries) error {
		dbNode, err := db.GetNodeByPubKey(
			ctx, sqlc.GetNodeByPubKeyParams{
				Version: int16(ProtocolV1),
				PubKey:  pubKey[:],
			},
		)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		} else if err != nil {
			return fmt.Errorf("unable to fetch node: %w", err)
		}

		exists = true

		if dbNode.LastUpdate.Valid {
			lastUpdate = time.Unix(dbNode.LastUpdate.Int64, 0)
		}

		return nil
	}, sqldb.NoOpReset)
	if err != nil {
		return time.Time{}, false,
			fmt.Errorf("unable to fetch node: %w", err)
	}

	return lastUpdate, exists, nil
}

// AddrsForNode returns all known addresses for the target node public key
// that the graph DB is aware of. The returned boolean indicates if the
// given node is unknown to the graph DB or not.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) AddrsForNode(nodePub *btcec.PublicKey) (bool, []net.Addr,
	error) {

	ctx := context.TODO()

	var (
		addresses []net.Addr
		known     bool
	)
	err := s.db.ExecTx(ctx, sqldb.ReadTxOpt(), func(db SQLQueries) error {
		var err error
		known, addresses, err = getNodeAddresses(
			ctx, db, nodePub.SerializeCompressed(),
		)
		if err != nil {
			return fmt.Errorf("unable to fetch node addresses: %w",
				err)
		}

		return nil
	}, sqldb.NoOpReset)
	if err != nil {
		return false, nil, fmt.Errorf("unable to get addresses for "+
			"node(%x): %w", nodePub.SerializeCompressed(), err)
	}

	return known, addresses, nil
}

// DeleteLightningNode starts a new database transaction to remove a vertex/node
// from the database according to the node's public key.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) DeleteLightningNode(pubKey route.Vertex) error {
	ctx := context.TODO()

	err := s.db.ExecTx(ctx, sqldb.WriteTxOpt(), func(db SQLQueries) error {
		res, err := db.DeleteNodeByPubKey(
			ctx, sqlc.DeleteNodeByPubKeyParams{
				Version: int16(ProtocolV1),
				PubKey:  pubKey[:],
			},
		)
		if err != nil {
			return err
		}

		rows, err := res.RowsAffected()
		if err != nil {
			return err
		}

		if rows == 0 {
			return ErrGraphNodeNotFound
		} else if rows > 1 {
			return fmt.Errorf("deleted %d rows, expected 1", rows)
		}

		return err
	}, sqldb.NoOpReset)
	if err != nil {
		return fmt.Errorf("unable to delete node: %w", err)
	}

	return nil
}

// FetchNodeFeatures returns the features of the given node. If no features are
// known for the node, an empty feature vector is returned.
//
// NOTE: this is part of the graphdb.NodeTraverser interface.
func (s *SQLStore) FetchNodeFeatures(nodePub route.Vertex) (
	*lnwire.FeatureVector, error) {

	ctx := context.TODO()

	return fetchNodeFeatures(ctx, s.db, nodePub)
}

// LookupAlias attempts to return the alias as advertised by the target node.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) LookupAlias(pub *btcec.PublicKey) (string, error) {
	var (
		ctx   = context.TODO()
		alias string
	)
	err := s.db.ExecTx(ctx, sqldb.ReadTxOpt(), func(db SQLQueries) error {
		dbNode, err := db.GetNodeByPubKey(
			ctx, sqlc.GetNodeByPubKeyParams{
				Version: int16(ProtocolV1),
				PubKey:  pub.SerializeCompressed(),
			},
		)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNodeAliasNotFound
		} else if err != nil {
			return fmt.Errorf("unable to fetch node: %w", err)
		}

		if !dbNode.Alias.Valid {
			return ErrNodeAliasNotFound
		}

		alias = dbNode.Alias.String

		return nil
	}, sqldb.NoOpReset)
	if err != nil {
		return "", fmt.Errorf("unable to look up alias: %w", err)
	}

	return alias, nil
}

// SourceNode returns the source node of the graph. The source node is treated
// as the center node within a star-graph. This method may be used to kick off
// a path finding algorithm in order to explore the reachability of another
// node based off the source node.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) SourceNode() (*models.LightningNode, error) {
	ctx := context.TODO()

	var node *models.LightningNode
	err := s.db.ExecTx(ctx, sqldb.ReadTxOpt(), func(db SQLQueries) error {
		_, nodePub, err := s.getSourceNode(ctx, db, ProtocolV1)
		if err != nil {
			return fmt.Errorf("unable to fetch V1 source node: %w",
				err)
		}

		_, node, err = getNodeByPubKey(ctx, db, nodePub)

		return err
	}, sqldb.NoOpReset)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch source node: %w", err)
	}

	return node, nil
}

// SetSourceNode sets the source node within the graph database. The source
// node is to be used as the center of a star-graph within path finding
// algorithms.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) SetSourceNode(node *models.LightningNode) error {
	ctx := context.TODO()

	return s.db.ExecTx(ctx, sqldb.WriteTxOpt(), func(db SQLQueries) error {
		id, err := upsertNode(ctx, db, node)
		if err != nil {
			return fmt.Errorf("unable to upsert source node: %w",
				err)
		}

		// Make sure that if a source node for this version is already
		// set, then the ID is the same as the one we are about to set.
		dbSourceNodeID, _, err := s.getSourceNode(ctx, db, ProtocolV1)
		if err != nil && !errors.Is(err, ErrSourceNodeNotSet) {
			return fmt.Errorf("unable to fetch source node: %w",
				err)
		} else if err == nil {
			if dbSourceNodeID != id {
				return fmt.Errorf("v1 source node already "+
					"set to a different node: %d vs %d",
					dbSourceNodeID, id)
			}

			return nil
		}

		return db.AddSourceNode(ctx, id)
	}, sqldb.NoOpReset)
}

// NodeUpdatesInHorizon returns all the known lightning node which have an
// update timestamp within the passed range. This method can be used by two
// nodes to quickly determine if they have the same set of up to date node
// announcements.
//
// NOTE: This is part of the V1Store interface.
func (s *SQLStore) NodeUpdatesInHorizon(startTime,
	endTime time.Time) ([]models.LightningNode, error) {

	ctx := context.TODO()

	var nodes []models.LightningNode
	err := s.db.ExecTx(ctx, sqldb.ReadTxOpt(), func(db SQLQueries) error {
		dbNodes, err := db.GetNodesByLastUpdateRange(
			ctx, sqlc.GetNodesByLastUpdateRangeParams{
				StartTime: sqldb.SQLInt64(startTime.Unix()),
				EndTime:   sqldb.SQLInt64(endTime.Unix()),
			},
		)
		if err != nil {
			return fmt.Errorf("unable to fetch nodes: %w", err)
		}

		for _, dbNode := range dbNodes {
			node, err := buildNode(ctx, db, &dbNode)
			if err != nil {
				return fmt.Errorf("unable to build node: %w",
					err)
			}

			nodes = append(nodes, *node)
		}

		return nil
	}, sqldb.NoOpReset)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch nodes: %w", err)
	}

	return nodes, nil
}

// AddChannelEdge adds a new (undirected, blank) edge to the graph database. An
// undirected edge from the two target nodes are created. The information stored
// denotes the static attributes of the channel, such as the channelID, the keys
// involved in creation of the channel, and the set of features that the channel
// supports. The chanPoint and chanID are used to uniquely identify the edge
// globally within the database.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) AddChannelEdge(edge *models.ChannelEdgeInfo,
	opts ...batch.SchedulerOption) error {

	ctx := context.TODO()

	var alreadyExists bool
	r := &batch.Request[SQLQueries]{
		Opts: batch.NewSchedulerOptions(opts...),
		Reset: func() {
			alreadyExists = false
		},
		Do: func(tx SQLQueries) error {
			err := insertChannel(ctx, tx, edge)

			// Silence ErrEdgeAlreadyExist so that the batch can
			// succeed, but propagate the error via local state.
			if errors.Is(err, ErrEdgeAlreadyExist) {
				alreadyExists = true
				return nil
			}

			return err
		},
		OnCommit: func(err error) error {
			switch {
			case err != nil:
				return err
			case alreadyExists:
				return ErrEdgeAlreadyExist
			default:
				s.rejectCache.remove(edge.ChannelID)
				s.chanCache.remove(edge.ChannelID)
				return nil
			}
		},
	}

	return s.chanScheduler.Execute(ctx, r)
}

// HighestChanID returns the "highest" known channel ID in the channel graph.
// This represents the "newest" channel from the PoV of the chain. This method
// can be used by peers to quickly determine if their graphs are in sync.
//
// NOTE: This is part of the V1Store interface.
func (s *SQLStore) HighestChanID() (uint64, error) {
	ctx := context.TODO()

	var highestChanID uint64
	err := s.db.ExecTx(ctx, sqldb.ReadTxOpt(), func(db SQLQueries) error {
		chanID, err := db.HighestSCID(ctx, int16(ProtocolV1))
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		} else if err != nil {
			return fmt.Errorf("unable to fetch highest chan ID: %w",
				err)
		}

		highestChanID = byteOrder.Uint64(chanID)

		return nil
	}, sqldb.NoOpReset)
	if err != nil {
		return 0, fmt.Errorf("unable to fetch highest chan ID: %w", err)
	}

	return highestChanID, nil
}

// UpdateEdgePolicy updates the edge routing policy for a single directed edge
// within the database for the referenced channel. The `flags` attribute within
// the ChannelEdgePolicy determines which of the directed edges are being
// updated. If the flag is 1, then the first node's information is being
// updated, otherwise it's the second node's information. The node ordering is
// determined by the lexicographical ordering of the identity public keys of the
// nodes on either side of the channel.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) UpdateEdgePolicy(edge *models.ChannelEdgePolicy,
	opts ...batch.SchedulerOption) (route.Vertex, route.Vertex, error) {

	ctx := context.TODO()

	var (
		isUpdate1    bool
		edgeNotFound bool
		from, to     route.Vertex
	)

	r := &batch.Request[SQLQueries]{
		Opts: batch.NewSchedulerOptions(opts...),
		Reset: func() {
			isUpdate1 = false
			edgeNotFound = false
		},
		Do: func(tx SQLQueries) error {
			var err error
			from, to, isUpdate1, err = updateChanEdgePolicy(
				ctx, tx, edge,
			)
			if err != nil {
				log.Errorf("UpdateEdgePolicy faild: %v", err)
			}

			// Silence ErrEdgeNotFound so that the batch can
			// succeed, but propagate the error via local state.
			if errors.Is(err, ErrEdgeNotFound) {
				edgeNotFound = true
				return nil
			}

			return err
		},
		OnCommit: func(err error) error {
			switch {
			case err != nil:
				return err
			case edgeNotFound:
				return ErrEdgeNotFound
			default:
				s.updateEdgeCache(edge, isUpdate1)
				return nil
			}
		},
	}

	err := s.chanScheduler.Execute(ctx, r)

	return from, to, err
}

// ForEachSourceNodeChannel iterates through all channels of the source node,
// executing the passed callback on each. The call-back is provided with the
// channel's outpoint, whether we have a policy for the channel and the channel
// peer's node information.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) ForEachSourceNodeChannel(cb func(chanPoint wire.OutPoint,
	havePolicy bool, otherNode *models.LightningNode) error) error {

	var ctx = context.TODO()

	return s.db.ExecTx(ctx, sqldb.ReadTxOpt(), func(db SQLQueries) error {
		nodeID, nodePub, err := s.getSourceNode(ctx, db, ProtocolV1)
		if err != nil {
			return fmt.Errorf("unable to fetch source node: %w",
				err)
		}

		return forEachNodeChannel(
			ctx, db, s.cfg.ChainHash, nodeID,
			func(info *models.ChannelEdgeInfo,
				outPolicy *models.ChannelEdgePolicy,
				_ *models.ChannelEdgePolicy) error {

				// Fetch the other node.
				var (
					otherNodePub [33]byte
					node1        = info.NodeKey1Bytes
					node2        = info.NodeKey2Bytes
				)
				switch {
				case bytes.Equal(node1[:], nodePub[:]):
					otherNodePub = node2
				case bytes.Equal(node2[:], nodePub[:]):
					otherNodePub = node1
				default:
					return fmt.Errorf("node not " +
						"participating in this channel")
				}

				_, otherNode, err := getNodeByPubKey(
					ctx, db, otherNodePub,
				)
				if err != nil {
					return fmt.Errorf("unable to fetch "+
						"other node(%x): %w",
						otherNodePub, err)
				}

				return cb(
					info.ChannelPoint, outPolicy != nil,
					otherNode,
				)
			},
		)
	}, func() {})
}

// ForEachNode iterates through all the stored vertices/nodes in the graph,
// executing the passed callback with each node encountered. If the callback
// returns an error, then the transaction is aborted and the iteration stops
// early. Any operations performed on the NodeTx passed to the call-back are
// executed under the same read transaction and so, methods on the NodeTx object
// _MUST_ only be called from within the call-back.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) ForEachNode(cb func(tx NodeRTx) error) error {
	var ctx = context.TODO()

	return s.db.ExecTx(ctx, sqldb.ReadTxOpt(), func(db SQLQueries) error {
		return forEachNode(ctx, db,
			func(nodeID int64, nodePub route.Vertex) error {
				node, err := getNodeByDBID(ctx, db, nodeID)
				if err != nil {
					return fmt.Errorf("unable to get "+
						"node(id=%d): %w", nodeID, err)
				}

				return cb(newSQLGraphNodeTx(
					db, s.cfg.ChainHash, nodeID, node,
				))
			},
		)
	}, func() {})
}

// sqlGraphNodeTx is an implementation of the NodeRTx interface backed by the
// SQLStore and a SQL transaction.
type sqlGraphNodeTx struct {
	db    SQLQueries
	id    int64
	node  *models.LightningNode
	chain chainhash.Hash
}

// A compile-time constraint to ensure sqlGraphNodeTx implements the NodeRTx
// interface.
var _ NodeRTx = (*sqlGraphNodeTx)(nil)

func newSQLGraphNodeTx(db SQLQueries, chain chainhash.Hash,
	id int64, node *models.LightningNode) *sqlGraphNodeTx {

	return &sqlGraphNodeTx{
		db:    db,
		chain: chain,
		id:    id,
		node:  node,
	}
}

// Node returns the raw information of the node.
//
// NOTE: This is a part of the NodeRTx interface.
func (s *sqlGraphNodeTx) Node() *models.LightningNode {
	return s.node
}

// ForEachChannel can be used to iterate over the node's channels under the same
// transaction used to fetch the node.
//
// NOTE: This is a part of the NodeRTx interface.
func (s *sqlGraphNodeTx) ForEachChannel(cb func(*models.ChannelEdgeInfo,
	*models.ChannelEdgePolicy, *models.ChannelEdgePolicy) error) error {

	ctx := context.TODO()

	return forEachNodeChannel(ctx, s.db, s.chain, s.id, cb)
}

// FetchNode fetches the node with the given pub key under the same transaction
// used to fetch the current node. The returned node is also a NodeRTx and any
// operations on that NodeRTx will also be done under the same transaction.
//
// NOTE: This is a part of the NodeRTx interface.
func (s *sqlGraphNodeTx) FetchNode(nodePub route.Vertex) (NodeRTx, error) {
	ctx := context.TODO()

	id, node, err := getNodeByPubKey(ctx, s.db, nodePub)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch V1 node(%x): %w",
			nodePub, err)
	}

	return newSQLGraphNodeTx(s.db, s.chain, id, node), nil
}

// ForEachNodeCacheable iterates through all the stored vertices/nodes in the
// graph, executing the passed callback with each node encountered. If the
// callback returns an error, then the transaction is aborted and the iteration
// stops early.
//
// NOTE: This is a part of the V1Store interface.
func (s *SQLStore) ForEachNodeCacheable(cb func(route.Vertex,
	*lnwire.FeatureVector) error) error {

	ctx := context.TODO()

	err := s.db.ExecTx(ctx, sqldb.ReadTxOpt(), func(db SQLQueries) error {
		return forEachNode(ctx, db, func(nodeID int64,
			nodePub route.Vertex) error {

			features, err := getNodeFeatures(ctx, db, nodeID)
			if err != nil {
				return fmt.Errorf("unable to fetch node "+
					"features: %w", err)
			}

			return cb(nodePub, features)
		})
	}, func() {})
	if err != nil {
		return fmt.Errorf("unable to fetch nodes: %w", err)
	}

	return nil
}

// ForEachChannel iterates through all the channel edges stored within the
// graph and invokes the passed callback for each edge. The callback takes two
// edges as since this is a directed graph, both the in/out edges are visited.
// If the callback returns an error, then the transaction is aborted and the
// iteration stops early.
//
// NOTE: If an edge can't be found, or wasn't advertised, then a nil pointer
// for that particular channel edge routing policy will be passed into the
// callback.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) ForEachChannel(cb func(*models.ChannelEdgeInfo,
	*models.ChannelEdgePolicy,
	*models.ChannelEdgePolicy) error) error {

	var ctx = context.TODO()

	return s.db.ExecTx(ctx, sqldb.ReadTxOpt(), func(db SQLQueries) error {
		dbChannels, err := db.ListAllChannels(ctx, int16(ProtocolV1))
		if err != nil {
			return fmt.Errorf("unable to fetch channels: %w", err)
		}

		for _, dbChannel := range dbChannels {
			e, p1, p2, err := buildChannel(
				ctx, db, s.cfg.ChainHash, dbChannel,
			)
			if err != nil {
				return fmt.Errorf("unable to build channel: %w",
					err)
			}

			if err := cb(e, p1, p2); err != nil {
				return err
			}
		}

		return nil
	}, func() {})
}

// ChanUpdatesInHorizon returns all the known channel edges which have at least
// one edge that has an update timestamp within the specified horizon.
//
// NOTE: This is part of the V1Store interface.
func (s *SQLStore) ChanUpdatesInHorizon(startTime,
	endTime time.Time) ([]ChannelEdge, error) {

	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	var (
		ctx = context.TODO()
		// To ensure we don't return duplicate ChannelEdges, we'll use an
		// additional map to keep track of the edges already seen to prevent
		// re-adding it.
		edgesSeen    = make(map[uint64]struct{})
		edgesToCache = make(map[uint64]ChannelEdge)
		edges        []ChannelEdge
		hits         int
	)
	err := s.db.ExecTx(ctx, sqldb.ReadTxOpt(), func(db SQLQueries) error {
		dbChans, err := db.GetChannelsByPolicyLastUpdateRange(
			ctx, sqlc.GetChannelsByPolicyLastUpdateRangeParams{
				Version: int16(ProtocolV1),
				StartTime: sql.NullInt64{
					Valid: true,
					Int64: startTime.Unix(),
				},
				EndTime: sql.NullInt64{
					Valid: true,
					Int64: endTime.Unix(),
				},
			},
		)
		if err != nil {
			return fmt.Errorf("unable to fetch channels: %w", err)
		}

		for _, dbChan := range dbChans {
			// If we've already retrieved the info and policies for
			// this edge, then we can skip it as we don't need to do
			// so again.
			chanIDInt := byteOrder.Uint64(dbChan.Scid)
			if _, ok := edgesSeen[chanIDInt]; ok {
				continue
			}

			if channel, ok := s.chanCache.get(chanIDInt); ok {
				hits++
				edgesSeen[chanIDInt] = struct{}{}
				edges = append(edges, channel)

				continue
			}

			channel, p1, p2, err := buildChannel(
				ctx, db, s.cfg.ChainHash, dbChan,
			)
			if err != nil {
				return fmt.Errorf("unable to build channel: %w",
					err)
			}

			node1, err := getNodeByDBID(ctx, db, dbChan.NodeID1)
			if err != nil {
				return fmt.Errorf("unable to fetch node(%d): "+
					"%w", dbChan.NodeID1, err)
			}

			node2, err := getNodeByDBID(ctx, db, dbChan.NodeID2)
			if err != nil {
				return fmt.Errorf("unable to fetch node(%d): "+
					"%w", dbChan.NodeID2, err)
			}

			edgesSeen[chanIDInt] = struct{}{}
			chanEdge := ChannelEdge{
				Info:    channel,
				Policy1: p1,
				Policy2: p2,
				Node1:   node1,
				Node2:   node2,
			}
			edges = append(edges, chanEdge)
			edgesToCache[chanIDInt] = chanEdge
		}

		return nil
	}, func() {
		edgesSeen = make(map[uint64]struct{})
		edgesToCache = make(map[uint64]ChannelEdge)
		edges = nil
	})
	if err != nil {
		return nil, fmt.Errorf("unable to fetch channels: %w", err)
	}

	// Insert any edges loaded from disk into the cache.
	for chanid, channel := range edgesToCache {
		s.chanCache.insert(chanid, channel)
	}

	log.Debugf("ChanUpdatesInHorizon hit percentage: %f (%d/%d)",
		float64(hits)/float64(len(edges)), hits, len(edges))

	return edges, nil
}

// FilterChannelRange returns the channel ID's of all known channels which were
// mined in a block height within the passed range. The channel IDs are grouped
// by their common block height. This method can be used to quickly share with a
// peer the set of channels we know of within a particular range to catch them
// up after a period of time offline. If withTimestamps is true then the
// timestamp info of the latest received channel update messages of the channel
// will be included in the response.
//
// NOTE: This is part of the V1Store interface.
func (s *SQLStore) FilterChannelRange(startHeight, endHeight uint32,
	withTimestamps bool) ([]BlockChannelRange, error) {

	var (
		ctx       = context.TODO()
		startSCID = &lnwire.ShortChannelID{
			BlockHeight: startHeight,
		}
		endSCID = lnwire.ShortChannelID{
			BlockHeight: endHeight,
			TxIndex:     math.MaxUint32 & 0x00ffffff,
			TxPosition:  math.MaxUint16,
		}
	)

	var chanIDStart [8]byte
	byteOrder.PutUint64(chanIDStart[:], startSCID.ToUint64())
	var chanIDEnd [8]byte
	byteOrder.PutUint64(chanIDEnd[:], endSCID.ToUint64())

	// 1) get all channels where channelID is between start and end chan ID.
	// 2) skip if not public (ie, no channel_proof)
	// 3) collect that channel.
	// 4) if timestamps are wanted, fetch both policies for node 1 and node2
	//    and add those timestamps to the collected channel.
	channelsPerBlock := make(map[uint32][]ChannelUpdateInfo)
	err := s.db.ExecTx(ctx, sqldb.ReadTxOpt(), func(db SQLQueries) error {
		dbChans, err := db.GetPublicV1ChannelsBySCID(
			ctx, sqlc.GetPublicV1ChannelsBySCIDParams{
				StartScid: chanIDStart[:],
				EndScid:   chanIDEnd[:],
			},
		)
		if err != nil {
			return fmt.Errorf("unable to fetch channel range: %w",
				err)
		}

		for _, dbChan := range dbChans {
			cid := lnwire.NewShortChanIDFromInt(
				byteOrder.Uint64(dbChan.Scid),
			)
			chanInfo := NewChannelUpdateInfo(
				cid, time.Time{}, time.Time{},
			)

			if !withTimestamps {
				channelsPerBlock[cid.BlockHeight] = append(
					channelsPerBlock[cid.BlockHeight],
					chanInfo,
				)

				continue
			}

			//nolint:ll
			node1Policy, err := db.GetChannelPolicyByChannelAndNode(
				ctx, sqlc.GetChannelPolicyByChannelAndNodeParams{
					Version:   int16(ProtocolV1),
					ChannelID: dbChan.ID,
					NodeID:    dbChan.NodeID1,
				},
			)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("unable to fetch node1 "+
					"policy: %w", err)
			} else if err == nil {
				chanInfo.Node1UpdateTimestamp = time.Unix(
					node1Policy.LastUpdate.Int64, 0,
				)
			}

			//nolint:ll
			node2Policy, err := db.GetChannelPolicyByChannelAndNode(
				ctx, sqlc.GetChannelPolicyByChannelAndNodeParams{
					Version:   int16(ProtocolV1),
					ChannelID: dbChan.ID,
					NodeID:    dbChan.NodeID2,
				},
			)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("unable to fetch node2 "+
					"policy: %w", err)
			} else if err == nil {
				chanInfo.Node2UpdateTimestamp = time.Unix(
					node2Policy.LastUpdate.Int64, 0,
				)
			}

			channelsPerBlock[cid.BlockHeight] = append(
				channelsPerBlock[cid.BlockHeight], chanInfo,
			)
		}

		return nil
	}, func() {
		channelsPerBlock = make(map[uint32][]ChannelUpdateInfo)
	})
	if err != nil {
		return nil, fmt.Errorf("unable to fetch channel range: %w", err)
	}

	if len(channelsPerBlock) == 0 {
		return nil, nil
	}

	// Return the channel ranges in ascending block height order.
	blocks := make([]uint32, 0, len(channelsPerBlock))
	for block := range channelsPerBlock {
		blocks = append(blocks, block)
	}
	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i] < blocks[j]
	})

	channelRanges := make([]BlockChannelRange, 0, len(channelsPerBlock))
	for _, block := range blocks {
		channelRanges = append(channelRanges, BlockChannelRange{
			Height:   block,
			Channels: channelsPerBlock[block],
		})
	}

	return channelRanges, nil
}

// ForEachNodeChannel iterates through all channels of the given node,
// executing the passed callback with an edge info structure and the policies
// of each end of the channel. The first edge policy is the outgoing edge *to*
// the connecting node, while the second is the incoming edge *from* the
// connecting node. If the callback returns an error, then the iteration is
// halted with the error propagated back up to the caller.
//
// Unknown policies are passed into the callback as nil values.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) ForEachNodeChannel(nodePub route.Vertex,
	cb func(*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy) error) error {

	var ctx = context.TODO()

	return s.db.ExecTx(ctx, sqldb.ReadTxOpt(), func(db SQLQueries) error {
		dbNode, err := db.GetNodeByPubKey(
			ctx, sqlc.GetNodeByPubKeyParams{
				Version: int16(ProtocolV1),
				PubKey:  nodePub[:],
			},
		)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		} else if err != nil {
			return fmt.Errorf("unable to fetch node: %w", err)
		}

		return forEachNodeChannel(
			ctx, db, s.cfg.ChainHash, dbNode.ID, cb,
		)
	}, func() {})
}

// ForEachNodeDirectedChannel iterates through all channels of a given node,
// executing the passed callback on the directed edge representing the channel
// and its incoming policy. If the callback returns an error, then the iteration
// is halted with the error propagated back up to the caller.
//
// Unknown policies are passed into the callback as nil values.
//
// NOTE: this is part of the graphdb.NodeTraverser interface.
func (s *SQLStore) ForEachNodeDirectedChannel(nodePub route.Vertex,
	cb func(channel *DirectedChannel) error) error {

	var ctx = context.TODO()

	return s.db.ExecTx(ctx, sqldb.ReadTxOpt(), func(db SQLQueries) error {
		return forEachNodeDirectedChannel(
			ctx, db, s.cfg.ChainHash, nodePub, cb,
		)
	}, func() {})
}

// ForEachNodeCached is similar to forEachNode, but it returns DirectedChannel
// data to the call-back.
//
// NOTE: The callback contents MUST not be modified.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) ForEachNodeCached(cb func(node route.Vertex,
	chans map[uint64]*DirectedChannel) error) error {

	var ctx = context.TODO()

	return s.db.ExecTx(ctx, sqldb.ReadTxOpt(), func(db SQLQueries) error {
		nodes, err := db.ListNodes(ctx, int16(ProtocolV1))
		if err != nil {
			return fmt.Errorf("unable to fetch node ids: %w", err)
		}

		for _, node := range nodes {
			features, err := getNodeFeatures(ctx, db, node.ID)
			if err != nil {
				return fmt.Errorf("unable to fetch node "+
					"features: %w", err)
			}

			var nodePub route.Vertex
			copy(nodePub[:], node.PubKey)

			toNodeCallback := func() route.Vertex {
				return nodePub
			}

			dbChannels, err := db.ListChannelsByNodeID(
				ctx, sqlc.ListChannelsByNodeIDParams{
					Version: int16(ProtocolV1),
					NodeID1: node.ID,
				},
			)
			if err != nil {
				return fmt.Errorf("unable to fetch channels: "+
					"%w", err)
			}

			channels := make(
				map[uint64]*DirectedChannel, len(dbChannels),
			)
			for _, dbChannel := range dbChannels {
				e, p1, p2, err := buildChannel(
					ctx, db, s.cfg.ChainHash, dbChannel,
				)
				if err != nil {
					return fmt.Errorf("unable to build "+
						"channel: %w", err)
				}

				// Determine the outgoing and incoming policy
				// for this channel and node combo.
				outPolicy, inPolicy := p1, p2
				if p1 != nil && p1.ToNode == nodePub {
					outPolicy, inPolicy = p2, p1
				} else if p2 != nil && p2.ToNode != nodePub {
					outPolicy, inPolicy = p2, p1
				}

				var cachedInPolicy *models.CachedEdgePolicy
				if inPolicy != nil {
					cachedInPolicy = models.NewCachedPolicy(
						p2,
					)
					cachedInPolicy.ToNodePubKey =
						toNodeCallback
					cachedInPolicy.ToNodeFeatures =
						features
				}

				var inboundFee lnwire.Fee
				if outPolicy != nil {
					// Extract inbound fee. If there is a
					// decoding error, skip this edge.
					_, err := outPolicy.ExtraOpaqueData.
						ExtractRecords(&inboundFee)
					if err != nil {
						return nil
					}
				}

				directedChannel := &DirectedChannel{
					ChannelID: e.ChannelID,
					IsNode1: nodePub ==
						e.NodeKey1Bytes,
					OtherNode:    e.NodeKey2Bytes,
					Capacity:     e.Capacity,
					OutPolicySet: p1 != nil,
					InPolicy:     cachedInPolicy,
					InboundFee:   inboundFee,
				}

				if nodePub == e.NodeKey2Bytes {
					directedChannel.OtherNode =
						e.NodeKey1Bytes
				}

				channels[e.ChannelID] = directedChannel
			}

			if err := cb(nodePub, channels); err != nil {
				return err
			}
		}

		return nil
	}, func() {})
}

// MarkEdgeZombie attempts to mark a channel identified by its channel ID as a
// zombie. This method is used on an ad-hoc basis, when channels need to be
// marked as zombies outside the normal pruning cycle.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) MarkEdgeZombie(chanID uint64,
	pubKey1, pubKey2 [33]byte) error {

	ctx := context.TODO()

	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	err := s.db.ExecTx(ctx, sqldb.WriteTxOpt(), func(db SQLQueries) error {
		return db.UpsertZombieChannel(
			ctx, sqlc.UpsertZombieChannelParams{
				Version:  int16(ProtocolV1),
				Scid:     int64(chanID),
				NodeKey1: pubKey1[:],
				NodeKey2: pubKey2[:],
			},
		)
	}, func() {})
	if err != nil {
		return err
	}

	s.rejectCache.remove(chanID)
	s.chanCache.remove(chanID)

	return nil
}

// MarkEdgeLive clears an edge from our zombie index, deeming it as live.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) MarkEdgeLive(chanID uint64) error {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	var ctx = context.TODO()
	err := s.db.ExecTx(ctx, sqldb.WriteTxOpt(), func(db SQLQueries) error {
		_, err := db.GetZombieChannel(
			ctx, sqlc.GetZombieChannelParams{
				Scid:    int64(chanID),
				Version: int16(ProtocolV1),
			},
		)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrZombieEdgeNotFound
		} else if err != nil {
			return fmt.Errorf("unable to fetch zombie channel: %w",
				err)
		}

		return db.DeleteZombieChannel(
			ctx, sqlc.DeleteZombieChannelParams{
				Scid:    int64(chanID),
				Version: int16(ProtocolV1),
			},
		)
	}, func() {})
	if err != nil {
		return err
	}

	s.rejectCache.remove(chanID)
	s.chanCache.remove(chanID)

	return err
}

// IsZombieEdge returns whether the edge is considered zombie. If it is a
// zombie, then the two node public keys corresponding to this edge are also
// returned.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) IsZombieEdge(chanID uint64) (bool, [33]byte, [33]byte) {
	var (
		ctx              = context.TODO()
		isZombie         bool
		pubKey1, pubKey2 route.Vertex
	)
	err := s.db.ExecTx(ctx, sqldb.ReadTxOpt(), func(db SQLQueries) error {
		zombie, err := db.GetZombieChannel(
			ctx, sqlc.GetZombieChannelParams{
				Scid:    int64(chanID),
				Version: int16(ProtocolV1),
			},
		)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("unable to fetch zombie channel: %w",
				err)
		}

		copy(pubKey1[:], zombie.NodeKey1)
		copy(pubKey2[:], zombie.NodeKey2)
		isZombie = true

		return nil
	}, func() {})
	if err != nil {
		return false, route.Vertex{}, route.Vertex{}
	}

	return isZombie, pubKey1, pubKey2
}

// NumZombies returns the current number of zombie channels in the graph.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) NumZombies() (uint64, error) {
	var (
		ctx        = context.TODO()
		numZombies uint64
	)
	err := s.db.ExecTx(ctx, sqldb.ReadTxOpt(), func(db SQLQueries) error {
		count, err := db.CountZombieChannels(ctx, int16(ProtocolV1))
		if err != nil {
			return fmt.Errorf("unable to count zombie channels: %w",
				err)
		}

		numZombies = uint64(count)

		return nil
	}, func() {})
	if err != nil {
		return 0, fmt.Errorf("unable to count zombies: %w", err)
	}

	return numZombies, nil
}

// DeleteChannelEdges removes edges with the given channel IDs from the
// database and marks them as zombies. This ensures that we're unable to re-add
// it to our database once again. If an edge does not exist within the
// database, then ErrEdgeNotFound will be returned. If strictZombiePruning is
// true, then when we mark these edges as zombies, we'll set up the keys such
// that we require the node that failed to send the fresh update to be the one
// that resurrects the channel from its zombie state. The markZombie bool
// denotes whether to mark the channel as a zombie.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) DeleteChannelEdges(strictZombiePruning, markZombie bool,
	chanIDs ...uint64) ([]*models.ChannelEdgeInfo, error) {

	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	var (
		ctx     = context.TODO()
		deleted []*models.ChannelEdgeInfo
	)
	err := s.db.ExecTx(ctx, sqldb.WriteTxOpt(), func(db SQLQueries) error {
		for _, chanID := range chanIDs {
			var chanIDB [8]byte
			byteOrder.PutUint64(chanIDB[:], chanID)

			dbChan, err := db.GetChannelBySCID(
				ctx, sqlc.GetChannelBySCIDParams{
					Scid:    chanIDB[:],
					Version: int16(ProtocolV1),
				},
			)
			if errors.Is(err, sql.ErrNoRows) {
				return ErrEdgeNotFound
			} else if err != nil {
				return fmt.Errorf("unable to fetch channel: %w",
					err)
			}

			info, e1, e2, err := buildChannel(
				ctx, db, s.cfg.ChainHash, dbChan,
			)
			if err != nil {
				return fmt.Errorf("unable to build channel: %w",
					err)
			}

			deleted = append(deleted, info)

			err = db.DeleteChannel(ctx, dbChan.ID)
			if err != nil {
				return fmt.Errorf("unable to delete "+
					"channel: %w", err)
			}

			if !markZombie {
				continue
			}

			nodeKey1, nodeKey2 := info.NodeKey1Bytes,
				info.NodeKey2Bytes
			if strictZombiePruning {
				nodeKey1, nodeKey2 = makeZombiePubkeys(
					info, e1, e2,
				)
			}

			err = db.UpsertZombieChannel(
				ctx, sqlc.UpsertZombieChannelParams{
					Version:  int16(ProtocolV1),
					Scid:     int64(chanID),
					NodeKey1: nodeKey1[:],
					NodeKey2: nodeKey2[:],
				},
			)
			if err != nil {
				return fmt.Errorf("unable to mark channel as "+
					"zombie: %w", err)
			}
		}

		return nil
	}, func() {})
	if err != nil {
		return nil, fmt.Errorf("unable to delete channel edges: %w",
			err)
	}

	for _, chanID := range chanIDs {
		s.rejectCache.remove(chanID)
		s.chanCache.remove(chanID)
	}

	return deleted, nil
}

// FetchChannelEdgesByID attempts to lookup the two directed edges for the
// channel identified by the channel ID. If the channel can't be found, then
// ErrEdgeNotFound is returned. A struct which houses the general information
// for the channel itself is returned as well as two structs that contain the
// routing policies for the channel in either direction.
//
// ErrZombieEdge an be returned if the edge is currently marked as a zombie
// within the database. In this case, the ChannelEdgePolicy's will be nil, and
// the ChannelEdgeInfo will only include the public keys of each node.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) FetchChannelEdgesByID(chanID uint64) (
	*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
	*models.ChannelEdgePolicy, error) {

	var (
		ctx              = context.TODO()
		edge             *models.ChannelEdgeInfo
		policy1, policy2 *models.ChannelEdgePolicy
	)
	err := s.db.ExecTx(ctx, sqldb.ReadTxOpt(), func(db SQLQueries) error {
		var chanIDB [8]byte
		byteOrder.PutUint64(chanIDB[:], chanID)

		dbChan, err := db.GetChannelBySCID(
			ctx, sqlc.GetChannelBySCIDParams{
				Scid:    chanIDB[:],
				Version: int16(ProtocolV1),
			},
		)
		if errors.Is(err, sql.ErrNoRows) {
			// First check if this edge is perhaps in the zombie
			// index.
			isZombie, err := db.IsZombieChannel(
				ctx, sqlc.IsZombieChannelParams{
					Scid:    int64(chanID),
					Version: int16(ProtocolV1),
				},
			)
			if err != nil {
				return fmt.Errorf("unable to check if "+
					"channel is zombie: %w", err)
			} else if isZombie {
				return ErrZombieEdge
			}

			return ErrEdgeNotFound
		} else if err != nil {
			return fmt.Errorf("unable to fetch channel: %w", err)
		}

		edge, policy1, policy2, err = buildChannel(
			ctx, db, s.cfg.ChainHash, dbChan,
		)
		if err != nil {
			return fmt.Errorf("unable to build channel edge: %w", err)
		}

		return nil
	}, func() {})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("could not fetch channel: %w",
			err)
	}

	return edge, policy1, policy2, nil
}

// FetchChannelEdgesByOutpoint attempts to lookup the two directed edges for
// the channel identified by the funding outpoint. If the channel can't be
// found, then ErrEdgeNotFound is returned. A struct which houses the general
// information for the channel itself is returned as well as two structs that
// contain the routing policies for the channel in either direction.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) FetchChannelEdgesByOutpoint(op *wire.OutPoint) (
	*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
	*models.ChannelEdgePolicy, error) {

	var (
		ctx              = context.TODO()
		edge             *models.ChannelEdgeInfo
		policy1, policy2 *models.ChannelEdgePolicy
	)
	err := s.db.ExecTx(ctx, sqldb.ReadTxOpt(), func(db SQLQueries) error {
		dbChan, err := db.GetChannelByOutpoint(
			ctx, sqlc.GetChannelByOutpointParams{
				Outpoint: op.String(),
				Version:  int16(ProtocolV1),
			},
		)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrEdgeNotFound
		} else if err != nil {
			return fmt.Errorf("unable to fetch channel: %w", err)
		}

		edge, policy1, policy2, err = buildChannel(
			ctx, db, s.cfg.ChainHash, dbChan,
		)
		if err != nil {
			return fmt.Errorf("unable to build channel edge: %w", err)
		}

		return nil
	}, func() {})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("could not fetch channel: %w",
			err)
	}

	return edge, policy1, policy2, nil
}

// HasChannelEdge returns true if the database knows of a channel edge with the
// passed channel ID, and false otherwise. If an edge with that ID is found
// within the graph, then two time stamps representing the last time the edge
// was updated for both directed edges are returned along with the boolean. If
// it is not found, then the zombie index is checked and its result is returned
// as the second boolean.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) HasChannelEdge(chanID uint64) (time.Time, time.Time, bool,
	bool, error) {

	ctx := context.TODO()

	var (
		exists          bool
		isZombie        bool
		node1LastUpdate time.Time
		node2LastUpdate time.Time
	)

	// We'll query the cache with the shared lock held to allow multiple
	// readers to access values in the cache concurrently if they exist.
	s.cacheMu.RLock()
	if entry, ok := s.rejectCache.get(chanID); ok {
		s.cacheMu.RUnlock()
		node1LastUpdate = time.Unix(entry.upd1Time, 0)
		node2LastUpdate = time.Unix(entry.upd2Time, 0)
		exists, isZombie = entry.flags.unpack()

		return node1LastUpdate, node2LastUpdate, exists, isZombie, nil
	}
	s.cacheMu.RUnlock()

	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	// The item was not found with the shared lock, so we'll acquire the
	// exclusive lock and check the cache again in case another method added
	// the entry to the cache while no lock was held.
	if entry, ok := s.rejectCache.get(chanID); ok {
		node1LastUpdate = time.Unix(entry.upd1Time, 0)
		node2LastUpdate = time.Unix(entry.upd2Time, 0)
		exists, isZombie = entry.flags.unpack()

		return node1LastUpdate, node2LastUpdate, exists, isZombie, nil
	}

	err := s.db.ExecTx(ctx, sqldb.ReadTxOpt(), func(db SQLQueries) error {
		var chanIDB [8]byte
		byteOrder.PutUint64(chanIDB[:], chanID)

		channel, err := db.GetChannelBySCID(
			ctx, sqlc.GetChannelBySCIDParams{
				Scid:    chanIDB[:],
				Version: int16(ProtocolV1),
			},
		)
		if errors.Is(err, sql.ErrNoRows) {
			// Check if it is a zombie channel.
			isZombie, err = db.IsZombieChannel(
				ctx, sqlc.IsZombieChannelParams{
					Scid:    int64(chanID),
					Version: int16(ProtocolV1),
				},
			)
			if err != nil {
				return fmt.Errorf("could not check if channel "+
					"is zombie: %w", err)
			}

			return nil
		} else if err != nil {
			return fmt.Errorf("unable to fetch channel: %w", err)
		}

		exists = true

		policy1, err := db.GetChannelPolicyByChannelAndNode(
			ctx, sqlc.GetChannelPolicyByChannelAndNodeParams{
				Version:   int16(ProtocolV1),
				ChannelID: channel.ID,
				NodeID:    channel.NodeID1,
			},
		)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("unable to fetch channel policy: %w",
				err)
		} else if err == nil {
			node1LastUpdate = time.Unix(policy1.LastUpdate.Int64, 0)
		}

		policy2, err := db.GetChannelPolicyByChannelAndNode(
			ctx, sqlc.GetChannelPolicyByChannelAndNodeParams{
				Version:   int16(ProtocolV1),
				ChannelID: channel.ID,
				NodeID:    channel.NodeID2,
			},
		)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("unable to fetch channel policy: %w",
				err)
		} else if err == nil {
			node2LastUpdate = time.Unix(policy2.LastUpdate.Int64, 0)
		}

		return nil
	}, func() {})
	if err != nil {
		return time.Time{}, time.Time{}, false, false,
			fmt.Errorf("unable to fetch channel: %w", err)
	}

	s.rejectCache.insert(chanID, rejectCacheEntry{
		upd1Time: node1LastUpdate.Unix(),
		upd2Time: node2LastUpdate.Unix(),
		flags:    packRejectFlags(exists, isZombie),
	})

	return node1LastUpdate, node2LastUpdate, exists, isZombie, nil
}

// ChannelID attempt to lookup the 8-byte compact channel ID which maps to the
// passed channel point (outpoint). If the passed channel doesn't exist within
// the database, then ErrEdgeNotFound is returned.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) ChannelID(chanPoint *wire.OutPoint) (uint64, error) {
	var (
		ctx       = context.TODO()
		channelID uint64
	)
	err := s.db.ExecTx(ctx, sqldb.ReadTxOpt(), func(db SQLQueries) error {
		chanID, err := db.GetSCIDByOutpoint(
			ctx, sqlc.GetSCIDByOutpointParams{
				Outpoint: chanPoint.String(),
				Version:  int16(ProtocolV1),
			},
		)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrEdgeNotFound
		} else if err != nil {
			return fmt.Errorf("unable to fetch channel ID: %w",
				err)
		}

		channelID = byteOrder.Uint64(chanID)

		return nil
	}, func() {})
	if err != nil {
		return 0, fmt.Errorf("unable to fetch channel ID: %w", err)
	}

	return channelID, nil
}

// FilterKnownChanIDs takes a set of channel IDs and return the subset of chan
// ID's that we don't know and are not known zombies of the passed set. In other
// words, we perform a set difference of our set of chan ID's and the ones
// passed in. This method can be used by callers to determine the set of
// channels another peer knows of that we don't. The ChannelUpdateInfos for the
// known zombies is also returned.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) FilterKnownChanIDs(chansInfo []ChannelUpdateInfo) ([]uint64,
	[]ChannelUpdateInfo, error) {

	var (
		ctx          = context.TODO()
		newChanIDs   []uint64
		knownZombies []ChannelUpdateInfo
	)
	err := s.db.ExecTx(ctx, sqldb.ReadTxOpt(), func(db SQLQueries) error {
		for _, chanInfo := range chansInfo {
			channelID := chanInfo.ShortChannelID.ToUint64()
			var chanIDB [8]byte
			byteOrder.PutUint64(chanIDB[:], channelID)

			_, err := db.GetChannelBySCID(
				ctx, sqlc.GetChannelBySCIDParams{
					Version: int16(ProtocolV1),
					Scid:    chanIDB[:],
				},
			)
			if err == nil {
				continue
			} else if !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("unable to fetch channel: %w",
					err)
			}

			isZombie, err := db.IsZombieChannel(
				ctx, sqlc.IsZombieChannelParams{
					Scid:    int64(channelID),
					Version: int16(ProtocolV1),
				},
			)
			if err != nil {
				return fmt.Errorf("unable to fetch zombie "+
					"channel: %w", err)
			}

			if isZombie {
				knownZombies = append(knownZombies, chanInfo)

				continue
			}

			newChanIDs = append(newChanIDs, channelID)
		}

		return nil
	}, func() {})
	if err != nil {
		return nil, nil, fmt.Errorf("unable to fetch channels: %w", err)
	}

	return newChanIDs, knownZombies, nil
}

// FetchChanInfos returns the set of channel edges that correspond to the passed
// channel ID's. If an edge is the query is unknown to the database, it will
// skipped and the result will contain only those edges that exist at the time
// of the query. This can be used to respond to peer queries that are seeking to
// fill in gaps in their view of the channel graph.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) FetchChanInfos(chanIDs []uint64) ([]ChannelEdge, error) {
	var (
		ctx    = context.TODO()
		readTx = NewReadTx()
		edges  []ChannelEdge
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		for _, chanID := range chanIDs {
			var chanIDB [8]byte
			byteOrder.PutUint64(chanIDB[:], chanID)

			dbChan, err := db.GetChannelBySCID(
				ctx, sqlc.GetChannelBySCIDParams{
					Scid:    chanIDB[:],
					Version: int16(ProtocolV1),
				},
			)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			} else if err != nil {
				return fmt.Errorf("unable to fetch channel: %w",
					err)
			}

			channel, p1, p2, err := buildChannel(
				ctx, db, s.cfg.ChainHash, dbChan,
			)
			if err != nil {
				return fmt.Errorf("unable to build channel: %w",
					err)
			}

			node1, err := getNodeByDBID(ctx, db, dbChan.NodeID1)
			if err != nil {
				return fmt.Errorf("unable to fetch "+
					"node(id=%d): %w", dbChan.NodeID1, err)
			}

			node2, err := getNodeByDBID(ctx, db, dbChan.NodeID2)
			if err != nil {
				return fmt.Errorf("unable to fetch "+
					"node(id=%d): %w", dbChan.NodeID2, err)
			}

			edges = append(edges, ChannelEdge{
				Info:    channel,
				Policy1: p1,
				Policy2: p2,
				Node1:   node1,
				Node2:   node2,
			})
		}

		return nil
	}, func() {})
	if err != nil {
		return nil, fmt.Errorf("unable to fetch channels: %w", err)
	}

	return edges, nil
}

// IsPublicNode is a helper method that determines whether the node with the
// given public key is seen as a public node in the graph from the graph's
// source node's point of view.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) IsPublicNode(pubKey [33]byte) (bool, error) {
	ctx := context.TODO()

	var (
		readTx   = NewReadTx()
		isPublic bool
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		var err error
		isPublic, err = db.IsPublicV1Node(ctx, pubKey[:])

		return err
	}, func() {})
	if err != nil {
		return false, fmt.Errorf("unable to check if node is "+
			"public: %w", err)
	}

	return isPublic, nil
}

// DisabledChannelIDs returns the channel ids of disabled channels.
// A channel is disabled when two of the associated ChanelEdgePolicies
// have their disabled bit on.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) DisabledChannelIDs() ([]uint64, error) {
	var (
		ctx     = context.TODO()
		readTx  = NewReadTx()
		chanIDs []uint64
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		dbChanIDs, err := db.GetV1DisabledSCIDs(ctx)
		if err != nil {
			return fmt.Errorf("unable to fetch disabled "+
				"channels: %w", err)
		}

		for _, dbChanID := range dbChanIDs {
			chanIDs = append(chanIDs, byteOrder.Uint64(dbChanID))
		}

		return nil
	}, func() {})
	if err != nil {
		return nil, fmt.Errorf("unable to fetch disabled channels: %w",
			err)
	}

	return chanIDs, nil
}

// PruneGraphNodes is a garbage collection method which attempts to prune out
// any nodes from the channel graph that are currently unconnected. This ensure
// that we only maintain a graph of reachable nodes. In the event that a pruned
// node gains more channels, it will be re-added back to the graph.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) PruneGraphNodes() ([]route.Vertex, error) {
	var (
		ctx     = context.TODO()
		writeTx TxOptions
	)

	var prunedNodes []route.Vertex
	err := s.db.ExecTx(ctx, &writeTx, func(db SQLQueries) error {
		var err error
		prunedNodes, err = pruneGraphNodes(ctx, db)

		return err
	}, func() {})
	if err != nil {
		return nil, fmt.Errorf("unable to prune nodes: %w", err)
	}

	return prunedNodes, nil
}

// PruneGraph prunes newly closed channels from the channel graph in response
// to a new block being solved on the network. Any transactions which spend the
// funding output of any known channels within he graph will be deleted.
// Additionally, the "prune tip", or the last block which has been used to
// prune the graph is stored so callers can ensure the graph is fully in sync
// with the current UTXO state. A slice of channels that have been closed by
// the target block along with any pruned nodes are returned if the function
// succeeds without error.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) PruneGraph(spentOutputs []*wire.OutPoint,
	blockHash *chainhash.Hash, blockHeight uint32) (
	[]*models.ChannelEdgeInfo, []route.Vertex, error) {

	ctx := context.TODO()

	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	var (
		writeTx     TxOptions
		closedChans []*models.ChannelEdgeInfo
		prunedNodes []route.Vertex
	)
	err := s.db.ExecTx(ctx, &writeTx, func(db SQLQueries) error {
		for _, outpoint := range spentOutputs {
			dbChannel, err := db.GetChannelByOutpoint(
				ctx, sqlc.GetChannelByOutpointParams{
					Outpoint: outpoint.String(),
					Version:  int16(ProtocolV1),
				},
			)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			} else if err != nil {
				return fmt.Errorf("unable to fetch channel: %w",
					err)
			}

			info, err := buildChannelInfo(
				ctx, db, s.cfg.ChainHash, dbChannel,
			)
			if err != nil {
				return fmt.Errorf("unable to build channel: %w",
					err)
			}

			closedChans = append(closedChans, info)

			err = db.DeleteChannel(ctx, dbChannel.ID)
			if err != nil {
				return fmt.Errorf("unable to delete "+
					"channel: %w", err)
			}
		}

		err := db.UpsertPruneLogEntry(
			ctx, sqlc.UpsertPruneLogEntryParams{
				BlockHash:   blockHash[:],
				BlockHeight: int64(blockHeight),
			},
		)
		if err != nil {
			return fmt.Errorf("unable to insert prune log "+
				"entry: %w", err)
		}

		// Now that we've pruned some channels, we'll also prune any
		// nodes that no longer have any channels.
		prunedNodes, err = pruneGraphNodes(ctx, db)
		if err != nil {
			return fmt.Errorf("unable to prune graph nodes: %w",
				err)
		}

		return nil
	}, func() {})
	if err != nil {
		return nil, nil, fmt.Errorf("unable to prune graph: %w", err)
	}

	for _, channel := range closedChans {
		s.rejectCache.remove(channel.ChannelID)
		s.chanCache.remove(channel.ChannelID)
	}

	return closedChans, prunedNodes, nil
}

// ChannelView returns the verifiable edge information for each active channel
// within the known channel graph. The set of UTXO's (along with their scripts)
// returned are the ones that need to be watched on chain to detect channel
// closes on the resident blockchain.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) ChannelView() ([]EdgePoint, error) {
	var (
		ctx        = context.TODO()
		readTx     = NewReadTx()
		edgePoints []EdgePoint
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		dbChannel, err := db.ListAllChannels(ctx, int16(ProtocolV1))
		if err != nil {
			return fmt.Errorf("unable to fetch channels: %w", err)
		}

		for _, dbChan := range dbChannel {
			if dbChan.BitcoinKey1 == nil {
				continue
			}

			pkScript, err := genMultiSigP2WSH(
				dbChan.BitcoinKey1, dbChan.BitcoinKey2,
			)
			if err != nil {
				return err
			}

			op, err := wire.NewOutPointFromString(dbChan.Outpoint)
			if err != nil {
				return err
			}

			edgePoints = append(edgePoints, EdgePoint{
				FundingPkScript: pkScript,
				OutPoint:        *op,
			})
		}

		return nil
	}, func() {})
	if err != nil {
		return nil, fmt.Errorf("unable to fetch channel view: %w", err)
	}

	return edgePoints, nil
}

// PruneTip returns the block height and hash of the latest block that has been
// used to prune channels in the graph. Knowing the "prune tip" allows callers
// to tell if the graph is currently in sync with the current best known UTXO
// state.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) PruneTip() (*chainhash.Hash, uint32, error) {
	var (
		ctx       = context.TODO()
		writeTx   = NewReadTx()
		tipHash   chainhash.Hash
		tipHeight uint32
	)
	err := s.db.ExecTx(ctx, &writeTx, func(db SQLQueries) error {
		pruneTip, err := db.GetPruneTip(ctx)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrGraphNeverPruned
		} else if err != nil {
			return fmt.Errorf("unable to fetch prune tip: %w", err)
		}

		tipHash = chainhash.Hash(pruneTip.BlockHash)
		tipHeight = uint32(pruneTip.BlockHeight)

		return nil
	}, func() {})
	if err != nil {
		return nil, 0, err
	}

	return &tipHash, tipHeight, nil
}

// DisconnectBlockAtHeight is used to indicate that the block specified
// by the passed height has been disconnected from the main chain. This
// will "rewind" the graph back to the height below, deleting channels
// that are no longer confirmed from the graph. The prune log will be
// set to the last prune height valid for the remaining chain.
// Channels that were removed from the graph resulting from the
// disconnected block are returned.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) DisconnectBlockAtHeight(height uint32) (
	[]*models.ChannelEdgeInfo, error) {

	ctx := context.TODO()

	var (
		writeTx = TxOptions{}

		// Every channel having a ShortChannelID starting at 'height'
		// will no longer be confirmed.
		startShortChanID = lnwire.ShortChannelID{
			BlockHeight: height,
		}

		// Delete everything after this height from the db up until the
		// SCID alias range.
		endShortChanID = aliasmgr.StartingAlias

		removedChans []*models.ChannelEdgeInfo
	)

	var chanIDStart [8]byte
	byteOrder.PutUint64(chanIDStart[:], startShortChanID.ToUint64())
	var chanIDEnd [8]byte
	byteOrder.PutUint64(chanIDEnd[:], endShortChanID.ToUint64())

	err := s.db.ExecTx(ctx, &writeTx, func(db SQLQueries) error {
		dbChans, err := db.GetChannelsBySCIDRange(
			ctx, sqlc.GetChannelsBySCIDRangeParams{
				StartScid: chanIDStart[:],
				EndScid:   chanIDEnd[:],
			},
		)
		if err != nil {
			return fmt.Errorf("unable to fetch channels: %w", err)
		}

		for _, dbChan := range dbChans {
			channel, err := buildChannelInfo(
				ctx, db, s.cfg.ChainHash, dbChan,
			)
			if err != nil {
				return fmt.Errorf("unable to build channel: %w",
					err)
			}

			removedChans = append(removedChans, channel)

			err = db.DeleteChannel(ctx, dbChan.ID)
			if err != nil {
				return fmt.Errorf("unable to delete "+
					"channel: %w", err)
			}
		}

		return db.DeletePruneLogEntriesInRange(
			ctx, sqlc.DeletePruneLogEntriesInRangeParams{
				StartHeight: int64(height),
				EndHeight:   int64(endShortChanID.BlockHeight),
			},
		)
	}, func() {})
	if err != nil {
		return nil, fmt.Errorf("unable to disconnect block at "+
			"height: %w", err)
	}

	for _, channel := range removedChans {
		s.rejectCache.remove(channel.ChannelID)
		s.chanCache.remove(channel.ChannelID)
	}

	return removedChans, nil
}

// AddEdgeProof sets the proof of an existing edge in the graph database.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) AddEdgeProof(scid lnwire.ShortChannelID,
	proof *models.ChannelAuthProof) error {

	var (
		ctx       = context.TODO()
		writeTx   TxOptions
		scidBytes [8]byte
	)
	byteOrder.PutUint64(scidBytes[:], scid.ToUint64())

	err := s.db.ExecTx(ctx, &writeTx, func(db SQLQueries) error {
		dbChan, err := db.GetChannelBySCID(
			ctx, sqlc.GetChannelBySCIDParams{
				Scid:    scidBytes[:],
				Version: int16(ProtocolV1),
			},
		)
		if err != nil {
			return fmt.Errorf("unable to fetch channel: %w", err)
		}

		return db.AddV1ChannelProof(
			ctx, sqlc.AddV1ChannelProofParams{
				ID:                dbChan.ID,
				Node1Signature:    proof.NodeSig1Bytes,
				Node2Signature:    proof.NodeSig2Bytes,
				Bitcoin1Signature: proof.BitcoinSig1Bytes,
				Bitcoin2Signature: proof.BitcoinSig2Bytes,
			},
		)
	}, func() {})
	if err != nil {
		return fmt.Errorf("unable to add edge proof: %w", err)
	}

	return nil
}

// PutClosedScid stores a SCID for a closed channel in the database. This is so
// that we can ignore channel announcements that we know to be closed without
// having to validate them and fetch a block.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) PutClosedScid(scid lnwire.ShortChannelID) error {
	ctx := context.TODO()

	var writeTx TxOptions
	return s.db.ExecTx(ctx, &writeTx, func(db SQLQueries) error {
		var chanIDB [8]byte
		byteOrder.PutUint64(chanIDB[:], scid.ToUint64())

		return db.InsertClosedChannel(ctx, chanIDB[:])
	}, func() {})
}

// IsClosedScid checks whether a channel identified by the passed in scid is
// closed. This helps avoid having to perform expensive validation checks.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) IsClosedScid(scid lnwire.ShortChannelID) (bool, error) {
	var (
		ctx      = context.TODO()
		readTx   = NewReadTx()
		isClosed bool
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		var chanIDB [8]byte
		byteOrder.PutUint64(chanIDB[:], scid.ToUint64())
		var err error
		isClosed, err = db.IsClosedChannel(ctx, chanIDB[:])
		if err != nil {
			return fmt.Errorf("unable to fetch closed channel: %w",
				err)
		}

		return nil
	}, func() {})
	if err != nil {
		return false, fmt.Errorf("unable to fetch closed channel: %w",
			err)
	}

	return isClosed, nil
}

// GraphSession will provide the call-back with access to a NodeTraverser
// instance which can be used to perform queries against the channel graph.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) GraphSession(cb func(graph NodeTraverser) error) error {
	var (
		ctx    = context.TODO()
		readTx = NewReadTx()
	)

	return s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		return cb(newSQLNodeTraverser(db, s.cfg.ChainHash))
	}, func() {})
}

// sqlNodeTraverser implements the NodeTraverser interface but with a backing
// read only transaction for a consistent view of the graph.
type sqlNodeTraverser struct {
	db    SQLQueries
	chain chainhash.Hash
}

// A compile-time assertion to ensure that sqlNodeTraverser implements the
// NodeTraverser interface.
var _ NodeTraverser = (*sqlNodeTraverser)(nil)

// newSQLNodeTraverser creates a new instance of the sqlNodeTraverser.
func newSQLNodeTraverser(db SQLQueries,
	chain chainhash.Hash) *sqlNodeTraverser {

	return &sqlNodeTraverser{
		db:    db,
		chain: chain,
	}
}

// ForEachNodeDirectedChannel calls the callback for every channel of the given
// node.
//
// NOTE: Part of the NodeTraverser interface.
func (s *sqlNodeTraverser) ForEachNodeDirectedChannel(nodePub route.Vertex,
	cb func(channel *DirectedChannel) error) error {

	ctx := context.TODO()

	return forEachNodeDirectedChannel(ctx, s.db, s.chain, nodePub, cb)
}

// FetchNodeFeatures returns the features of the given node. If the node is
// unknown, assume no additional features are supported.
//
// NOTE: Part of the NodeTraverser interface.
func (s *sqlNodeTraverser) FetchNodeFeatures(nodePub route.Vertex) (
	*lnwire.FeatureVector, error) {

	ctx := context.TODO()

	return fetchNodeFeatures(ctx, s.db, nodePub)
}

// pruneGraphNodes deletes any node in the DB that doesn't have a channel
//
// NOTE: this acts across protocol versions.
func pruneGraphNodes(ctx context.Context, db SQLQueries) ([]route.Vertex,
	error) {

	nodes, err := db.GetUnconnectedNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch unconnected nodes: %w",
			err)
	}

	sourceNodes, err := getSourceNodes(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch source nodes: %w", err)
	}

	prunedNodes := make([]route.Vertex, 0, len(nodes))
	for _, node := range nodes {
		// Don't delete source nodes.
		if _, ok := sourceNodes[node.ID]; ok {
			continue
		}

		if err := db.DeleteNode(ctx, node.ID); err != nil {
			return nil, fmt.Errorf("unable to delete node: %w", err)
		}

		var pubKey route.Vertex
		copy(pubKey[:], node.PubKey)
		prunedNodes = append(prunedNodes, pubKey)
	}

	return prunedNodes, nil
}

// getSourceNodes returns a map of all source nodes in the database.
func getSourceNodes(ctx context.Context, db SQLQueries) (map[int64]struct{},
	error) {

	nodes, err := db.GetSourceNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch source nodes: %w", err)
	}

	m := make(map[int64]struct{}, len(nodes))
	for _, node := range nodes {
		m[node.NodeID] = struct{}{}
	}

	return m, nil
}

func forEachNodeDirectedChannel(ctx context.Context, db SQLQueries,
	chain chainhash.Hash, nodePub route.Vertex,
	cb func(channel *DirectedChannel) error) error {

	// Fallback that uses the database.
	toNodeCallback := func() route.Vertex {
		return nodePub
	}

	dbNode, err := db.GetNodeByPubKey(
		ctx, sqlc.GetNodeByPubKeyParams{
			Version: int16(ProtocolV1),
			PubKey:  nodePub[:],
		},
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return fmt.Errorf("unable to fetch node: %w", err)
	}

	features, err := getNodeFeatures(ctx, db, dbNode.ID)
	if err != nil {
		return fmt.Errorf("unable to fetch node features: %w", err)
	}

	dbChannels, err := db.ListChannelsByNodeID(
		ctx, sqlc.ListChannelsByNodeIDParams{
			Version: int16(ProtocolV1),
			NodeID1: dbNode.ID,
		},
	)
	if err != nil {
		return fmt.Errorf("unable to fetch channels: %w", err)
	}

	for _, dbChannel := range dbChannels {
		e, p1, p2, err := buildChannel(ctx, db, chain, dbChannel)
		if err != nil {
			return fmt.Errorf("unable to build channel: %w",
				err)
		}

		// Determine the outgoing and incoming policy for this
		// channel and node combo.
		outPolicy, inPolicy := p1, p2
		if p1 != nil && p1.ToNode == nodePub {
			outPolicy, inPolicy = p2, p1
		} else if p2 != nil && p2.ToNode != nodePub {
			outPolicy, inPolicy = p2, p1
		}

		var cachedInPolicy *models.CachedEdgePolicy
		if inPolicy != nil {
			cachedInPolicy = models.NewCachedPolicy(inPolicy)
			cachedInPolicy.ToNodePubKey = toNodeCallback
			cachedInPolicy.ToNodeFeatures = features
		}

		var inboundFee lnwire.Fee
		if outPolicy != nil {
			// Extract inbound fee. If there is a decoding
			// error, skip this edge.
			_, err := outPolicy.ExtraOpaqueData.
				ExtractRecords(&inboundFee)
			if err != nil {
				return nil
			}
		}

		directedChannel := &DirectedChannel{
			ChannelID:    e.ChannelID,
			IsNode1:      nodePub == e.NodeKey1Bytes,
			OtherNode:    e.NodeKey2Bytes,
			Capacity:     e.Capacity,
			OutPolicySet: outPolicy != nil,
			InPolicy:     cachedInPolicy,
			InboundFee:   inboundFee,
		}

		if nodePub == e.NodeKey2Bytes {
			directedChannel.OtherNode = e.NodeKey1Bytes
		}

		if err := cb(directedChannel); err != nil {
			return err
		}
	}

	return nil
}

// forEachNodeChannel iterates through all channels of a node, executing
// the passed callback on each. The call-back is provided with the channel's
// edge information, the outgoing policy and the incoming policy for the
// channel and node combo.
func forEachNodeChannel(ctx context.Context, db SQLQueries,
	chain chainhash.Hash, id int64, cb func(*models.ChannelEdgeInfo,
		*models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy) error) error {

	// Get all the V1 channels for this node.
	dbChannels, err := db.ListChannelsByNodeID(
		ctx, sqlc.ListChannelsByNodeIDParams{
			Version: int16(ProtocolV1),
			NodeID1: id,
		},
	)
	if err != nil {
		return fmt.Errorf("unable to fetch channels: %w", err)
	}

	// Call the call-back for each channel and its known policies.
	for _, dbChannel := range dbChannels {
		e, p1, p2, err := buildChannel(ctx, db, chain, dbChannel)
		if err != nil {
			return fmt.Errorf("unable to build channel: %w", err)
		}

		// Determine the outgoing and incoming policy for this
		// channel and node combo.
		p1ToNode := dbChannel.NodeID2
		p2ToNode := dbChannel.NodeID1
		outPolicy, inPolicy := p1, p2
		if (p1 != nil && p1ToNode == id) ||
			(p2 != nil && p2ToNode != id) {

			outPolicy, inPolicy = p2, p1
		}

		if err := cb(e, outPolicy, inPolicy); err != nil {
			return err
		}
	}

	return nil
}

func forEachNode(ctx context.Context, db SQLQueries,
	cb func(nodeID int64, nodePub route.Vertex) error) error {

	nodes, err := db.ListNodeIDsAndPubKeys(ctx, int16(ProtocolV1))
	if err != nil {
		return fmt.Errorf("unable to fetch nodes: %w", err)
	}

	for _, node := range nodes {
		var pub route.Vertex
		copy(pub[:], node.PubKey)

		if err := cb(node.ID, pub); err != nil {
			return fmt.Errorf("callback failed: %w", err)
		}
	}

	return nil
}

func buildChannel(ctx context.Context, db SQLQueries,
	chain chainhash.Hash, dbChan sqlc.Channel) (*models.ChannelEdgeInfo,
	*models.ChannelEdgePolicy, *models.ChannelEdgePolicy, error) {

	edgeInfo, err := buildChannelInfo(ctx, db, chain, dbChan)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to build channel "+
			"info: %w", err)
	}

	node1Policy, err := getAndBuildChanPolicy(
		ctx, db, byteOrder.Uint64(dbChan.Scid), dbChan.ID,
		dbChan.NodeID1, edgeInfo.NodeKey2Bytes, true,
	)
	if err != nil {
		return nil, nil, nil, err
	}

	node2Policy, err := getAndBuildChanPolicy(
		ctx, db, byteOrder.Uint64(dbChan.Scid), dbChan.ID,
		dbChan.NodeID2, edgeInfo.NodeKey1Bytes, false,
	)
	if err != nil {
		return nil, nil, nil, err
	}

	return edgeInfo, node1Policy, node2Policy, nil
}

func buildChannelInfo(ctx context.Context, db SQLQueries,
	chain chainhash.Hash, dbChan sqlc.Channel) (*models.ChannelEdgeInfo,
	error) {

	op, err := wire.NewOutPointFromString(dbChan.Outpoint)
	if err != nil {
		return nil, err
	}

	node1Vertex, node2Vertex, err := getChannelNodes(ctx, db, dbChan)
	if err != nil {
		return nil, err
	}

	features, err := getChanFeatures(ctx, db, dbChan.ID)
	if err != nil {
		return nil, err
	}

	var featureBuf bytes.Buffer
	if err := features.Encode(&featureBuf); err != nil {
		return nil, fmt.Errorf("unable to encode features: %w", err)
	}

	extraTypes, err := getChannelExtraSignedFields(ctx, db, dbChan.ID)
	if err != nil {
		return nil, err
	}

	recs, err := lnwire.CustomRecords(extraTypes).Serialize()
	if err != nil {
		return nil, fmt.Errorf("unable to serialize extra signed "+
			"fields: %w", err)
	}
	if recs == nil {
		recs = make([]byte, 0)
	}

	var btcKey1, btcKey2 route.Vertex
	copy(btcKey1[:], dbChan.BitcoinKey1)
	copy(btcKey2[:], dbChan.BitcoinKey2)

	channel := &models.ChannelEdgeInfo{
		ChainHash:        chain,
		ChannelID:        byteOrder.Uint64(dbChan.Scid),
		NodeKey1Bytes:    node1Vertex,
		NodeKey2Bytes:    node2Vertex,
		BitcoinKey1Bytes: btcKey1,
		BitcoinKey2Bytes: btcKey2,
		ChannelPoint:     *op,
		Capacity:         btcutil.Amount(dbChan.Capacity.Int64),
		Features:         featureBuf.Bytes(),
		ExtraOpaqueData:  recs,
	}

	if dbChan.Bitcoin1Signature != nil {
		channel.AuthProof = &models.ChannelAuthProof{
			NodeSig1Bytes:    dbChan.Node1Signature,
			NodeSig2Bytes:    dbChan.Node2Signature,
			BitcoinSig1Bytes: dbChan.Bitcoin1Signature,
			BitcoinSig2Bytes: dbChan.Bitcoin2Signature,
		}
	}

	return channel, nil
}

func getChannelNodes(ctx context.Context, db SQLQueries,
	dbChan sqlc.Channel) (route.Vertex, route.Vertex, error) {

	var node1Vertex, node2Vertex route.Vertex

	node1, err := db.GetNode(ctx, dbChan.NodeID1)
	if err != nil {
		return node1Vertex, node2Vertex, fmt.Errorf("unable to "+
			"fetch node(%d) pub key: %w", dbChan.NodeID1, err)
	}
	node1Vertex, err = route.NewVertexFromBytes(node1.PubKey)
	if err != nil {
		return node1Vertex, node2Vertex, err
	}

	node2, err := db.GetNode(ctx, dbChan.NodeID2)
	if err != nil {
		return node1Vertex, node2Vertex, fmt.Errorf("unable to "+
			"fetch node(%d) pub key: %w", dbChan.NodeID2, err)
	}
	node2Vertex, err = route.NewVertexFromBytes(node2.PubKey)
	if err != nil {
		return node1Vertex, node2Vertex, err
	}

	return node1Vertex, node2Vertex, nil
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
		features.Set(lnwire.FeatureBit(feature.FeatureBit))
	}

	return features, nil
}

func getChannelExtraSignedFields(ctx context.Context, db SQLQueries,
	channelID int64) (map[uint64][]byte, error) {

	fields, err := db.GetExtraChannelTypes(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("unable to get channel(%d) extra "+
			"signed fields: %w", channelID, err)
	}

	extraFields := make(map[uint64][]byte)
	for _, field := range fields {
		extraFields[uint64(field.Type)] = field.Value
	}

	return extraFields, nil
}

// getAndBuildChanPolicy retrieves the channel policy for a given channel and
// node, and builds a ChannelEdgePolicy model from it. If the policy does not
// exist, it returns nil without an error.
func getAndBuildChanPolicy(ctx context.Context, db SQLQueries, channelID uint64,
	dbChanID, dbNodeID int64, toNode route.Vertex,
	isNode1 bool) (*models.ChannelEdgePolicy, error) {

	// Fetch the channel policy for the given channel and node. We might
	// not have such a policy yet. In that case, we return nil.
	policy, err := db.GetChannelPolicyByChannelAndNode(
		ctx, sqlc.GetChannelPolicyByChannelAndNodeParams{
			Version:   int16(ProtocolV1),
			ChannelID: dbChanID,
			NodeID:    dbNodeID,
		},
	)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("unable to fetch channel policy: %w",
			err)
	} else if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	extra, err := getChanPolicyExtraSignedFields(ctx, db, policy.ID)
	if err != nil {
		return nil, err
	}

	recs, err := lnwire.CustomRecords(extra).Serialize()
	if err != nil {
		return nil, fmt.Errorf("unable to serialize extra signed "+
			"fields: %w", err)
	}

	var msgFlags lnwire.ChanUpdateMsgFlags
	if policy.MaxHtlcMsat.Valid {
		msgFlags |= lnwire.ChanUpdateRequiredMaxHtlc
	}

	var chanFlags lnwire.ChanUpdateChanFlags
	if !isNode1 {
		chanFlags |= lnwire.ChanUpdateDirection
	}
	if policy.Disabled.Bool {
		chanFlags |= lnwire.ChanUpdateDisabled
	}

	return &models.ChannelEdgePolicy{
		SigBytes:                  policy.Signature,
		ChannelID:                 channelID,
		LastUpdate:                time.Unix(policy.LastUpdate.Int64, 0),
		MessageFlags:              msgFlags,
		ChannelFlags:              chanFlags,
		TimeLockDelta:             uint16(policy.Timelock),
		MinHTLC:                   lnwire.MilliSatoshi(policy.MinHtlcMsat),
		MaxHTLC:                   lnwire.MilliSatoshi(policy.MaxHtlcMsat.Int64),
		FeeBaseMSat:               lnwire.MilliSatoshi(policy.BaseFeeMsat),
		FeeProportionalMillionths: lnwire.MilliSatoshi(policy.FeePpm),
		ToNode:                    toNode,
		ExtraOpaqueData:           recs,
	}, nil
}

func getChanPolicyExtraSignedFields(ctx context.Context, db SQLQueries,
	nodeID int64) (map[uint64][]byte, error) {

	fields, err := db.GetChannelPolicyExtraTypes(ctx, nodeID)
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

// updateChanEdgePolicy upserts the channel policy info we have stored for
// a channel we already know of.
func updateChanEdgePolicy(ctx context.Context, tx SQLQueries,
	edge *models.ChannelEdgePolicy) (route.Vertex, route.Vertex, bool,
	error) {

	var (
		node1Pub, node2Pub route.Vertex
		isNode1            bool
	)

	var chanIDB [8]byte
	byteOrder.PutUint64(chanIDB[:], edge.ChannelID)

	// Check that this edge policy refers to a channel that we already
	// know of. We also need to get the DB level ID of the channel since
	// we'll need it to insert the edge policy.
	dbChan, err := tx.GetChannelAndNodesBySCID(
		ctx, sqlc.GetChannelAndNodesBySCIDParams{
			Scid:    chanIDB[:],
			Version: int16(ProtocolV1),
		},
	)
	if errors.Is(err, sql.ErrNoRows) {
		return node1Pub, node2Pub, false, ErrEdgeNotFound
	} else if err != nil {
		return node1Pub, node2Pub, false, fmt.Errorf("unable to "+
			"fetch channel(%v): %w", edge.ChannelID, err)
	}

	copy(node1Pub[:], dbChan.Node1PubKey)
	copy(node2Pub[:], dbChan.Node2PubKey)

	// Figure out which node this edge is from.
	isNode1 = edge.ChannelFlags&lnwire.ChanUpdateDirection == 0
	nodeID := dbChan.NodeID1
	if !isNode1 {
		nodeID = dbChan.NodeID2
	}

	id, err := tx.UpsertEdgePolicy(ctx, sqlc.UpsertEdgePolicyParams{
		Version:     int16(ProtocolV1),
		ChannelID:   dbChan.ID,
		NodeID:      nodeID,
		Timelock:    int32(edge.TimeLockDelta),
		FeePpm:      int64(edge.FeeProportionalMillionths),
		BaseFeeMsat: int64(edge.FeeBaseMSat),
		MinHtlcMsat: int64(edge.MinHTLC),
		LastUpdate:  sqldb.SQLInt64(edge.LastUpdate.Unix()),
		Disabled: sql.NullBool{
			Valid: true,
			Bool:  edge.IsDisabled(),
		},
		MaxHtlcMsat: sql.NullInt64{
			Valid: edge.MessageFlags.HasMaxHtlc(),
			Int64: int64(edge.MaxHTLC),
		},
		Signature: edge.SigBytes,
	})
	if err != nil {
		return node1Pub, node2Pub, isNode1,
			fmt.Errorf("unable to upsert edge policy: %w", err)
	}

	// Convert the flat extra opaque data into a map of TLV types to
	// values.
	extra, err := marshalExtraOpaqueData(edge.ExtraOpaqueData)
	if err != nil {
		return node1Pub, node2Pub, false, fmt.Errorf("unable to "+
			"marshal extra opaque data: %w", err)
	}

	// Update the channel policy's extra signed fields.
	err = upsertChanPolicyExtraSignedFields(ctx, tx, id, extra)
	if err != nil {
		return node1Pub, node2Pub, false, fmt.Errorf("inserting chan "+
			"policy extra TLVs: %w", err)
	}

	return node1Pub, node2Pub, isNode1, nil
}

// updateEdgeCache updates our reject and channel caches with the new
// edge policy information.
func (s *SQLStore) updateEdgeCache(e *models.ChannelEdgePolicy,
	isUpdate1 bool) {

	// If an entry for this channel is found in reject cache, we'll modify
	// the entry with the updated timestamp for the direction that was just
	// written. If the edge doesn't exist, we'll load the cache entry lazily
	// during the next query for this edge.
	if entry, ok := s.rejectCache.get(e.ChannelID); ok {
		if isUpdate1 {
			entry.upd1Time = e.LastUpdate.Unix()
		} else {
			entry.upd2Time = e.LastUpdate.Unix()
		}
		s.rejectCache.insert(e.ChannelID, entry)
	}

	// If an entry for this channel is found in channel cache, we'll modify
	// the entry with the updated policy for the direction that was just
	// written. If the edge doesn't exist, we'll defer loading the info and
	// policies and lazily read from disk during the next query.
	if channel, ok := s.chanCache.get(e.ChannelID); ok {
		if isUpdate1 {
			channel.Policy1 = e
		} else {
			channel.Policy2 = e
		}
		s.chanCache.insert(e.ChannelID, channel)
	}
}

// getNodeByPubKey attempts to look up a target node by its public key.
func getNodeByPubKey(ctx context.Context, db SQLQueries,
	pubKey route.Vertex) (int64, *models.LightningNode, error) {

	dbNode, err := db.GetNodeByPubKey(
		ctx, sqlc.GetNodeByPubKeyParams{
			Version: int16(ProtocolV1),
			PubKey:  pubKey[:],
		},
	)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil, ErrGraphNodeNotFound
	} else if err != nil {
		return 0, nil, fmt.Errorf("unable to fetch node: %w", err)
	}

	node, err := buildNode(ctx, db, &dbNode)
	if err != nil {
		return 0, nil, fmt.Errorf("unable to build node: %w", err)
	}

	return dbNode.ID, node, nil
}

func getNodeByDBID(ctx context.Context, db SQLQueries,
	id int64) (*models.LightningNode, error) {

	dbNode, err := db.GetNode(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("unable to get node(id=%d): %w", id, err)
	}

	return buildNode(ctx, db, &dbNode)
}

// buildNode constructs a LightningNode instance from the given database node
// record. The node's features, addresses and extra signed fields are also
// fetched from the database and set on the node.
func buildNode(ctx context.Context, db SQLQueries, dbNode *sqlc.Node) (
	*models.LightningNode, error) {

	if dbNode.Version != int16(ProtocolV1) {
		return nil, fmt.Errorf("unsupported node version: %d",
			dbNode.Version)
	}

	var pub [33]byte
	copy(pub[:], dbNode.PubKey)

	node := &models.LightningNode{
		PubKeyBytes: pub,
		Features:    lnwire.EmptyFeatureVector(),
		LastUpdate:  time.Unix(0, 0),
	}

	if len(dbNode.Signature) == 0 {
		return node, nil
	}

	node.HaveNodeAnnouncement = true
	node.AuthSigBytes = dbNode.Signature
	node.Alias = dbNode.Alias.String
	node.LastUpdate = time.Unix(dbNode.LastUpdate.Int64, 0)

	var err error
	node.Color, err = DecodeHexColor(dbNode.Color.String)
	if err != nil {
		return nil, fmt.Errorf("unable to decode color: %w", err)
	}

	// Fetch the node's features.
	node.Features, err = getNodeFeatures(ctx, db, dbNode.ID)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch node(%d) "+
			"features: %w", dbNode.ID, err)
	}

	// Fetch the node's addresses.
	_, node.Addresses, err = getNodeAddresses(ctx, db, pub[:])
	if err != nil {
		return nil, fmt.Errorf("unable to fetch node(%d) "+
			"addresses: %w", dbNode.ID, err)
	}

	// Fetch the node's extra signed fields.
	extraTLVMap, err := getNodeExtraSignedFields(ctx, db, dbNode.ID)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch node(%d) "+
			"extra signed fields: %w", dbNode.ID, err)
	}

	recs, err := lnwire.CustomRecords(extraTLVMap).Serialize()
	if err != nil {
		return nil, fmt.Errorf("unable to serialize extra signed "+
			"fields: %w", err)
	}

	if len(recs) != 0 {
		node.ExtraOpaqueData = recs
	}

	return node, nil
}

// getNodeFeatures fetches the feature bits and constructs the feature vector
// for a node with the given DB ID.
func getNodeFeatures(ctx context.Context, db SQLQueries,
	nodeID int64) (*lnwire.FeatureVector, error) {

	rows, err := db.GetNodeFeatures(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("unable to get node(%d) features: %w",
			nodeID, err)
	}

	features := lnwire.EmptyFeatureVector()
	for _, feature := range rows {
		features.Set(lnwire.FeatureBit(feature.FeatureBit))
	}

	return features, nil
}

// getNodeExtraSignedFields fetches the extra signed fields for a node with the
// given DB ID.
func getNodeExtraSignedFields(ctx context.Context, db SQLQueries,
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

// upsertNode upserts the node record into the database. If the node already
// exists, then the node's information is updated. If the node doesn't exist,
// then a new node is created. The node's features, addresses and extra TLV
// types are also updated. The node's DB ID is returned.
func upsertNode(ctx context.Context, db SQLQueries,
	node *models.LightningNode) (int64, error) {

	params := sqlc.UpsertNodeParams{
		Version: int16(ProtocolV1),
		PubKey:  node.PubKeyBytes[:],
	}

	if node.HaveNodeAnnouncement {
		params.LastUpdate = sqldb.SQLInt64(node.LastUpdate.Unix())
		params.Color = sqldb.SQLStr(EncodeHexColor(node.Color))
		params.Alias = sqldb.SQLStr(node.Alias)
		params.Signature = node.AuthSigBytes
	}

	nodeID, err := db.UpsertNode(ctx, params)
	if err != nil {
		return 0, fmt.Errorf("upserting node(%x): %w", node.PubKeyBytes,
			err)
	}

	// We can exit here if we don't have the announcement yet.
	if !node.HaveNodeAnnouncement {
		return nodeID, nil
	}

	// Update the node's features.
	err = upsertNodeFeatures(ctx, db, nodeID, node.Features)
	if err != nil {
		return 0, fmt.Errorf("inserting node features: %w", err)
	}

	// Update the node's addresses.
	err = upsertNodeAddresses(ctx, db, nodeID, node.Addresses)
	if err != nil {
		return 0, fmt.Errorf("inserting node addresses: %w", err)
	}

	// Convert the flat extra opaque data into a map of TLV types to
	// values.
	extra, err := marshalExtraOpaqueData(node.ExtraOpaqueData)
	if err != nil {
		return 0, fmt.Errorf("unable to marshal extra opaque data: %w",
			err)
	}

	// Update the node's extra signed fields.
	err = upsertNodeExtraSignedFields(ctx, db, nodeID, extra)
	if err != nil {
		return 0, fmt.Errorf("inserting node extra TLVs: %w", err)
	}

	return nodeID, nil
}

// upsertNodeFeatures updates the node's features node_features table. This
// includes deleting any feature bits no longer present and inserting any new
// feature bits. If the feature bit does not yet exist in the features table,
// then an entry is created in that table first.
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
		if _, ok := newFeatures[feature.FeatureBit]; ok {
			delete(newFeatures, feature.FeatureBit)
			continue
		}

		// The feature is no longer present, so we remove it from the
		// database.
		err := db.DeleteNodeFeature(ctx, sqlc.DeleteNodeFeatureParams{
			NodeID:     nodeID,
			FeatureBit: feature.FeatureBit,
		})
		if err != nil {
			return fmt.Errorf("unable to delete node(%d) "+
				"feature(%v): %w", nodeID, feature.FeatureBit,
				err)
		}
	}

	// Any remaining entries in newFeatures are new features that need to be
	// added to the database for the first time.
	for feature := range newFeatures {
		err = db.InsertNodeFeature(ctx, sqlc.InsertNodeFeatureParams{
			NodeID:     nodeID,
			FeatureBit: feature,
		})
		if err != nil {
			return fmt.Errorf("unable to insert node(%d) "+
				"feature(%v): %w", nodeID, feature, err)
		}
	}

	return nil
}

// fetchNodeFeatures fetches the features for a node with the given public key.
func fetchNodeFeatures(ctx context.Context, queries SQLQueries,
	nodePub route.Vertex) (*lnwire.FeatureVector, error) {

	rows, err := queries.GetNodeFeaturesByPubKey(
		ctx, sqlc.GetNodeFeaturesByPubKeyParams{
			PubKey:  nodePub[:],
			Version: int16(ProtocolV1),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("unable to get node(%s) features: %w",
			nodePub, err)
	}

	features := lnwire.EmptyFeatureVector()
	for _, bit := range rows {
		features.Set(lnwire.FeatureBit(bit))
	}

	return features, nil
}

// dbAddressType is an enum type that represents the different address types
// that we store in the node_addresses table. The address type determines how
// the address is to be serialised/deserialize.
type dbAddressType uint8

const (
	addressTypeIPv4   dbAddressType = 1
	addressTypeIPv6   dbAddressType = 2
	addressTypeTorV2  dbAddressType = 3
	addressTypeTorV3  dbAddressType = 4
	addressTypeOpaque dbAddressType = math.MaxInt8
)

// upsertNodeAddresses updates the node's addresses in the database. This
// includes deleting any existing addresses and inserting the new set of
// addresses. The deletion is necessary since the ordering of the addresses may
// change, and we need to ensure that the database reflects the latest set of
// addresses so that at the time of reconstructing the node announcement, the
// order is preserved and the signature over the message remains valid.
func upsertNodeAddresses(ctx context.Context, db SQLQueries, nodeID int64,
	addresses []net.Addr) error {

	// Delete any existing addresses for the node. This is required since
	// even if the new set of addresses is the same, the ordering may have
	// changed for a given address type.
	err := db.DeleteNodeAddresses(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("unable to delete node(%d) addresses: %w",
			nodeID, err)
	}

	// Copy the nodes latest set of addresses.
	newAddresses := map[dbAddressType][]string{
		addressTypeIPv4:   {},
		addressTypeIPv6:   {},
		addressTypeTorV2:  {},
		addressTypeTorV3:  {},
		addressTypeOpaque: {},
	}
	addAddr := func(t dbAddressType, addr net.Addr) {
		newAddresses[t] = append(newAddresses[t], addr.String())
	}

	for _, address := range addresses {
		switch addr := address.(type) {
		case *net.TCPAddr:
			if ip4 := addr.IP.To4(); ip4 != nil {
				addAddr(addressTypeIPv4, addr)
			} else if ip6 := addr.IP.To16(); ip6 != nil {
				addAddr(addressTypeIPv6, addr)
			} else {
				return fmt.Errorf("unhandled IP address: %v",
					addr)
			}

		case *tor.OnionAddr:
			switch len(addr.OnionService) {
			case tor.V2Len:
				addAddr(addressTypeTorV2, addr)
			case tor.V3Len:
				addAddr(addressTypeTorV3, addr)
			default:
				return fmt.Errorf("invalid length for a tor " +
					"address")
			}

		case *lnwire.OpaqueAddrs:
			addAddr(addressTypeOpaque, addr)

		default:
			return fmt.Errorf("unhandled address type: %T", addr)
		}
	}

	// Any remaining entries in newAddresses are new addresses that need to
	// be added to the database for the first time.
	for addrType, addrList := range newAddresses {
		for position, addr := range addrList {
			err := db.InsertNodeAddress(
				ctx, sqlc.InsertNodeAddressParams{
					NodeID:   nodeID,
					Type:     int16(addrType),
					Address:  addr,
					Position: int32(position),
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

// getNodeAddresses fetches the addresses for a node with the given public key.
func getNodeAddresses(ctx context.Context, db SQLQueries, nodePub []byte) (bool,
	[]net.Addr, error) {

	// GetNodeAddressesByPubKey ensures that the addresses for a given type
	// are returned in the same order as they were inserted.
	rows, err := db.GetNodeAddressesByPubKey(
		ctx, sqlc.GetNodeAddressesByPubKeyParams{
			Version: int16(ProtocolV1),
			PubKey:  nodePub,
		},
	)
	if err != nil {
		return false, nil, err
	}

	// GetNodeAddressesByPubKey uses a left join so there should always be
	// at least one row returned if the node exists even if it has no
	// addresses.
	if len(rows) == 0 {
		return false, nil, nil
	}

	addresses := make([]net.Addr, 0, len(rows))
	for _, addr := range rows {
		if !(addr.Type.Valid && addr.Address.Valid) {
			continue
		}

		address := addr.Address.String

		switch dbAddressType(addr.Type.Int16) {
		case addressTypeIPv4:
			tcp, err := net.ResolveTCPAddr("tcp4", address)
			if err != nil {
				return false, nil, nil
			}
			tcp.IP = tcp.IP.To4()

			addresses = append(addresses, tcp)

		case addressTypeIPv6:
			tcp, err := net.ResolveTCPAddr("tcp6", address)
			if err != nil {
				return false, nil, nil
			}
			addresses = append(addresses, tcp)

		case addressTypeTorV3, addressTypeTorV2:
			service, portStr, err := net.SplitHostPort(address)
			if err != nil {
				return false, nil, fmt.Errorf("unable to "+
					"split tor v3 address: %v",
					addr.Address)
			}

			port, err := strconv.Atoi(portStr)
			if err != nil {
				return false, nil, err
			}

			addresses = append(addresses, &tor.OnionAddr{
				OnionService: service,
				Port:         port,
			})

		case addressTypeOpaque:
			opaque, err := hex.DecodeString(address)
			if err != nil {
				return false, nil, fmt.Errorf("unable to "+
					"decode opaque address: %v", addr)
			}

			addresses = append(addresses, &lnwire.OpaqueAddrs{
				Payload: opaque,
			})

		default:
			return false, nil, fmt.Errorf("unknown address "+
				"type: %v", addr.Type)
		}
	}

	return true, addresses, nil
}

// upsertNodeExtraSignedFields updates the node's extra signed fields in the
// database. This includes updating any existing types, inserting any new types,
// and deleting any types that are no longer present.
func upsertNodeExtraSignedFields(ctx context.Context, db SQLQueries,
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

// upsertChanPolicyExtraSignedFields updates the policy's extra signed fields in
// the database. This includes updating any existing types, inserting any new
// types, and deleting any types that are no longer present.
func upsertChanPolicyExtraSignedFields(ctx context.Context, db SQLQueries,
	chanPolicyID int64, extraFields map[uint64][]byte) error {

	// Get any existing extra signed fields for the channel policy.
	existingFields, err := db.GetChannelPolicyExtraTypes(ctx, chanPolicyID)
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
		err = db.UpsertChanPolicyExtraType(
			ctx, sqlc.UpsertChanPolicyExtraTypeParams{
				ChannelPolicyID: chanPolicyID,
				Type:            int64(tlvType),
				Value:           value,
			},
		)
		if err != nil {
			return fmt.Errorf("unable to upsert "+
				"channel_policy(%d) extra signed field(%v): %w",
				chanPolicyID, tlvType, err)
		}

		// Remove the field from the map of existing fields if it was
		// present.
		delete(m, tlvType)
	}

	// For all the fields that are left in the map of existing fields, we'll
	// delete them as they are no longer present in the new set of fields.
	for tlvType := range m {
		err = db.DeleteChannelPolicyExtraType(
			ctx, sqlc.DeleteChannelPolicyExtraTypeParams{
				ChannelPolicyID: chanPolicyID,
				Type:            int64(tlvType),
			},
		)
		if err != nil {
			return fmt.Errorf("unable to delete "+
				"channel_policy(%d) extra signed field(%v): %w",
				chanPolicyID, tlvType, err)
		}
	}

	return nil
}

// getSourceNode returns the DB node ID and pub key of the source node for the
// specified protocol version.
func (s *SQLStore) getSourceNode(ctx context.Context, db SQLQueries,
	version ProtocolVersion) (int64, route.Vertex, error) {

	s.srcNodeMu.Lock()
	defer s.srcNodeMu.Unlock()

	// If we already have the source node ID and pub key cached, then
	// return them.
	if s.srcNodeID != 0 {
		return s.srcNodeID, s.srcNodePub, nil
	}

	var pubKey route.Vertex

	nodes, err := db.GetSourceNodesByVersion(ctx, int16(version))
	if err != nil {
		return 0, pubKey, fmt.Errorf("unable to fetch source node: %w",
			err)
	}

	if len(nodes) == 0 {
		return 0, pubKey, ErrSourceNodeNotSet
	} else if len(nodes) > 1 {
		return 0, pubKey, fmt.Errorf("multiple source nodes for "+
			"protocol %s found", version)
	}

	copy(pubKey[:], nodes[0].PubKey)

	s.srcNodeID = nodes[0].NodeID
	s.srcNodePub = pubKey

	return nodes[0].NodeID, pubKey, nil
}

// marshalExtraOpaqueData takes a flat byte slice parses it as a TLV stream.
// This then produces a map from TLV type to value. If the input is not a
// valid TLV stream, then an error is returned.
func marshalExtraOpaqueData(data []byte) (map[uint64][]byte, error) {
	r := bytes.NewReader(data)

	tlvStream, err := tlv.NewStream()
	if err != nil {
		return nil, err
	}

	// Since ExtraOpaqueData is provided by a potentially malicious peer,
	// pass it into the P2P decoding variant.
	parsedTypes, err := tlvStream.DecodeWithParsedTypesP2P(r)
	if err != nil {
		return nil, err
	}
	if len(parsedTypes) == 0 {
		return nil, nil
	}

	records := make(map[uint64][]byte)
	for k, v := range parsedTypes {
		records[uint64(k)] = v
	}

	return records, nil
}

// insertChannel inserts a new channel record into the database.
func insertChannel(ctx context.Context, db SQLQueries,
	edge *models.ChannelEdgeInfo) error {

	var chanIDB [8]byte
	byteOrder.PutUint64(chanIDB[:], edge.ChannelID)

	// Make sure that the channel doesn't already exist. We do this
	// explicitly instead of relying on catching a unique constraint error
	// because relying on SQL to throw that error would abort the entire
	// batch of transactions.
	_, err := db.GetChannelBySCID(
		ctx, sqlc.GetChannelBySCIDParams{
			Scid:    chanIDB[:],
			Version: int16(ProtocolV1),
		},
	)
	if err == nil {
		return ErrEdgeAlreadyExist
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("unable to fetch channel: %w", err)
	}

	// Make sure that at least a "shell" entry for each node is present in
	// the nodes table.
	node1DBID, err := maybeCreateShellNode(ctx, db, edge.NodeKey1Bytes)
	if err != nil {
		return fmt.Errorf("unable to create shell node: %w", err)
	}

	node2DBID, err := maybeCreateShellNode(ctx, db, edge.NodeKey2Bytes)
	if err != nil {
		return fmt.Errorf("unable to create shell node: %w", err)
	}

	var capacity sql.NullInt64
	if edge.Capacity != 0 {
		capacity = sqldb.SQLInt64(int64(edge.Capacity))
	}

	createParams := sqlc.CreateChannelParams{
		Version:     int16(ProtocolV1),
		Scid:        chanIDB[:],
		NodeID1:     node1DBID,
		NodeID2:     node2DBID,
		Outpoint:    edge.ChannelPoint.String(),
		Capacity:    capacity,
		BitcoinKey1: edge.BitcoinKey1Bytes[:],
		BitcoinKey2: edge.BitcoinKey2Bytes[:],
	}

	if edge.AuthProof != nil {
		proof := edge.AuthProof

		createParams.Node1Signature = proof.NodeSig1Bytes
		createParams.Node2Signature = proof.NodeSig2Bytes
		createParams.Bitcoin1Signature = proof.BitcoinSig1Bytes
		createParams.Bitcoin2Signature = proof.BitcoinSig2Bytes
	}

	// Insert the new channel record.
	dbChanID, err := db.CreateChannel(ctx, createParams)
	if err != nil {
		return err
	}

	// Insert any channel features.
	if len(edge.Features) != 0 {
		chanFeatures := lnwire.NewRawFeatureVector()
		err := chanFeatures.Decode(bytes.NewReader(edge.Features))
		if err != nil {
			return err
		}

		fv := lnwire.NewFeatureVector(chanFeatures, lnwire.Features)
		for feature := range fv.Features() {
			err = db.InsertChannelFeature(
				ctx, sqlc.InsertChannelFeatureParams{
					ChannelID:  dbChanID,
					FeatureBit: int32(feature),
				},
			)
			if err != nil {
				return fmt.Errorf("unable to insert "+
					"channel(%d) feature(%v): %w", dbChanID,
					feature, err)
			}
		}
	}

	// Finally, insert any extra TLV fields in the channel announcement.
	extra, err := marshalExtraOpaqueData(edge.ExtraOpaqueData)
	if err != nil {
		return fmt.Errorf("unable to marshal extra opaque data: %w",
			err)
	}

	for tlvType, value := range extra {
		err := db.CreateChannelExtraType(
			ctx, sqlc.CreateChannelExtraTypeParams{
				ChannelID: dbChanID,
				Type:      int64(tlvType),
				Value:     value,
			},
		)
		if err != nil {
			return fmt.Errorf("unable to upsert channel(%d) extra "+
				"signed field(%v): %w", edge.ChannelID,
				tlvType, err)
		}
	}

	return nil
}

// maybeCreateShellNode checks if a shell node entry exists for the
// given public key. If it does not exist, then a new shell node entry is
// created. The ID of the node is returned. A shell node only has a protocol
// version and public key persisted.
func maybeCreateShellNode(ctx context.Context, db SQLQueries,
	pubKey route.Vertex) (int64, error) {

	dbNode, err := db.GetNodeByPubKey(
		ctx, sqlc.GetNodeByPubKeyParams{
			PubKey:  pubKey[:],
			Version: int16(ProtocolV1),
		},
	)
	// The node exists. Return the ID.
	if err == nil {
		return dbNode.ID, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}

	// Otherwise, the node does not exist, so we create a shell entry for
	// it.
	id, err := db.UpsertNode(ctx, sqlc.UpsertNodeParams{
		Version: int16(ProtocolV1),
		PubKey:  pubKey[:],
	})
	if err != nil {
		return 0, fmt.Errorf("unable to create shell node: %w", err)
	}

	return id, nil
}
