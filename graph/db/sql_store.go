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
	"strconv"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
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
type SQLQueries interface {
	/*
		Node queries.
	*/
	CreateNode(ctx context.Context, arg sqlc.CreateNodeParams) (int64, error)
	UpdateNode(ctx context.Context, arg sqlc.UpdateNodeParams) error
	GetNodeByID(ctx context.Context, id int64) (sqlc.Node, error)
	GetNodeByPubKeyAndVersion(ctx context.Context, arg sqlc.GetNodeByPubKeyAndVersionParams) (sqlc.Node, error)
	GetNodeAliasByPubKeyAndVersion(ctx context.Context, arg sqlc.GetNodeAliasByPubKeyAndVersionParams) (sql.NullString, error)
	DeleteNode(ctx context.Context, id int64) error

	UpsertNodeExtraType(ctx context.Context, arg sqlc.UpsertNodeExtraTypeParams) error
	GetExtraNodeTypes(ctx context.Context, nodeID int64) ([]sqlc.NodeExtraType, error)
	DeleteExtraNodeType(ctx context.Context, arg sqlc.DeleteExtraNodeTypeParams) error

	InsertNodeFeature(ctx context.Context, arg sqlc.InsertNodeFeatureParams) error
	GetNodeFeatures(ctx context.Context, nodeID int64) ([]sqlc.GetNodeFeaturesRow, error)
	DeleteNodeFeature(ctx context.Context, arg sqlc.DeleteNodeFeatureParams) error

	InsertNodeAddress(ctx context.Context, arg sqlc.InsertNodeAddressParams) error
	GetNodeAddresses(ctx context.Context, nodeID int64) ([]sqlc.GetNodeAddressesRow, error)
	DeleteNodeAddresses(ctx context.Context, nodeID int64) error

	UpsertV1NodeData(ctx context.Context, arg sqlc.UpsertV1NodeDataParams) error
	GetV1NodeData(ctx context.Context, nodeID int64) (sqlc.NodesV1Datum, error)

	/*
		Feature queries.
	*/
	CreateFeature(ctx context.Context, bit int32) (int64, error)

	/*
		Channel queries.
	*/
	CreateChannel(ctx context.Context, arg sqlc.CreateChannelParams) (int64, error)
	CreateChannelsV1Data(ctx context.Context, arg sqlc.CreateChannelsV1DataParams) error
	CreateV1ChannelProof(ctx context.Context, arg sqlc.CreateV1ChannelProofParams) error
	DeleteExtraChannelType(ctx context.Context, arg sqlc.DeleteExtraChannelTypeParams) error
	GetChannelBySCIDAndVersion(ctx context.Context, arg sqlc.GetChannelBySCIDAndVersionParams) (sqlc.Channel, error)
	GetExtraChannelTypes(ctx context.Context, channelID int64) ([]sqlc.ChannelExtraType, error)
	InsertChannelFeature(ctx context.Context, arg sqlc.InsertChannelFeatureParams) error
	UpsertChannelExtraType(ctx context.Context, arg sqlc.UpsertChannelExtraTypeParams) error
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
	db BatchedSQLQueries

	// Temporary fall-back to the KVStore so that we can implement the
	// interface incrementally.
	*KVStore
}

// A compile-time assertion to ensure that SQLStore implements the V1Store
// interface.
var _ V1Store = (*SQLStore)(nil)

// NewSQLStore creates a new SQLStore instance given an open BatchedSQLQueries
// storage backend.
func NewSQLStore(db BatchedSQLQueries, kvStore *KVStore) *SQLStore {
	return &SQLStore{
		db:      db,
		KVStore: kvStore,
	}
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

// AddLightningNode adds a vertex/node to the graph database. If the node is not
// in the database from before, this will add a new, unconnected one to the
// graph. If it is present from before, this will update that node's
// information. Note that this method is expected to only be called to update an
// already present node from a node announcement, or to insert a node found in a
// channel update.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) AddLightningNode(node *models.LightningNode,
	_ ...batch.SchedulerOption) error {

	ctx := context.TODO()

	var writeTxOpts TxOptions
	err := s.db.ExecTx(ctx, &writeTxOpts, func(db SQLQueries) error {
		_, err := upsertV1Node(ctx, db, node)
		return err
	}, func() {})
	if err != nil {
		return fmt.Errorf("unable to add node(%x): %w",
			node.PubKeyBytes, err)
	}

	return nil
}

