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

const (
	ProtocolV1 = 1
)

type SQLQueries interface {
	CreateNode(ctx context.Context, arg sqlc.CreateNodeParams) (int64, error)
	UpsertV1NodeData(ctx context.Context, arg sqlc.UpsertV1NodeDataParams) error
	CreateFeature(ctx context.Context, bit int32) (int64, error)
	GetNodeAddresses(ctx context.Context, nodeID int64) ([]sqlc.GetNodeAddressesRow, error)
	GetNodeByID(ctx context.Context, id int64) (sqlc.Node, error)
	GetNodeByPubKeyAndVersion(ctx context.Context, arg sqlc.GetNodeByPubKeyAndVersionParams) (sqlc.Node, error)
	GetV1NodeData(ctx context.Context, nodeID int64) (sqlc.NodesV1Datum, error)
	GetSourceNodesByVersion(ctx context.Context, version int16) ([]int64, error)
	GetNodeFeatures(ctx context.Context, nodeID int64) ([]sqlc.GetNodeFeaturesRow, error)
	GetExtraNodeTypes(ctx context.Context, nodeID int64) ([]sqlc.NodeExtraType, error)
	UpsertNodeExtraType(ctx context.Context, arg sqlc.UpsertNodeExtraTypeParams) error
	DeleteExtraNodeType(ctx context.Context, arg sqlc.DeleteExtraNodeTypeParams) error
	GetNodeAliasByPubKeyAndVersion(ctx context.Context, arg sqlc.GetNodeAliasByPubKeyAndVersionParams) (sql.NullString, error)
	DeleteNode(ctx context.Context, id int64) error
	DeleteNodeAddresses(ctx context.Context, nodeID int64) error
	DeleteNodeFeature(ctx context.Context, arg sqlc.DeleteNodeFeatureParams) error
	InsertNodeAddress(ctx context.Context, arg sqlc.InsertNodeAddressParams) error
	InsertNodeFeature(ctx context.Context, arg sqlc.InsertNodeFeatureParams) error
	UpdateNode(ctx context.Context, arg sqlc.UpdateNodeParams) error
	GetV1NodesByLastUpdateRange(ctx context.Context, arg sqlc.GetV1NodesByLastUpdateRangeParams) ([]sqlc.Node, error)
	ListNodeIDsAndPubKeysV1(ctx context.Context) ([]sqlc.ListNodeIDsAndPubKeysV1Row, error)
	CreateChannel(ctx context.Context, arg sqlc.CreateChannelParams) (int64, error)
	GetChannelByID(ctx context.Context, id int64) (sqlc.Channel, error)
	GetChannelByOutpointAndVersion(ctx context.Context, arg sqlc.GetChannelByOutpointAndVersionParams) (sqlc.Channel, error)
	GetChannelByChannelIDAndVersion(ctx context.Context, arg sqlc.GetChannelByChannelIDAndVersionParams) (sqlc.Channel, error)
	CreateChannelsV1Data(ctx context.Context, arg sqlc.CreateChannelsV1DataParams) error
	CreateV1ChannelProof(ctx context.Context, arg sqlc.CreateV1ChannelProofParams) error
	GetChannelFeatures(ctx context.Context, channelID int64) ([]sqlc.GetChannelFeaturesRow, error)
	InsertChannelFeature(ctx context.Context, arg sqlc.InsertChannelFeatureParams) error
	GetExtraChannelTypes(ctx context.Context, channelID int64) ([]sqlc.ChannelExtraType, error)
	UpsertChannelExtraType(ctx context.Context, arg sqlc.UpsertChannelExtraTypeParams) error
	DeleteExtraChannelType(ctx context.Context, arg sqlc.DeleteExtraChannelTypeParams) error
	HighestChannelID(ctx context.Context, version int16) ([]byte, error)
	AddChannelPolicyExtraType(ctx context.Context, arg sqlc.AddChannelPolicyExtraTypeParams) error
	CreateChannelPolicy(ctx context.Context, arg sqlc.CreateChannelPolicyParams) (int64, error)
	DeleteAllChannelPolicyExtraTypes(ctx context.Context, channelPolicyID int64) error
	DeleteChannelPolicyExtraType(ctx context.Context, arg sqlc.DeleteChannelPolicyExtraTypeParams) error
	GetChannelPolicyByChannelAndNode(ctx context.Context, arg sqlc.GetChannelPolicyByChannelAndNodeParams) (sqlc.ChannelPolicy, error)
	GetChannelPolicyExtraTypes(ctx context.Context, channelPolicyID int64) ([]sqlc.ChannelPolicyExtraType, error)
	UpdateChannelPolicy(ctx context.Context, arg sqlc.UpdateChannelPolicyParams) error
	CreateChannelPolicyV1Data(ctx context.Context, arg sqlc.CreateChannelPolicyV1DataParams) error
	GetChannelPolicyV1Data(ctx context.Context, channelPolicyID int64) (sqlc.ChannelPolicyV1Datum, error)
	UpdateChannelPolicyV1Data(ctx context.Context, arg sqlc.UpdateChannelPolicyV1DataParams) error
	GetV1ChannelProof(ctx context.Context, channelID int64) (sqlc.V1ChannelProof, error)
	GetChannelIDByOutpoint(ctx context.Context, arg sqlc.GetChannelIDByOutpointParams) ([]byte, error)
	CountZombieChannels(ctx context.Context, version int16) (int64, error)
	IsZombieChannel(ctx context.Context, arg sqlc.IsZombieChannelParams) (bool, error)
	UpsertZombieChannel(ctx context.Context, arg sqlc.UpsertZombieChannelParams) error
	DeleteZombieChannel(ctx context.Context, arg sqlc.DeleteZombieChannelParams) error
	GetV1ChannelPolicyByChannelAndNode(ctx context.Context, arg sqlc.GetV1ChannelPolicyByChannelAndNodeParams) (sqlc.GetV1ChannelPolicyByChannelAndNodeRow, error)
	GetChannelsV1Data(ctx context.Context, channelID int64) (sqlc.ChannelsV1Datum, error)
	ListChannelsByNodeIDAndVersion(ctx context.Context, arg sqlc.ListChannelsByNodeIDAndVersionParams) ([]sqlc.Channel, error)
	ListNodesByVersion(ctx context.Context, version int16) ([]sqlc.ListNodesByVersionRow, error)
	ListAllChannelsByVersion(ctx context.Context, version int16) ([]sqlc.Channel, error)
	AddSourceNode(ctx context.Context, nodeID int64) error
	GetUnconnectedNodes(ctx context.Context, version int16) ([]sqlc.GetUnconnectedNodesRow, error)
	NodeHasV1ProofChannel(ctx context.Context, nodeID1 int64) (bool, error)
	GetZombieChannel(ctx context.Context, arg sqlc.GetZombieChannelParams) (sqlc.ZombieChannel, error)
	DeleteChannel(ctx context.Context, id int64) error
	GetV1DisabledChannelIDs(ctx context.Context) ([][]byte, error)
	GetV1ChannelsByPolicyLastUpdateRange(ctx context.Context, arg sqlc.GetV1ChannelsByPolicyLastUpdateRangeParams) ([]sqlc.Channel, error)
	GetPublicV1ChannelsByChannelID(ctx context.Context, arg sqlc.GetPublicV1ChannelsByChannelIDParams) ([]sqlc.Channel, error)
	DeletePruneLogEntry(ctx context.Context, blockHeight int64) error
	GetPruneTip(ctx context.Context) (sqlc.PruneLog, error)
	GetChannelByOutpoint(ctx context.Context, outpoint string) (sqlc.Channel, error)
	IsV1ChannelPublic(ctx context.Context, channelID int64) (bool, error)
	GetChannelsByChannelIDRange(ctx context.Context, arg sqlc.GetChannelsByChannelIDRangeParams) ([]sqlc.Channel, error)
	DeletePruneLogEntriesInRange(ctx context.Context, arg sqlc.DeletePruneLogEntriesInRangeParams) error
	UpsertPruneLogEntry(ctx context.Context, arg sqlc.UpsertPruneLogEntryParams) error
	InsertClosedChannel(ctx context.Context, channelID []byte) error
	IsClosedChannel(ctx context.Context, channelID []byte) (bool, error)
}

// TxOptions defines the set of db txn options the SQLQueries
// understands.
type TxOptions struct {
	// readOnly governs if a read only transaction is needed or not.
	readOnly bool
}

// ReadOnly returns true if the transaction should be read only.
//
// NOTE: This implements the TxOptions.
func (a *TxOptions) ReadOnly() bool {
	return a.readOnly
}

// NewReadTx creates a new read transaction option set.
func NewReadTx() TxOptions {
	return TxOptions{
		readOnly: true,
	}
}

type BatchedSQLQueries interface {
	SQLQueries

	sqldb.BatchedTx[SQLQueries]
}

type SQLStoreConfig struct {
	ChainHash chainhash.Hash
}

type SQLStore struct {
	cfg *SQLStoreConfig

	db BatchedSQLQueries
}

var _ V1Store = (*SQLStore)(nil)

// NewSQLStore creates a new SQLStore instance given an open BatchedSQLQueries
// storage backend.
func NewSQLStore(cfg *SQLStoreConfig, db BatchedSQLQueries) *SQLStore {
	return &SQLStore{
		cfg: cfg,
		db:  db,
	}
}

func (s *SQLStore) AddLightningNode(node *models.LightningNode,
	_ ...batch.SchedulerOption) error {

	ctx := context.TODO()

	var writeTxOpts TxOptions
	err := s.db.ExecTx(ctx, &writeTxOpts, func(db SQLQueries) error {
		_, err := upsertNode(ctx, db, node)
		return err
	}, func() {})
	if err != nil {
		return fmt.Errorf("unable to insert node: %w", err)
	}

	return nil
}

func (s *SQLStore) SourceNode() (*models.LightningNode, error) {
	ctx := context.TODO()

	var (
		readTx = NewReadTx()
		node   *models.LightningNode
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		nodeID, err := getV1SourceNodeID(ctx, db)
		if err != nil {
			return fmt.Errorf("unable to fetch source node: %w",
				err)
		}

		dbNode, err := db.GetNodeByID(ctx, nodeID)
		if err != nil {
			return fmt.Errorf("unable to fetch source node: %w",
				err)
		}

		node, err = buildNode(ctx, db, &dbNode)

		return err
	}, func() {})
	if err != nil {
		return nil, fmt.Errorf("unable to fetch source node: %w", err)
	}

	return node, nil
}

func getV1SourceNodeID(ctx context.Context, db SQLQueries) (int64, error) {
	nodeIDs, err := db.GetSourceNodesByVersion(ctx, ProtocolV1)
	if err != nil {
		return 0, fmt.Errorf("unable to fetch source node: %w", err)
	}

	if len(nodeIDs) == 0 {
		return 0, ErrSourceNodeNotSet
	} else if len(nodeIDs) > 1 {
		return 0, fmt.Errorf("multiple source nodes for protocol " +
			"version 1 found")
	}

	return nodeIDs[0], nil
}

func (s *SQLStore) IsPublicNode(pubKey [33]byte) (bool, error) {
	ctx := context.TODO()

	var (
		readTx   = NewReadTx()
		isPublic bool
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		dbNode, err := db.GetNodeByPubKeyAndVersion(
			ctx, sqlc.GetNodeByPubKeyAndVersionParams{
				PubKey:  pubKey[:],
				Version: ProtocolV1,
			},
		)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		} else if err != nil {
			return fmt.Errorf("unable to fetch node: %w", err)
		}

		isPublic, err = db.NodeHasV1ProofChannel(ctx, dbNode.ID)
		if err != nil {
			return fmt.Errorf("unable to check if node is "+
				"public: %w", err)
		}

		return nil
	}, func() {})
	if err != nil {
		return false, fmt.Errorf("unable to check if node is "+
			"public: %w", err)
	}

	return isPublic, nil
}

func (s *SQLStore) SetSourceNode(node *models.LightningNode) error {
	ctx := context.TODO()

	var writeTxOpts TxOptions
	return s.db.ExecTx(ctx, &writeTxOpts, func(db SQLQueries) error {
		var nodeID int64
		dbNode, err := db.GetNodeByPubKeyAndVersion(
			ctx, sqlc.GetNodeByPubKeyAndVersionParams{
				PubKey:  node.PubKeyBytes[:],
				Version: ProtocolV1,
			},
		)
		if errors.Is(err, sql.ErrNoRows) {
			id, err := insertNode(ctx, db, node)
			if err != nil {
				return fmt.Errorf("unable to insert new "+
					"node: %w", err)
			}

			nodeID = id
		} else if err != nil {
			return fmt.Errorf("unable to fetch node: %w", err)
		} else if err == nil {
			nodeID = dbNode.ID
		}

		return db.AddSourceNode(ctx, nodeID)
	}, func() {})
}

func (s *SQLStore) ForEachSourceNodeChannel(cb func(chanPoint wire.OutPoint,
	havePolicy bool, otherNode *models.LightningNode) error) error {

	var (
		ctx    = context.TODO()
		readTx = NewReadTx()
	)

	return s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		nodeID, err := getV1SourceNodeID(ctx, db)
		if err != nil {
			return fmt.Errorf("unable to fetch source node: %w",
				err)
		}

		dbNode, err := db.GetNodeByID(ctx, nodeID)
		if err != nil {
			return fmt.Errorf("unable to fetch source node: %w",
				err)
		}

		var nodePub route.Vertex
		copy(nodePub[:], dbNode.PubKey)

		return forEachNodeChannel(
			ctx, db, s.cfg.ChainHash, nodePub,
			func(info *models.ChannelEdgeInfo,
				outPolicy *models.ChannelEdgePolicy,
				_ *models.ChannelEdgePolicy) error {

				// Fetch the other node.
				var otherNodePub [33]byte
				switch {
				case bytes.Equal(info.NodeKey1Bytes[:], nodePub[:]):
					otherNodePub = info.NodeKey2Bytes
				case bytes.Equal(info.NodeKey2Bytes[:], nodePub[:]):
					otherNodePub = info.NodeKey1Bytes
				default:
					return fmt.Errorf("node not " +
						"participating in this channel")
				}

				otherDBNode, err := db.GetNodeByPubKeyAndVersion(
					ctx, sqlc.GetNodeByPubKeyAndVersionParams{
						PubKey:  otherNodePub[:],
						Version: ProtocolV1,
					},
				)
				if err != nil {
					return fmt.Errorf("unable to fetch "+
						"node: %w", err)
				}

				otherNode, err := buildNode(
					ctx, db, &otherDBNode,
				)
				if err != nil {
					return fmt.Errorf("unable to build "+
						"node: %w", err)
				}

				return cb(
					info.ChannelPoint, outPolicy != nil,
					otherNode,
				)
			},
		)
	}, func() {})
}