// FetchLightningNode attempts to look up a target node by its identity public
// key. If the node isn't found in the database, then ErrGraphNodeNotFound is
// returned.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) FetchLightningNode(pubKey route.Vertex) (
	*models.LightningNode, error) {

	ctx := context.TODO()

	var (
		readTx = NewReadTx()
		node   *models.LightningNode
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		var err error
		_, node, err = getNodeByPubKey(ctx, db, pubKey, ProtocolV1)

		return err
	}, func() {})
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
		readTx     = NewReadTx()
		exists     bool
		lastUpdate time.Time
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		dbNode, err := db.GetNodeByPubKeyAndVersion(
			ctx, sqlc.GetNodeByPubKeyAndVersionParams{
				PubKey:  pubKey[:],
				Version: int16(ProtocolV1),
			},
		)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		} else if err != nil {
			return fmt.Errorf("unable to fetch node: %w", err)
		}

		exists = true

		v1Node, err := db.GetV1NodeData(ctx, dbNode.ID)
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

// AddrsForNode returns all known addresses for the target node public key
// that the graph DB is aware of. The returned boolean indicates if the
// given node is unknown to the graph DB or not.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) AddrsForNode(nodePub *btcec.PublicKey) (bool, []net.Addr,
	error) {

	ctx := context.TODO()

	var (
		readTx    = NewReadTx()
		addresses []net.Addr
		known     bool
	)
	err := s.db.ExecTx(ctx, &readTx, func(db SQLQueries) error {
		dbNode, err := db.GetNodeByPubKeyAndVersion(
			ctx, sqlc.GetNodeByPubKeyAndVersionParams{
				PubKey:  nodePub.SerializeCompressed(),
				Version: int16(ProtocolV1),
			},
		)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		} else if err != nil {
			return fmt.Errorf("unable to fetch node: %w", err)
		}

		known = true

		addresses, err = getNodeAddresses(ctx, db, dbNode.ID)
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

// DeleteLightningNode starts a new database transaction to remove a vertex/node
// from the database according to the node's public key.
//
// NOTE: part of the V1Store interface.
func (s *SQLStore) DeleteLightningNode(pubKey route.Vertex) error {
	ctx := context.TODO()

	var writeTxOpts TxOptions
	err := s.db.ExecTx(ctx, &writeTxOpts, func(db SQLQueries) error {
		dbNode, err := db.GetNodeByPubKeyAndVersion(
			ctx, sqlc.GetNodeByPubKeyAndVersionParams{
				PubKey:  pubKey[:],
				Version: int16(ProtocolV1),
			},
		)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrGraphNodeNotFound
		} else if err != nil {
			return fmt.Errorf("unable to fetch node: %w", err)
		}

		return db.DeleteNode(ctx, dbNode.ID)
	}, func() {})
	if err != nil {
		return fmt.Errorf("unable to delete node: %w", err)
	}

	return nil
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
		_, err := db.GetChannelBySCIDAndVersion(
			ctx, sqlc.GetChannelBySCIDAndVersionParams{
				Scid:    chanIDB[:],
				Version: int16(ProtocolV1),
			})
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil {
			return ErrEdgeAlreadyExist
		}

		_, err = insertChannel(ctx, db, edge)

		return err
	}, func() {})
	if err != nil {
		return err
	}

	s.rejectCache.remove(edge.ChannelID)
	s.chanCache.remove(edge.ChannelID)

	return nil
}

// LookupAlias attempts to return the alias as advertised by the target node.
//
// NOTE: part of the V1Store interface.
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
				Version: int16(ProtocolV1),
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
		return "", fmt.Errorf("unable to look up alias: %w", err)
	}

	return alias, nil
}

// upsertV1Node first checks if an entry for this V1 node already exists in
// the database. If it does, it updates the existing record. If it doesn't,
// it creates a new record. It returns the node ID of the node in the
// database.
func upsertV1Node(ctx context.Context, db SQLQueries,
	node *models.LightningNode) (int64, error) {

	// First, check if this node already exists.
	dbNode, err := db.GetNodeByPubKeyAndVersion(
		ctx, sqlc.GetNodeByPubKeyAndVersionParams{
			PubKey:  node.PubKeyBytes[:],
			Version: int16(ProtocolV1),
		},
	)
	switch {
	// The node does not yet exist in the DB, so we insert a fresh record.
	case errors.Is(err, sql.ErrNoRows):
		return insertV1Node(ctx, db, node)

	// The node already exists, so we update the existing node info.
	case err == nil:
		return dbNode.ID, updateV1Node(ctx, db, dbNode.ID, node)

	default:
		return 0, fmt.Errorf("unable to fetch node(%x): %w",
			node.PubKeyBytes, err)
	}
}

// insertV1Node creates a new V1 node record.
func insertV1Node(ctx context.Context, db SQLQueries,
	node *models.LightningNode) (int64, error) {

	nodeID, err := db.CreateNode(ctx, sqlc.CreateNodeParams{
		Version: int16(ProtocolV1),
		PubKey:  node.PubKeyBytes[:],
		Alias: sql.NullString{
			Valid:  node.Alias != "",
			String: node.Alias,
		},
		Signature: node.AuthSigBytes,
	})
	if err != nil {
		return 0, fmt.Errorf("creating record for "+
			"node(%x): %w", node.PubKeyBytes, err)
	}

	err = upsertV1NodeData(ctx, db, nodeID, node)
	if err != nil {
		return 0, fmt.Errorf("inserting data for node(%x): %w",
			node.PubKeyBytes, err)
	}

	return nodeID, nil
}

// updateV1Node updates the existing V1 node record and all its associated data.
func updateV1Node(ctx context.Context, db SQLQueries, id int64,
	node *models.LightningNode) error {

	err := db.UpdateNode(ctx, sqlc.UpdateNodeParams{
		ID: id,
		Alias: sql.NullString{
			Valid:  node.Alias != "",
			String: node.Alias,
		},
		Signature: node.AuthSigBytes,
	})
	if err != nil {
		return fmt.Errorf("updating node(%x): %w", node.PubKeyBytes,
			err)
	}

	err = upsertV1NodeData(ctx, db, id, node)
	if err != nil {
		return fmt.Errorf("updating data for node(%x): %w",
			node.PubKeyBytes, err)
	}

	return nil
}