func (s *SQLStore) FetchLightningNode(pubKey route.Vertex) (
	*models.LightningNode, error) {

	ctx := context.TODO()

	var (
		readTx = NewReadTx()
		node   *models.LightningNode
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

func (s *SQLStore) PruneGraph(spentOutputs []*wire.OutPoint,
	blockHash *chainhash.Hash, blockHeight uint32) (
	[]*models.ChannelEdgeInfo, []route.Vertex, error) {

	var (
		ctx         = context.TODO()
		writeTx     TxOptions
		closedChans []*models.ChannelEdgeInfo
		prunedNodes []route.Vertex
	)
	err := s.db.ExecTx(ctx, &writeTx, func(db SQLQueries) error {
		for _, outpoint := range spentOutputs {
			chanInfo, err := db.GetChannelByOutpoint(
				ctx, outpoint.String(),
			)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			} else if err != nil {
				return fmt.Errorf("unable to fetch channel: %w",
					err)
			}

			info, _, _, err := buildChannel(
				ctx, db, s.cfg.ChainHash, chanInfo,
			)
			if err != nil {
				return fmt.Errorf("unable to build channel: %w",
					err)
			}

			closedChans = append(closedChans, info)

			err = db.DeleteChannel(ctx, chanInfo.ID)
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

	return closedChans, prunedNodes, nil
}

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

func (s *SQLStore) DisconnectBlockAtHeight(height uint32) (
	[]*models.ChannelEdgeInfo, error) {

	var (
		ctx     = context.TODO()
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
		dbChans, err := db.GetChannelsByChannelIDRange(
			ctx, sqlc.GetChannelsByChannelIDRangeParams{
				StartChannelID: chanIDStart[:],
				EndChannelID:   chanIDEnd[:],
			},
		)
		if err != nil {
			return fmt.Errorf("unable to fetch channels: %w", err)
		}

		for _, dbChan := range dbChans {
			// TODO(elle): more efficient query.
			//  Also: let buildChannel switch on protocol version
			//  and error out if unhandled.
			channel, _, _, err := buildChannel(
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

	return removedChans, nil
}

func (s *SQLStore) DeleteChannelEdges(strictZombiePruning, markZombie bool,
	chanIDs ...uint64) ([]*models.ChannelEdgeInfo, error) {

	// remove edges with given channel_ids.
	// maybe marks them as zombies
	// if strict, pub keys get set in zombie index
	// returns the deleted edges

	var (
		ctx     = context.TODO()
		writeTx TxOptions
		deleted []*models.ChannelEdgeInfo
	)
	err := s.db.ExecTx(ctx, &writeTx, func(db SQLQueries) error {
		for _, chanID := range chanIDs {
			var chanIDB [8]byte
			byteOrder.PutUint64(chanIDB[:], chanID)

			dbChan, err := db.GetChannelByChannelIDAndVersion(
				ctx, sqlc.GetChannelByChannelIDAndVersionParams{
					ChannelID: chanIDB[:],
					Version:   ProtocolV1,
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
					Version:   ProtocolV1,
					ChannelID: int64(chanID),
					NodeKey1:  nodeKey1[:],
					NodeKey2:  nodeKey2[:],
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

	return deleted, nil
}

func (s *SQLStore) HasChannelEdge(chanID uint64) (time.Time, time.Time, bool,
	bool, error) {

	ctx := context.TODO()

	var (
		readTx          = NewReadTx()
		exists          bool
		isZombie        bool
		node1LastUpdate time.Time
		node2LastUpdate time.Time
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		var chanIDB [8]byte
		byteOrder.PutUint64(chanIDB[:], chanID)

		channel, err := db.GetChannelByChannelIDAndVersion(
			ctx, sqlc.GetChannelByChannelIDAndVersionParams{
				ChannelID: chanIDB[:],
				Version:   ProtocolV1,
			},
		)
		if errors.Is(err, sql.ErrNoRows) {
			// Check if it is a zombie channel.
			isZombie, err = db.IsZombieChannel(
				ctx, sqlc.IsZombieChannelParams{
					ChannelID: int64(chanID),
					Version:   ProtocolV1,
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

		policy1, err := db.GetV1ChannelPolicyByChannelAndNode(
			ctx, sqlc.GetV1ChannelPolicyByChannelAndNodeParams{
				ChannelID: channel.ID,
				NodeID:    channel.NodeID1,
			},
		)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("unable to fetch channel policy: %w",
				err)
		} else if err == nil {
			node1LastUpdate = time.Unix(policy1.LastUpdate, 0)
		}

		policy2, err := db.GetV1ChannelPolicyByChannelAndNode(
			ctx, sqlc.GetV1ChannelPolicyByChannelAndNodeParams{
				ChannelID: channel.ID,
				NodeID:    channel.NodeID2,
			},
		)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("unable to fetch channel policy: %w",
				err)
		} else if err == nil {
			node2LastUpdate = time.Unix(policy2.LastUpdate, 0)
		}

		return nil
	}, func() {})
	if err != nil {
		return time.Time{}, time.Time{}, false, false,
			fmt.Errorf("unable to fetch channel: %w", err)
	}

	return node1LastUpdate, node2LastUpdate, exists, isZombie, nil
}

func (s *SQLStore) HasLightningNode(pubKey [33]byte) (time.Time, bool,
	error) {

	ctx := context.TODO()

	var (
		readTx     = NewReadTx()
		exists     bool
		lastUpdate time.Time
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		node, err := db.GetNodeByPubKeyAndVersion(
			ctx, sqlc.GetNodeByPubKeyAndVersionParams{
				PubKey:  pubKey[:],
				Version: ProtocolV1,
			},
		)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		} else if err != nil {
			return fmt.Errorf("unable to fetch node: %w", err)
		}

		exists = true

		v1Node, err := db.GetV1NodeData(ctx, node.ID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		} else if err != nil {
			return fmt.Errorf("unable to fetch v1 node data: %w",
				err)
		}

		lastUpdate = time.Unix(v1Node.LastUpdate, 0)

		return nil

	}, func() {})
	if err != nil {
		return lastUpdate, false,
			fmt.Errorf("unable to fetch node: %w", err)
	}

	return lastUpdate, exists, nil
}

func (s *SQLStore) AddEdgeProof(chanID lnwire.ShortChannelID,
	proof *models.ChannelAuthProof) error {

	var (
		ctx     = context.TODO()
		writeTx TxOptions
	)
	err := s.db.ExecTx(ctx, &writeTx, func(db SQLQueries) error {
		// Get Channel with ID.
		// Add ChannelProof.
		var chanDBB [8]byte
		byteOrder.PutUint64(chanDBB[:], chanID.ToUint64())

		dbChan, err := db.GetChannelByChannelIDAndVersion(
			ctx, sqlc.GetChannelByChannelIDAndVersionParams{
				ChannelID: chanDBB[:],
				Version:   ProtocolV1,
			},
		)
		if err != nil {
			return fmt.Errorf("unable to fetch channel: %w", err)
		}

		return db.CreateV1ChannelProof(
			ctx, sqlc.CreateV1ChannelProofParams{
				ChannelID:         dbChan.ID,
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

func pruneGraphNodes(ctx context.Context, db SQLQueries) ([]route.Vertex,
	error) {

	// TODO(elle): just do all protocols here.
	nodes, err := db.GetUnconnectedNodes(ctx, ProtocolV1)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch unconnected nodes: %w",
			err)
	}

	sourceNodeID, err := getV1SourceNodeID(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch source node: %w", err)
	}

	prunedNodes := make([]route.Vertex, len(nodes))
	for _, node := range nodes {
		if node.ID == sourceNodeID {
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

func (s *SQLStore) DeleteLightningNode(pubKey route.Vertex) error {
	ctx := context.TODO()

	var writeTxOpts TxOptions
	err := s.db.ExecTx(ctx, &writeTxOpts, func(db SQLQueries) error {
		node, err := db.GetNodeByPubKeyAndVersion(
			ctx, sqlc.GetNodeByPubKeyAndVersionParams{
				PubKey:  pubKey[:],
				Version: ProtocolV1,
			},
		)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrGraphNodeNotFound
		} else if err != nil {
			return fmt.Errorf("unable to fetch node: %w", err)
		}

		return db.DeleteNode(ctx, node.ID)
	}, func() {})
	if err != nil {
		return fmt.Errorf("unable to delete node: %w", err)
	}

	return nil
}

func (s *SQLStore) LookupAlias(pub *btcec.PublicKey) (string, error) {
	var (
		ctx    = context.TODO()
		readTx = NewReadTx()
		alias  string
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		dbAlias, err := db.GetNodeAliasByPubKeyAndVersion(
			ctx, sqlc.GetNodeAliasByPubKeyAndVersionParams{
				PubKey:  pub.SerializeCompressed(),
				Version: ProtocolV1,
			},
		)
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

func (s *SQLStore) ForEachNodeChannel(nodePub route.Vertex,
	cb func(*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy) error) error {

	var (
		ctx    = context.TODO()
		readTx = NewReadTx()
	)

	return s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		return forEachNodeChannel(ctx, db, s.cfg.ChainHash, nodePub, cb)
	}, func() {})
}

func forEachNodeChannel(ctx context.Context, db SQLQueries,
	chain chainhash.Hash,
	nodePub route.Vertex, cb func(*models.ChannelEdgeInfo,
		*models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy) error) error {

	dbNode, err := db.GetNodeByPubKeyAndVersion(
		ctx, sqlc.GetNodeByPubKeyAndVersionParams{
			Version: ProtocolV1,
			PubKey:  nodePub[:],
		},
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return fmt.Errorf("unable to fetch node: %w", err)
	}

	dbChannels, err := db.ListChannelsByNodeIDAndVersion(
		ctx, sqlc.ListChannelsByNodeIDAndVersionParams{
			Version: ProtocolV1,
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

		if err := cb(e, outPolicy, inPolicy); err != nil {
			return err
		}
	}

	return nil
}

func (s *SQLStore) ForEachNodeCached(cb func(node route.Vertex,
	chans map[uint64]*DirectedChannel) error) error {

	var (
		ctx    = context.TODO()
		readTx = NewReadTx()
	)
	return s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		nodes, err := db.ListNodesByVersion(ctx, ProtocolV1)
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

			dbChannels, err := db.ListChannelsByNodeIDAndVersion(
				ctx, sqlc.ListChannelsByNodeIDAndVersionParams{
					Version: ProtocolV1,
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

func (s *SQLStore) GraphSession(cb func(graph NodeTraverser) error) error {
	var (
		ctx    = context.TODO()
		readTx = NewReadTx()
	)

	return s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		return cb(newSQLNodeTraverser(db, s.cfg.ChainHash))
	}, func() {})
}

type sqlNodeTraverser struct {
	db    SQLQueries
	chain chainhash.Hash
}

func newSQLNodeTraverser(db SQLQueries,
	chain chainhash.Hash) *sqlNodeTraverser {

	return &sqlNodeTraverser{
		db:    db,
		chain: chain,
	}
}

func (s *sqlNodeTraverser) ForEachNodeDirectedChannel(nodePub route.Vertex,
	cb func(channel *DirectedChannel) error) error {

	ctx := context.TODO()

	return forEachNodeDirectedChannel(ctx, s.db, s.chain, nodePub, cb)
}

func (s *sqlNodeTraverser) FetchNodeFeatures(nodePub route.Vertex) (
	*lnwire.FeatureVector, error) {

	ctx := context.TODO()

	dbNode, err := s.db.GetNodeByPubKeyAndVersion(
		ctx, sqlc.GetNodeByPubKeyAndVersionParams{
			PubKey:  nodePub[:],
			Version: ProtocolV1,
		},
	)
	if errors.Is(err, sql.ErrNoRows) {
		return lnwire.EmptyFeatureVector(), nil
	} else if err != nil {
		return nil, fmt.Errorf("unable to fetch node: %w", err)
	}

	return getNodeFeatures(ctx, s.db, dbNode.ID)
}

var _ NodeTraverser = (*sqlNodeTraverser)(nil)

func (s *SQLStore) ForEachChannel(cb func(*models.ChannelEdgeInfo,
	*models.ChannelEdgePolicy,
	*models.ChannelEdgePolicy) error) error {

	var (
		ctx    = context.TODO()
		readTx = NewReadTx()
	)

	return s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		dbChannels, err := db.ListAllChannelsByVersion(ctx, ProtocolV1)
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

func (s *SQLStore) ForEachNode(cb func(tx NodeRTx) error) error {
	var (
		ctx    = context.TODO()
		readTx = NewReadTx()
	)

	return s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		return forEachNode(ctx, db,
			func(nodeID int64, nodePub route.Vertex) error {
				dbNode, err := db.GetNodeByID(ctx, nodeID)
				if err != nil {
					return fmt.Errorf("unable to fetch "+
						"node: %w", err)
				}

				node, err := buildNode(ctx, db, &dbNode)
				if err != nil {
					return fmt.Errorf("unable to build "+
						"node: %w", err)
				}

				return cb(newSqlGraphNodeTx(
					db, s.cfg.ChainHash, node,
				))
			},
		)
	}, func() {})
}

type sqlGraphNodeTx struct {
	db    SQLQueries
	node  *models.LightningNode
	chain chainhash.Hash
}

func newSqlGraphNodeTx(db SQLQueries, chain chainhash.Hash,
	node *models.LightningNode) *sqlGraphNodeTx {

	return &sqlGraphNodeTx{
		db:    db,
		node:  node,
		chain: chain,
	}
}

func (s *sqlGraphNodeTx) Node() *models.LightningNode {
	return s.node
}

func (s *sqlGraphNodeTx) ForEachChannel(cb func(*models.ChannelEdgeInfo,
	*models.ChannelEdgePolicy, *models.ChannelEdgePolicy) error) error {

	ctx := context.TODO()

	return forEachNodeChannel(
		ctx, s.db, s.chain, s.node.PubKeyBytes, cb,
	)
}

func (s *sqlGraphNodeTx) FetchNode(nodePub route.Vertex) (NodeRTx, error) {
	ctx := context.TODO()

	dbNode, err := s.db.GetNodeByPubKeyAndVersion(
		ctx, sqlc.GetNodeByPubKeyAndVersionParams{
			Version: ProtocolV1,
			PubKey:  nodePub[:],
		},
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrGraphNodeNotFound
	} else if err != nil {
		return nil, fmt.Errorf("unable to fetch node: %w", err)
	}

	node, err := buildNode(ctx, s.db, &dbNode)
	if err != nil {
		return nil, fmt.Errorf("unable to build node: %w", err)
	}

	return newSqlGraphNodeTx(s.db, s.chain, node), nil
}

var _ NodeRTx = (*sqlGraphNodeTx)(nil)

func (s *SQLStore) ForEachNodeDirectedChannel(nodePub route.Vertex,
	cb func(channel *DirectedChannel) error) error {

	var (
		ctx    = context.TODO()
		readTx = NewReadTx()
	)

	return s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		return forEachNodeDirectedChannel(
			ctx, db, s.cfg.ChainHash, nodePub, cb,
		)
	}, func() {})
}

func forEachNodeDirectedChannel(ctx context.Context, db SQLQueries,
	chain chainhash.Hash, nodePub route.Vertex,
	cb func(channel *DirectedChannel) error) error {

	// Fallback that uses the database.
	toNodeCallback := func() route.Vertex {
		return nodePub
	}

	dbNode, err := db.GetNodeByPubKeyAndVersion(
		ctx, sqlc.GetNodeByPubKeyAndVersionParams{
			Version: ProtocolV1,
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
		return fmt.Errorf("unable to fetch node features: %w",
			err)
	}

	dbChannels, err := db.ListChannelsByNodeIDAndVersion(
		ctx, sqlc.ListChannelsByNodeIDAndVersionParams{
			Version: ProtocolV1,
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

func forEachNode(ctx context.Context, db SQLQueries,
	cb func(nodeID int64, nodePub route.Vertex) error) error {

	nodes, err := db.ListNodeIDsAndPubKeysV1(ctx)
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

func (s *SQLStore) AddrsForNode(nodePub *btcec.PublicKey) (bool, []net.Addr,
	error) {

	ctx := context.TODO()

	var (
		readTx    = NewReadTx()
		addresses []net.Addr
		known     bool
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		node, err := db.GetNodeByPubKeyAndVersion(
			ctx, sqlc.GetNodeByPubKeyAndVersionParams{
				PubKey:  nodePub.SerializeCompressed(),
				Version: ProtocolV1,
			},
		)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		} else if err != nil {
			return fmt.Errorf("unable to fetch node: %w", err)
		}

		known = true

		addresses, err = getNodeAddresses(ctx, db, node.ID)
		if err != nil {
			return fmt.Errorf("unable to fetch node addresses: %w",
				err)
		}

		return nil

	}, func() {})
	if err != nil {
		return false, nil, fmt.Errorf("unable to fetch node: %w", err)
	}

	return known, addresses, nil
}

func (s *SQLStore) FetchNodeFeatures(nodePub route.Vertex) (
	*lnwire.FeatureVector, error) {

	ctx := context.TODO()

	var (
		readTx   = NewReadTx()
		features *lnwire.FeatureVector
	)

	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		node, err := db.GetNodeByPubKeyAndVersion(
			ctx, sqlc.GetNodeByPubKeyAndVersionParams{
				PubKey:  nodePub[:],
				Version: ProtocolV1,
			},
		)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrGraphNodeNotFound
		} else if err != nil {
			return fmt.Errorf("unable to fetch node: %w", err)
		}

		features, err = getNodeFeatures(ctx, db, node.ID)
		if err != nil {
			return fmt.Errorf("unable to fetch node features: %w", err)
		}

		return nil
	}, func() {})
	if err != nil {
		return nil, fmt.Errorf("unable to fetch node: %w", err)
	}

	return features, nil
}

func (s *SQLStore) ForEachNodeCacheable(cb func(route.Vertex,
	*lnwire.FeatureVector) error) error {

	ctx := context.TODO()

	var readTx = NewReadTx()
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
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

func (s *SQLStore) NodeUpdatesInHorizon(startTime,
	endTime time.Time) ([]models.LightningNode, error) {

	ctx := context.TODO()

	var (
		readTx = NewReadTx()
		nodes  []models.LightningNode
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		dbNodes, err := db.GetV1NodesByLastUpdateRange(
			ctx, sqlc.GetV1NodesByLastUpdateRangeParams{
				StartTime: startTime.Unix(),
				EndTime:   endTime.Unix(),
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
	}, func() {})
	if err != nil {
		return nil, fmt.Errorf("unable to fetch nodes: %w", err)
	}

	return nodes, nil
}

func (s *SQLStore) HighestChanID() (uint64, error) {
	ctx := context.TODO()

	var (
		readTx        = NewReadTx()
		highestChanID uint64
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		chanID, err := db.HighestChannelID(ctx, ProtocolV1)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		} else if err != nil {
			return fmt.Errorf("unable to fetch highest chan ID: %w",
				err)
		}

		highestChanID = byteOrder.Uint64(chanID)

		return nil
	}, func() {})
	if err != nil {
		return 0, fmt.Errorf("unable to fetch highest chan ID: %w", err)
	}

	return highestChanID, nil
}

func (s *SQLStore) AddChannelEdge(edge *models.ChannelEdgeInfo,
	_ ...batch.SchedulerOption) error {

	ctx := context.TODO()

	var writeTxOpts TxOptions
	err := s.db.ExecTx(ctx, &writeTxOpts, func(db SQLQueries) error {
		var chanIDB [8]byte
		byteOrder.PutUint64(chanIDB[:], edge.ChannelID)

		// Make sure that this channel does not already exist in the
		// database.
		_, err := db.GetChannelByChannelIDAndVersion(
			ctx, sqlc.GetChannelByChannelIDAndVersionParams{
				ChannelID: chanIDB[:],
				Version:   ProtocolV1,
			})
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

func (s *SQLStore) UpdateEdgePolicy(edge *models.ChannelEdgePolicy,
	_ ...batch.SchedulerOption) (route.Vertex, route.Vertex, error) {

	ctx := context.TODO()
	var (
		writeTxOpts        TxOptions
		node1Pub, node2Pub route.Vertex
	)
	err := s.db.ExecTx(ctx, &writeTxOpts, func(db SQLQueries) error {
		var chanIDB [8]byte
		byteOrder.PutUint64(chanIDB[:], edge.ChannelID)

		dbChan, err := db.GetChannelByChannelIDAndVersion(
			ctx, sqlc.GetChannelByChannelIDAndVersionParams{
				ChannelID: chanIDB[:],
				Version:   ProtocolV1,
			},
		)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrEdgeNotFound
		} else if err != nil {
			return fmt.Errorf("unable to fetch channel: %w", err)
		}

		node1, err := db.GetNodeByID(ctx, dbChan.NodeID1)
		if err != nil {
			return err
		}
		copy(node1Pub[:], node1.PubKey)

		node2, err := db.GetNodeByID(ctx, dbChan.NodeID2)
		if err != nil {
			return err
		}
		copy(node2Pub[:], node2.PubKey)

		// Figure out which node this edge is from.
		isNode1 := edge.ChannelFlags&lnwire.ChanUpdateDirection == 0
		nodeID := dbChan.NodeID1
		if !isNode1 {
			nodeID = dbChan.NodeID2
		}

		// First check if a record for this policy already exists. If
		// one does not, we create a new record, otherwise we update
		// the existing one.
		dbPolicy, err := db.GetChannelPolicyByChannelAndNode(
			ctx, sqlc.GetChannelPolicyByChannelAndNodeParams{
				ChannelID: dbChan.ID,
				NodeID:    nodeID,
			},
		)
		switch {
		// The policy does not yet exist in the DB, so we insert a
		// fresh record.
		case errors.Is(err, sql.ErrNoRows):
			err := insertChanPolicy(
				ctx, db, dbChan.ID, nodeID, edge,
			)
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
			return updateChanPolicy(ctx, db, dbPolicy.ID, edge)
		}

	}, func() {})
	if err != nil {
		return route.Vertex{}, route.Vertex{}, fmt.Errorf("unable to "+
			"update edge policy: %w", err)
	}

	return node1Pub, node2Pub, nil
}

func (s *SQLStore) ChannelID(chanPoint *wire.OutPoint) (uint64, error) {
	var (
		ctx       = context.TODO()
		readTx    = NewReadTx()
		channelID uint64
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		chanID, err := db.GetChannelIDByOutpoint(
			ctx, sqlc.GetChannelIDByOutpointParams{
				Outpoint: chanPoint.String(),
				Version:  ProtocolV1,
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

func (s *SQLStore) MarkEdgeZombie(chanID uint64,
	pubKey1, pubKey2 [33]byte) error {

	ctx := context.TODO()

	var writeTxOpts TxOptions
	return s.db.ExecTx(ctx, &writeTxOpts, func(db SQLQueries) error {
		return db.UpsertZombieChannel(
			ctx, sqlc.UpsertZombieChannelParams{
				Version:   ProtocolV1,
				ChannelID: int64(chanID),
				NodeKey1:  pubKey1[:],
				NodeKey2:  pubKey2[:],
			},
		)
	}, func() {})
}

func (s *SQLStore) MarkEdgeLive(chanID uint64) error {
	var (
		ctx     = context.TODO()
		writeTx TxOptions
	)
	return s.db.ExecTx(ctx, &writeTx, func(db SQLQueries) error {
		_, err := db.GetZombieChannel(
			ctx, sqlc.GetZombieChannelParams{
				ChannelID: int64(chanID),
				Version:   ProtocolV1,
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
				ChannelID: int64(chanID),
				Version:   ProtocolV1,
			},
		)
	}, func() {})
}

func (s *SQLStore) IsZombieEdge(chanID uint64) (bool, [33]byte, [33]byte) {
	var (
		ctx              = context.TODO()
		readTx           = NewReadTx()
		isZombie         bool
		pubKey1, pubKey2 route.Vertex
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		zombie, err := db.GetZombieChannel(
			ctx, sqlc.GetZombieChannelParams{
				ChannelID: int64(chanID),
				Version:   ProtocolV1,
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

func (s *SQLStore) NumZombies() (uint64, error) {
	var (
		ctx        = context.TODO()
		readTx     = NewReadTx()
		numZombies uint64
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		count, err := db.CountZombieChannels(ctx, ProtocolV1)
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

func (s *SQLStore) DisabledChannelIDs() ([]uint64, error) {
	var (
		ctx     = context.TODO()
		readTx  = NewReadTx()
		chanIDs []uint64
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		dbChanIDs, err := db.GetV1DisabledChannelIDs(ctx)
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

func (s *SQLStore) ChannelView() ([]EdgePoint, error) {
	// For each channel: get its channel point and btc1 & btc2 keys.

	var (
		ctx        = context.TODO()
		readTx     = NewReadTx()
		edgePoints []EdgePoint
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		dbChannel, err := db.ListAllChannelsByVersion(ctx, ProtocolV1)
		if err != nil {
			return fmt.Errorf("unable to fetch channels: %w", err)
		}

		for _, dbChan := range dbChannel {
			v1Data, err := db.GetChannelsV1Data(ctx, dbChan.ID)
			if err != nil {
				return fmt.Errorf("unable to fetch v1 data: %w",
					err)
			}

			pkScript, err := genMultiSigP2WSH(
				v1Data.BitcoinKey1,
				v1Data.BitcoinKey2,
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

func (s *SQLStore) FilterChannelRange(startHeight, endHeight uint32,
	withTimestamps bool) ([]BlockChannelRange, error) {

	var (
		ctx       = context.TODO()
		readTx    = NewReadTx()
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
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		dbChans, err := db.GetPublicV1ChannelsByChannelID(
			ctx, sqlc.GetPublicV1ChannelsByChannelIDParams{
				StartChannelID: chanIDStart[:],
				EndChannelID:   chanIDEnd[:],
			},
		)
		if err != nil {
			return fmt.Errorf("unable to fetch channel range: %w",
				err)
		}

		for _, dbChan := range dbChans {
			cid := lnwire.NewShortChanIDFromInt(
				byteOrder.Uint64(dbChan.ChannelID),
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

			node1Policy, err := db.GetV1ChannelPolicyByChannelAndNode(
				ctx, sqlc.GetV1ChannelPolicyByChannelAndNodeParams{
					ChannelID: dbChan.ID,
					NodeID:    dbChan.NodeID1,
				},
			)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("unable to fetch node1 "+
					"policy: %w", err)
			} else if err == nil {
				chanInfo.Node1UpdateTimestamp = time.Unix(
					node1Policy.LastUpdate, 0,
				)
			}

			node2Policy, err := db.GetV1ChannelPolicyByChannelAndNode(
				ctx, sqlc.GetV1ChannelPolicyByChannelAndNodeParams{
					ChannelID: dbChan.ID,
					NodeID:    dbChan.NodeID2,
				},
			)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("unable to fetch node2 "+
					"policy: %w", err)
			} else if err == nil {
				chanInfo.Node2UpdateTimestamp = time.Unix(
					node2Policy.LastUpdate, 0,
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

func (s *SQLStore) FilterKnownChanIDs(chansInfo []ChannelUpdateInfo) ([]uint64,
	[]ChannelUpdateInfo, error) {

	var (
		ctx          = context.TODO()
		readTx       = NewReadTx()
		newChanIDs   []uint64
		knownZombies []ChannelUpdateInfo
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		for _, chanInfo := range chansInfo {
			channelID := chanInfo.ShortChannelID.ToUint64()
			var chanIDB [8]byte
			byteOrder.PutUint64(chanIDB[:], channelID)

			_, err := db.GetChannelByChannelIDAndVersion(
				ctx, sqlc.GetChannelByChannelIDAndVersionParams{
					Version:   ProtocolV1,
					ChannelID: chanIDB[:],
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
					ChannelID: int64(channelID),
					Version:   ProtocolV1,
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

			dbChan, err := db.GetChannelByChannelIDAndVersion(
				ctx, sqlc.GetChannelByChannelIDAndVersionParams{
					ChannelID: chanIDB[:],
					Version:   ProtocolV1,
				},
			)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			} else if err != nil {
				return fmt.Errorf("unable to fetch channel: %w", err)
			}

			channel, p1, p2, err := buildChannel(
				ctx, db, s.cfg.ChainHash, dbChan,
			)
			if err != nil {
				return fmt.Errorf("unable to build channel: %w",
					err)
			}

			dbNode1, err := db.GetNodeByID(ctx, dbChan.NodeID1)
			if err != nil {
				return fmt.Errorf("unable to fetch node(%d) "+
					"pub key: %w", dbChan.NodeID1, err)
			}

			node1, err := buildNode(ctx, db, &dbNode1)
			if err != nil {
				return fmt.Errorf("unable to build node: %w",
					err)
			}

			dbNode2, err := db.GetNodeByID(ctx, dbChan.NodeID2)
			if err != nil {
				return fmt.Errorf("unable to fetch node(%d) "+
					"pub key: %w", dbChan.NodeID2, err)
			}

			node2, err := buildNode(ctx, db, &dbNode2)
			if err != nil {
				return fmt.Errorf("unable to build node: %w",
					err)
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

func (s *SQLStore) ChanUpdatesInHorizon(startTime,
	endTime time.Time) ([]ChannelEdge, error) {

	var (
		ctx    = context.TODO()
		readTx = NewReadTx()
		edges  []ChannelEdge
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		dbChans, err := db.GetV1ChannelsByPolicyLastUpdateRange(
			ctx, sqlc.GetV1ChannelsByPolicyLastUpdateRangeParams{
				StartTime: startTime.Unix(),
				EndTime:   endTime.Unix(),
			},
		)
		if err != nil {
			return fmt.Errorf("unable to fetch channels: %w", err)
		}

		for _, dbChan := range dbChans {
			channel, p1, p2, err := buildChannel(
				ctx, db, s.cfg.ChainHash, dbChan,
			)
			if err != nil {
				return fmt.Errorf("unable to build channel: %w",
					err)
			}

			dbNode1, err := db.GetNodeByID(ctx, dbChan.NodeID1)
			if err != nil {
				return fmt.Errorf("unable to fetch node(%d) "+
					"pub key: %w", dbChan.NodeID1, err)
			}

			node1, err := buildNode(ctx, db, &dbNode1)
			if err != nil {
				return fmt.Errorf("unable to build node: %w",
					err)
			}

			dbNode2, err := db.GetNodeByID(ctx, dbChan.NodeID2)
			if err != nil {
				return fmt.Errorf("unable to fetch node(%d) "+
					"pub key: %w", dbChan.NodeID2, err)
			}

			node2, err := buildNode(ctx, db, &dbNode2)
			if err != nil {
				return fmt.Errorf("unable to build node: %w",
					err)
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

func (s *SQLStore) PutClosedScid(scid lnwire.ShortChannelID) error {
	ctx := context.TODO()

	var writeTx TxOptions
	return s.db.ExecTx(ctx, &writeTx, func(db SQLQueries) error {
		var chanIDB [8]byte
		byteOrder.PutUint64(chanIDB[:], scid.ToUint64())

		return db.InsertClosedChannel(ctx, chanIDB[:])
	}, func() {})
}

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

func (s *SQLStore) FetchChannelEdgesByID(chanID uint64) (
	*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
	*models.ChannelEdgePolicy, error) {

	var (
		ctx              = context.TODO()
		readTx           = NewReadTx()
		edge             *models.ChannelEdgeInfo
		policy1, policy2 *models.ChannelEdgePolicy
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		var chanIDB [8]byte
		byteOrder.PutUint64(chanIDB[:], chanID)

		dbChan, err := db.GetChannelByChannelIDAndVersion(
			ctx, sqlc.GetChannelByChannelIDAndVersionParams{
				ChannelID: chanIDB[:],
				Version:   ProtocolV1,
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

func (s *SQLStore) FetchChannelEdgesByOutpoint(op *wire.OutPoint) (
	*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
	*models.ChannelEdgePolicy, error) {

	var (
		ctx              = context.TODO()
		readTx           = NewReadTx()
		edge             *models.ChannelEdgeInfo
		policy1, policy2 *models.ChannelEdgePolicy
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		dbChan, err := db.GetChannelByOutpointAndVersion(
			ctx, sqlc.GetChannelByOutpointAndVersionParams{
				Outpoint: op.String(),
				Version:  ProtocolV1,
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

func getChannelNodes(ctx context.Context, db SQLQueries,
	dbChan sqlc.Channel) (route.Vertex, route.Vertex, error) {

	var node1Vertex, node2Vertex route.Vertex

	node1, err := db.GetNodeByID(ctx, dbChan.NodeID1)
	if err != nil {
		return node1Vertex, node2Vertex, fmt.Errorf("unable to "+
			"fetch node(%d) pub key: %w", dbChan.NodeID1, err)
	}
	node1Vertex, err = route.NewVertexFromBytes(node1.PubKey)
	if err != nil {
		return node1Vertex, node2Vertex, err
	}

	node2, err := db.GetNodeByID(ctx, dbChan.NodeID2)
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

func buildChannel(ctx context.Context, db SQLQueries,
	chain chainhash.Hash, dbChan sqlc.Channel) (*models.ChannelEdgeInfo,
	*models.ChannelEdgePolicy, *models.ChannelEdgePolicy, error) {

	op, err := wire.NewOutPointFromString(dbChan.Outpoint)
	if err != nil {
		return nil, nil, nil, err
	}

	node1Vertex, node2Vertex, err := getChannelNodes(ctx, db, dbChan)
	if err != nil {
		return nil, nil, nil, err
	}

	features, err := getChanFeatures(ctx, db, dbChan.ID)
	if err != nil {
		return nil, nil, nil, err
	}

	var featureBuf bytes.Buffer
	if err := features.Encode(&featureBuf); err != nil {
		return nil, nil, nil, fmt.Errorf("unable to encode "+
			"features: %w", err)
	}

	extraTypes, err := getChannelExtraSignedFields(ctx, db, dbChan.ID)
	if err != nil {
		return nil, nil, nil, err
	}

	recs, err := lnwire.CustomRecords(extraTypes).Serialize()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to serialize extra "+
			"signed fields: %w", err)
	}
	if recs == nil {
		recs = make([]byte, 0)
	}

	v1Data, err := db.GetChannelsV1Data(ctx, dbChan.ID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to fetch v1 "+
			"channel data: %w", err)
	}

	var btcKey1, btcKey2 route.Vertex
	copy(btcKey1[:], v1Data.BitcoinKey1)
	copy(btcKey2[:], v1Data.BitcoinKey2)

	channel := &models.ChannelEdgeInfo{
		ChainHash:        chain,
		ChannelID:        byteOrder.Uint64(dbChan.ChannelID),
		NodeKey1Bytes:    node1Vertex,
		NodeKey2Bytes:    node2Vertex,
		BitcoinKey1Bytes: btcKey1,
		BitcoinKey2Bytes: btcKey2,
		ChannelPoint:     *op,
		Capacity:         btcutil.Amount(dbChan.Capacity),
		Features:         featureBuf.Bytes(),
		ExtraOpaqueData:  recs,
	}

	dbProof, err := db.GetV1ChannelProof(ctx, dbChan.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil, fmt.Errorf("unable to fetch channel "+
			"proof: %w", err)
	} else if !errors.Is(err, sql.ErrNoRows) {
		auth := models.ChannelAuthProof{
			NodeSig1Bytes:    dbProof.Node1Signature,
			NodeSig2Bytes:    dbProof.Node2Signature,
			BitcoinSig1Bytes: dbProof.Bitcoin1Signature,
			BitcoinSig2Bytes: dbProof.Bitcoin2Signature,
		}

		channel.AuthProof = &auth
	}

	node1Policy, err := getChanPolicy(
		ctx, db, byteOrder.Uint64(dbChan.ChannelID), dbChan.ID,
		dbChan.NodeID1, node2Vertex, true,
	)
	if err != nil {
		return nil, nil, nil, err
	}

	node2Policy, err := getChanPolicy(
		ctx, db, byteOrder.Uint64(dbChan.ChannelID), dbChan.ID,
		dbChan.NodeID2, node1Vertex, false,
	)
	if err != nil {
		return nil, nil, nil, err
	}

	return channel, node1Policy, node2Policy, nil
}

func upsertNode(ctx context.Context, db SQLQueries,
	node *models.LightningNode) (int64, error) {

	// First, check if this node already exists.
	dbNode, err := db.GetNodeByPubKeyAndVersion(
		ctx, sqlc.GetNodeByPubKeyAndVersionParams{
			PubKey:  node.PubKeyBytes[:],
			Version: ProtocolV1,
		},
	)
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

func insertNode(ctx context.Context, db SQLQueries,
	node *models.LightningNode) (int64, error) {

	var alias sql.NullString
	if node.HaveNodeAnnouncement {
		alias = sql.NullString{
			Valid:  true,
			String: node.Alias,
		}
	}

	nodeParams := sqlc.CreateNodeParams{
		Version:   ProtocolV1,
		PubKey:    node.PubKeyBytes[:],
		Alias:     alias,
		Signature: node.AuthSigBytes,
	}

	// Insert the main node info.
	nodeID, err := db.CreateNode(ctx, nodeParams)
	if err != nil {
		return 0, fmt.Errorf("inserting node: %w", err)
	}

	// We can exit here if we don't have the announcement yet.
	if !node.HaveNodeAnnouncement {
		return nodeID, nil
	}

	// Otherwise, we insert the v1 data.
	nodeV1Params := sqlc.UpsertV1NodeDataParams{
		NodeID:     nodeID,
		LastUpdate: node.LastUpdate.Unix(),
		Color:      EncodeHexColor(node.Color),
	}

	err = db.UpsertV1NodeData(ctx, nodeV1Params)
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

	extra, err := marshalExtraOpaqueData(node.ExtraOpaqueData)
	if err != nil {
		return 0, fmt.Errorf("unable to marshal extra opaque data: %w",
			err)
	}

	err = upsertNodeExtraSignedFields(ctx, db, nodeID, extra)
	if err != nil {
		return 0, fmt.Errorf("inserting node extra TLVs: %w", err)
	}

	return nodeID, nil
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
		if _, ok := newFeatures[feature.Bit]; ok {
			delete(newFeatures, feature.Bit)
			continue
		}

		// The feature is no longer present, so we remove it from the
		// database.
		err := db.DeleteNodeFeature(ctx, sqlc.DeleteNodeFeatureParams{
			NodeID:    nodeID,
			FeatureID: feature.FeatureID,
		})
		if err != nil {
			return fmt.Errorf("unable to delete node(%d) "+
				"feature(%v): %w", nodeID, feature.FeatureID,
				err)
		}
	}

	// Any remaining entries in newFeatures are new features that need to be
	// added to the database for the first time.
	for feature := range newFeatures {
		// Make sure an entry exists for this feature.
		featureID, err := db.CreateFeature(ctx, feature)
		if err != nil {
			return fmt.Errorf("unable to create feature(%v): %w",
				feature, err)
		}

		err = db.InsertNodeFeature(ctx, sqlc.InsertNodeFeatureParams{
			NodeID:    nodeID,
			FeatureID: featureID,
		})
		if err != nil {
			return fmt.Errorf("unable to insert node(%d) "+
				"feature(%v): %w", nodeID, feature, err)
		}
	}

	return nil
}

type dbAddressType uint8

const (
	addressTypeIPv4   dbAddressType = 1
	addressTypeIPv6   dbAddressType = 2
	addressTypeTorV2  dbAddressType = 3
	addressTypeTorV3  dbAddressType = 4
	addressTypeOpaque dbAddressType = math.MaxInt8
)

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

func updateNode(ctx context.Context, db SQLQueries, id int64,
	node *models.LightningNode) error {

	nodeParams := sqlc.UpdateNodeParams{
		ID: id,
		Alias: sql.NullString{
			Valid:  true,
			String: node.Alias,
		},
		Signature: node.AuthSigBytes,
	}

	err := db.UpdateNode(ctx, nodeParams)
	if err != nil {
		return err
	}

	nodeV1Params := sqlc.UpsertV1NodeDataParams{
		NodeID:     id,
		LastUpdate: node.LastUpdate.Unix(),
		Color:      EncodeHexColor(node.Color),
	}

	err = db.UpsertV1NodeData(ctx, nodeV1Params)
	if err != nil {
		return fmt.Errorf("updating node v1 params: %w", err)
	}

	err = upsertNodeFeatures(ctx, db, id, node.Features)
	if err != nil {
		return fmt.Errorf("inserting node features: %w", err)
	}

	err = upsertNodeAddresses(ctx, db, id, node.Addresses)
	if err != nil {
		return fmt.Errorf("inserting node addresses: %w", err)
	}

	extra, err := marshalExtraOpaqueData(node.ExtraOpaqueData)
	if err != nil {
		return fmt.Errorf("unable to marshal extra opaque data: %w",
			err)
	}

	err = upsertNodeExtraSignedFields(ctx, db, id, extra)
	if err != nil {
		return fmt.Errorf("upserting node extra TLVs: %w", err)
	}

	return nil
}

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

func fetchNodeByPubKey(ctx context.Context, db SQLQueries,
	pubKey route.Vertex) (*models.LightningNode, error) {

	dbNode, err := db.GetNodeByPubKeyAndVersion(
		ctx, sqlc.GetNodeByPubKeyAndVersionParams{
			PubKey:  pubKey[:],
			Version: ProtocolV1,
		},
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrGraphNodeNotFound
	} else if err != nil {
		return nil, fmt.Errorf("unable to fetch node: %w", err)
	}

	return buildNode(ctx, db, &dbNode)
}

func buildNode(ctx context.Context, db SQLQueries, dbNode *sqlc.Node) (
	*models.LightningNode, error) {

	if dbNode.Version != ProtocolV1 {
		return nil, fmt.Errorf("unsupported node version: %d",
			dbNode.Version)
	}

	var pub [33]byte
	copy(pub[:], dbNode.PubKey)

	node := &models.LightningNode{
		PubKeyBytes:          pub,
		Alias:                dbNode.Alias.String,
		HaveNodeAnnouncement: len(dbNode.Signature) > 0,
		AuthSigBytes:         dbNode.Signature,
		Features:             lnwire.EmptyFeatureVector(),
		LastUpdate:           time.Unix(0, 0),
		ExtraOpaqueData:      make([]byte, 0),
	}

	if !node.HaveNodeAnnouncement {
		return node, nil
	}

	v1Node, err := db.GetV1NodeData(ctx, dbNode.ID)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch node(%d) v1 data: %w",
			dbNode.ID, err)
	}

	node.LastUpdate = time.Unix(v1Node.LastUpdate, 0)
	node.Color, err = DecodeHexColor(v1Node.Color)
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
	node.Addresses, err = getNodeAddresses(ctx, db, dbNode.ID)
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

	if recs != nil {
		node.ExtraOpaqueData = recs
	}

	return node, nil
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
		features.Set(lnwire.FeatureBit(feature.Bit))
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
		switch dbAddressType(addr.Type) {
		case addressTypeIPv4:
			tcp, err := net.ResolveTCPAddr("tcp4", addr.Address)
			if err != nil {
				return nil, err
			}
			tcp.IP = tcp.IP.To4()

			addresses = append(addresses, tcp)

		case addressTypeIPv6:
			tcp, err := net.ResolveTCPAddr("tcp6", addr.Address)
			if err != nil {
				return nil, err
			}
			addresses = append(addresses, tcp)

		case addressTypeTorV3, addressTypeTorV2: // TODO(elle): test both types.
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

		case addressTypeOpaque:
			opaque, err := hex.DecodeString(addr.Address)
			if err != nil {
				return nil, fmt.Errorf("unable to decode opaque "+
					"address: %v", addr)
			}

			addresses = append(addresses, &lnwire.OpaqueAddrs{
				Payload: opaque,
			})

		default:
			return nil, fmt.Errorf("unknown address type: %v",
				addr.Type)
		}
	}

	return addresses, nil
}

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

func (s *SQLStore) insertChannel(ctx context.Context, db SQLQueries,
	edge *models.ChannelEdgeInfo) error {

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

	var chanIDB [8]byte
	byteOrder.PutUint64(chanIDB[:], edge.ChannelID)

	dbChanID, err := db.CreateChannel(
		ctx, sqlc.CreateChannelParams{
			Version:   ProtocolV1,
			ChannelID: chanIDB[:],
			NodeID1:   node1DBID,
			NodeID2:   node2DBID,
			Outpoint:  edge.ChannelPoint.String(),
			Capacity:  int64(edge.Capacity),
		},
	)
	if err != nil {
		return fmt.Errorf("unable to insert channel: %w", err)
	}

	err = db.CreateChannelsV1Data(ctx, sqlc.CreateChannelsV1DataParams{
		ChannelID:   dbChanID,
		BitcoinKey1: edge.BitcoinKey1Bytes[:],
		BitcoinKey2: edge.BitcoinKey2Bytes[:],
	})
	if err != nil {
		return fmt.Errorf("unable to insert channel v1 data: %w", err)
	}

	if edge.AuthProof != nil {
		proof := edge.AuthProof

		err = db.CreateV1ChannelProof(
			ctx, sqlc.CreateV1ChannelProofParams{
				ChannelID:         dbChanID,
				Node1Signature:    proof.NodeSig1Bytes,
				Node2Signature:    proof.NodeSig2Bytes,
				Bitcoin1Signature: proof.BitcoinSig1Bytes,
				Bitcoin2Signature: proof.BitcoinSig2Bytes,
			},
		)
		if err != nil {
			return fmt.Errorf("unable to insert channel proof: %w",
				err)
		}
	}

	if len(edge.Features) != 0 {
		chanFeatures := lnwire.NewRawFeatureVector()
		err := chanFeatures.Decode(bytes.NewReader(edge.Features))
		if err != nil {
			return err
		}

		fv := lnwire.NewFeatureVector(chanFeatures, lnwire.Features)
		for feature := range fv.Features() {
			featureID, err := db.CreateFeature(ctx, int32(feature))
			if err != nil {
				return fmt.Errorf("unable to create "+
					"feature(%v): %w", feature, err)
			}

			err = db.InsertChannelFeature(
				ctx, sqlc.InsertChannelFeatureParams{
					ChannelID: dbChanID,
					FeatureID: featureID,
				},
			)
			if err != nil {
				return fmt.Errorf("unable to insert "+
					"channel(%d) feature(%v): %w", dbChanID,
					feature, err)
			}
		}
	}

	extra, err := marshalExtraOpaqueData(edge.ExtraOpaqueData)
	if err != nil {
		return fmt.Errorf("unable to marshal extra opaque data: %w",
			err)
	}

	err = upsertChannelExtraSignedFields(ctx, db, dbChanID, extra)
	if err != nil {
		return fmt.Errorf("unable to insert channel extra "+
			"types: %w", err)
	}

	return nil
}

func maybeCreateShellNode(ctx context.Context, db SQLQueries,
	pubKey route.Vertex) (int64, error) {

	node, err := db.GetNodeByPubKeyAndVersion(
		ctx, sqlc.GetNodeByPubKeyAndVersionParams{
			PubKey:  pubKey[:],
			Version: ProtocolV1,
		},
	)
	// The node exists. Return the ID.
	if err == nil {
		return node.ID, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}

	// Otherwise, the node does not exist, so we create a shell entry for
	// it.
	id, err := db.CreateNode(ctx, sqlc.CreateNodeParams{
		Version: ProtocolV1,
		PubKey:  pubKey[:],
	})
	if err != nil {
		return 0, fmt.Errorf("unable to create shell node: %w", err)
	}

	return id, nil
}

func upsertChannelExtraSignedFields(ctx context.Context, db SQLQueries,
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
func insertChanPolicy(ctx context.Context, db SQLQueries, chanID, nodeID int64,
	policy *models.ChannelEdgePolicy) error {

	/*
		1) insert general chan policy [x]
		2) insert v1 data [x]
		3) insert extra data
	*/

	id, err := db.CreateChannelPolicy(ctx, sqlc.CreateChannelPolicyParams{
		ChannelID:   chanID,
		NodeID:      nodeID,
		Timelock:    int32(policy.TimeLockDelta),
		FeePpm:      int64(policy.FeeProportionalMillionths),
		BaseFeeMsat: int64(policy.FeeBaseMSat),
		MinHtlcMsat: int64(policy.MinHTLC),
		Signature:   policy.SigBytes,
	})
	if err != nil {
		return fmt.Errorf("unable to insert channel policy: %w", err)
	}

	err = db.CreateChannelPolicyV1Data(ctx, sqlc.CreateChannelPolicyV1DataParams{
		ChannelPolicyID: id,
		LastUpdate:      policy.LastUpdate.Unix(),
		Disabled:        policy.IsDisabled(),
		MaxHtlcMsat: sql.NullInt64{
			Valid: policy.MessageFlags.HasMaxHtlc(),
			Int64: int64(policy.MaxHTLC),
		},
	})

	extra, err := marshalExtraOpaqueData(policy.ExtraOpaqueData)
	if err != nil {
		return fmt.Errorf("unable to marshal extra opaque data: %w",
			err)
	}

	return upsertChanPolicyExtraSignedFields(ctx, db, id, extra)
}

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
		err = db.AddChannelPolicyExtraType(
			ctx, sqlc.AddChannelPolicyExtraTypeParams{
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

func updateChanPolicy(ctx context.Context, db SQLQueries,
	dbID int64, policy *models.ChannelEdgePolicy) error {

	err := db.UpdateChannelPolicy(ctx, sqlc.UpdateChannelPolicyParams{
		ID:          dbID,
		Timelock:    int32(policy.TimeLockDelta),
		FeePpm:      int64(policy.FeeProportionalMillionths),
		BaseFeeMsat: int64(policy.FeeBaseMSat),
		MinHtlcMsat: int64(policy.MinHTLC),
		Signature:   policy.SigBytes,
	})
	if err != nil {
		return fmt.Errorf("unable to update channel policy: %w", err)
	}

	err = db.UpdateChannelPolicyV1Data(
		ctx, sqlc.UpdateChannelPolicyV1DataParams{
			ChannelPolicyID: dbID,
			LastUpdate:      policy.LastUpdate.Unix(),
			Disabled:        policy.IsDisabled(),
			MaxHtlcMsat: sql.NullInt64{
				Valid: policy.MessageFlags.HasMaxHtlc(),
				Int64: int64(policy.MaxHTLC),
			},
		},
	)
	if err != nil {
		return fmt.Errorf("unable to update channel policy v1 data: %w",
			err)
	}

	extra, err := marshalExtraOpaqueData(policy.ExtraOpaqueData)
	if err != nil {
		return fmt.Errorf("unable to marshal extra opaque data: %w",
			err)
	}

	return upsertChanPolicyExtraSignedFields(ctx, db, dbID, extra)
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
		features.Set(lnwire.FeatureBit(feature.Bit))
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

func getChanPolicy(ctx context.Context, db SQLQueries, channelID uint64,
	dbChanID, dbNodeID int64,
	toNode route.Vertex, isNode1 bool) (*models.ChannelEdgePolicy, error) {

	policy, err := db.GetChannelPolicyByChannelAndNode(
		ctx, sqlc.GetChannelPolicyByChannelAndNodeParams{
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

	v1Data, err := db.GetChannelPolicyV1Data(ctx, policy.ID)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch v1 channel policy "+
			"data: %w", err)
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
	if v1Data.MaxHtlcMsat.Valid {
		msgFlags |= lnwire.ChanUpdateRequiredMaxHtlc
	}

	var chanFlags lnwire.ChanUpdateChanFlags
	if !isNode1 {
		chanFlags |= lnwire.ChanUpdateDirection
	}
	if v1Data.Disabled {
		chanFlags |= lnwire.ChanUpdateDisabled
	}

	return &models.ChannelEdgePolicy{
		SigBytes:                  policy.Signature,
		ChannelID:                 channelID,
		LastUpdate:                time.Unix(v1Data.LastUpdate, 0),
		MessageFlags:              msgFlags,
		ChannelFlags:              chanFlags,
		TimeLockDelta:             uint16(policy.Timelock),
		MinHTLC:                   lnwire.MilliSatoshi(policy.MinHtlcMsat),
		MaxHTLC:                   lnwire.MilliSatoshi(v1Data.MaxHtlcMsat.Int64),
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