// upsertV1NodeData updates the V1 node data for the given node ID. This
// includes updating v1 specific fields, the node's features, addresses and
// extra TLV types.
func upsertV1NodeData(ctx context.Context, db SQLQueries, id int64,
	node *models.LightningNode) error {

	// We can exit here if we don't have the announcement yet.
	if !node.HaveNodeAnnouncement {
		return nil
	}

	// Otherwise, we insert the v1 data.
	err := db.UpsertV1NodeData(ctx, sqlc.UpsertV1NodeDataParams{
		NodeID:     id,
		LastUpdate: node.LastUpdate.Unix(),
		Color:      EncodeHexColor(node.Color),
	})
	if err != nil {
		return fmt.Errorf("inserting node: %w", err)
	}

	// Update the node's features.
	err = upsertNodeFeatures(ctx, db, id, node.Features)
	if err != nil {
		return fmt.Errorf("inserting node features: %w", err)
	}

	// Update the node's addresses.
	err = upsertNodeAddresses(ctx, db, id, node.Addresses)
	if err != nil {
		return fmt.Errorf("inserting node addresses: %w", err)
	}

	// Convert the flat extra opaque data into a map of TLV types to
	// values.
	extra, err := marshalExtraOpaqueData(node.ExtraOpaqueData)
	if err != nil {
		return fmt.Errorf("unable to marshal extra opaque data: %w",
			err)
	}

	// Update the node's extra signed fields.
	err = upsertNodeExtraSignedFields(ctx, db, id, extra)
	if err != nil {
		return fmt.Errorf("inserting node extra TLVs: %w", err)
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

func getNodeByPubKey(ctx context.Context, db SQLQueries,
	pubKey route.Vertex, version ProtocolVersion) (int64,
	*models.LightningNode, error) {

	dbNode, err := db.GetNodeByPubKeyAndVersion(
		ctx, sqlc.GetNodeByPubKeyAndVersionParams{
			PubKey:  pubKey[:],
			Version: int16(version),
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

	dbNode, err := db.GetNodeByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("unable to get node(id=%d): %w", id, err)
	}

	return buildNode(ctx, db, &dbNode)
}

func buildNode(ctx context.Context, db SQLQueries, dbNode *sqlc.Node) (
	*models.LightningNode, error) {

	if dbNode.Version != int16(ProtocolV1) {
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

type dbChanInfo struct {
	channelID int64
	node1ID   int64
	node2ID   int64
}

func insertChannel(ctx context.Context, db SQLQueries,
	edge *models.ChannelEdgeInfo) (*dbChanInfo, error) {

	// Make sure that at least a "shell" entry for each node is present in
	// the nodes table.
	node1DBID, err := maybeCreateShellNode(ctx, db, edge.NodeKey1Bytes)
	if err != nil {
		return nil, fmt.Errorf("unable to create shell node: %w", err)
	}

	node2DBID, err := maybeCreateShellNode(ctx, db, edge.NodeKey2Bytes)
	if err != nil {
		return nil, fmt.Errorf("unable to create shell node: %w", err)
	}

	var chanIDB [8]byte
	byteOrder.PutUint64(chanIDB[:], edge.ChannelID)

	dbChanID, err := db.CreateChannel(
		ctx, sqlc.CreateChannelParams{
			Version:  int16(ProtocolV1),
			Scid:     chanIDB[:],
			NodeID1:  node1DBID,
			NodeID2:  node2DBID,
			Outpoint: edge.ChannelPoint.String(),
			Capacity: int64(edge.Capacity),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("unable to insert channel: %w", err)
	}

	err = db.CreateChannelsV1Data(ctx, sqlc.CreateChannelsV1DataParams{
		ChannelID:   dbChanID,
		BitcoinKey1: edge.BitcoinKey1Bytes[:],
		BitcoinKey2: edge.BitcoinKey2Bytes[:],
	})
	if err != nil {
		return nil, fmt.Errorf("unable to insert channel v1 data: %w",
			err)
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
			return nil, fmt.Errorf("unable to insert channel "+
				"proof: %w", err)
		}
	}

	if len(edge.Features) != 0 {
		chanFeatures := lnwire.NewRawFeatureVector()
		err := chanFeatures.Decode(bytes.NewReader(edge.Features))
		if err != nil {
			return nil, err
		}

		fv := lnwire.NewFeatureVector(chanFeatures, lnwire.Features)
		for feature := range fv.Features() {
			featureID, err := db.CreateFeature(ctx, int32(feature))
			if err != nil {
				return nil, fmt.Errorf("unable to create "+
					"feature(%v): %w", feature, err)
			}

			err = db.InsertChannelFeature(
				ctx, sqlc.InsertChannelFeatureParams{
					ChannelID: dbChanID,
					FeatureID: featureID,
				},
			)
			if err != nil {
				return nil, fmt.Errorf("unable to insert "+
					"channel(%d) feature(%v): %w", dbChanID,
					feature, err)
			}
		}
	}

	extra, err := marshalExtraOpaqueData(edge.ExtraOpaqueData)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal extra opaque data: %w",
			err)
	}

	err = upsertChannelExtraSignedFields(ctx, db, dbChanID, extra)
	if err != nil {
		return nil, fmt.Errorf("unable to insert channel extra "+
			"types: %w", err)
	}

	return &dbChanInfo{
		channelID: dbChanID,
		node1ID:   node1DBID,
		node2ID:   node2DBID,
	}, nil
}

func maybeCreateShellNode(ctx context.Context, db SQLQueries,
	pubKey route.Vertex) (int64, error) {

	dbNode, err := db.GetNodeByPubKeyAndVersion(
		ctx, sqlc.GetNodeByPubKeyAndVersionParams{
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
	id, err := db.CreateNode(ctx, sqlc.CreateNodeParams{
		Version: int16(ProtocolV1),
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
