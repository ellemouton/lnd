package graphdb

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/davecgh/go-spew/spew"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/sqldb/sqlc"
	"github.com/pmezard/go-difflib/difflib"
)

// ErrMigrationMismatch is returned when a migrated graph record does not
// match the original record.
var ErrMigrationMismatch = fmt.Errorf("migrated graph record does not match " +
	"original record")

func MigrateGraphToSQL(ctx context.Context, kvBackend kvdb.Backend,
	tx *sqlc.Queries, chain chainhash.Hash) error {

	log.Infof("Starting migration of the graph store from KV to SQL")

	sqlDB := SQLQueries(tx)

	// 1) Migrate all the nodes.
	if err := migrateNodes(ctx, kvBackend, sqlDB); err != nil {
		return fmt.Errorf("could not migrate nodes: %w", err)
	}

	// 2) Migrate the source node.
	if err := migrateSourceNode(ctx, kvBackend, sqlDB); err != nil {
		return fmt.Errorf("could not migrate source node: %w", err)
	}

	// 3) Migrate all the channels and channel policies.
	err := migrateChannelsAndPolicies(ctx, kvBackend, sqlDB, chain)
	if err != nil {
		return fmt.Errorf("could not migrate channels and policies: %w",
			err)
	}

	// 4) Migrate the Prune log.
	if err := migratePruneLog(ctx, kvBackend, sqlDB); err != nil {
		return fmt.Errorf("could not migrate prune log: %w", err)
	}

	// 5) Migrate the closed SCID index.
	err = migrateClosedSCIDIndex(ctx, kvBackend, sqlDB)
	if err != nil {
		return fmt.Errorf("could not migrate closed SCID index: %w",
			err)
	}

	// 6) Migrate the zombie index.
	if err := migrateZombieIndex(ctx, kvBackend, sqlDB); err != nil {
		return fmt.Errorf("could not migrate zombie index: %w", err)
	}

	log.Infof("Finished migration of the graph store from KV to SQL")

	return nil
}

func migrateNodes(ctx context.Context, kvBackend kvdb.Backend,
	sqlDB SQLQueries) error {

	// Loop through each node in the KV store and insert it into the SQL
	// database.
	return forEachNode(kvBackend, func(_ kvdb.RTx,
		node *models.LightningNode) error {

		// Migrate the single node record.
		id, err := upsertNode(ctx, sqlDB, node)
		if err != nil {
			return fmt.Errorf("could not migrate node(%x): %w",
				node.PubKeyBytes, err)
		}

		// Fetch it from the SQL store and compare it against the
		// original node object to ensure the migration was successful.
		dbNode, err := sqlDB.GetNodeByPubKey(
			ctx, sqlc.GetNodeByPubKeyParams{
				PubKey:  node.PubKeyBytes[:],
				Version: int16(ProtocolV1),
			},
		)
		if err != nil {
			return fmt.Errorf("could not get node by pubkey (%x)"+
				"after migration: %w", node.PubKeyBytes, err)
		}

		// Sanity check: ensure the migrated node ID matches
		// the one we just inserted.
		if dbNode.ID != id {
			return fmt.Errorf("node ID mismatch after migration: "+
				"expected %d, got %d", id, dbNode.ID)
		}

		migratedNode, err := buildNode(ctx, sqlDB, &dbNode)
		if err != nil {
			return fmt.Errorf("could not build migrated node "+
				"from dbNode: %w", err)
		}

		return compare(
			node, migratedNode,
			fmt.Sprintf("node %x", node.PubKeyBytes),
		)
	})
}

func compare(original, migrated any, identifier string) error {
	if !reflect.DeepEqual(original, migrated) {
		diff := difflib.UnifiedDiff{
			A: difflib.SplitLines(
				spew.Sdump(original),
			),
			B: difflib.SplitLines(
				spew.Sdump(migrated),
			),
			FromFile: "Expected",
			FromDate: "",
			ToFile:   "Actual",
			ToDate:   "",
			Context:  3,
		}
		diffText, _ := difflib.GetUnifiedDiffString(diff)

		return fmt.Errorf("%w: %s.\n%v", ErrMigrationMismatch,
			identifier, diffText)
	}

	return nil
}

func migrateSourceNode(ctx context.Context, kvdb kvdb.Backend,
	sqlDB SQLQueries) error {

	sourceNode, err := getSourceNode(kvdb)
	if errors.Is(err, ErrSourceNodeNotSet) {
		// If the source node has not been set yet, we can skip this
		// migration step.
		return nil
	} else if err != nil {
		return fmt.Errorf("could not get source node: %w", err)
	}

	// Get the DB ID of the source node by its public key. This node must
	// already exist in the SQL database, as it should have been migrated
	// in the previous step.
	id, err := sqlDB.GetNodeIDByPubKey(
		ctx, sqlc.GetNodeIDByPubKeyParams{
			PubKey:  sourceNode.PubKeyBytes[:],
			Version: int16(ProtocolV1),
		},
	)
	if err != nil {
		return fmt.Errorf("could not get source node ID: %w", err)
	}

	// Now we can add the source node to the SQL database.
	err = sqlDB.AddSourceNode(ctx, id)
	if err != nil {
		return fmt.Errorf("could not add source node: %w", err)
	}

	// Verify that the source node was added correctly by fetching it back
	// from the SQL database and checking that the expected DB ID and
	// pub key are returned. We don't need to do a whole node comparison
	// here, as this was already done in the previous migration step.
	srcNodes, err := sqlDB.GetSourceNodesByVersion(ctx, int16(ProtocolV1))
	if err != nil {
		return fmt.Errorf("could not get source nodes: %w", err)
	}

	if len(srcNodes) != 1 {
		return fmt.Errorf("expected exactly one source node, "+
			"got %d", len(srcNodes))
	}

	if srcNodes[0].NodeID != id {
		return fmt.Errorf("source node ID mismatch after migration: "+
			"expected %d, got %d", id, srcNodes[0].NodeID)
	}

	return compare(
		sourceNode.PubKeyBytes[:], srcNodes[0].PubKey, "source node",
	)
}

func migrateChannelsAndPolicies(ctx context.Context, kvBackend kvdb.Backend,
	sqlDB SQLQueries, chain chainhash.Hash) error {

	migrateChanPolicy := func(dbInfo *dbChanInfo,
		policy *models.ChannelEdgePolicy) error {

		if policy == nil {
			return nil
		}

		_, _, _, err := updateChanEdgePolicy(ctx, sqlDB, policy)

		return err
	}

	// Next, migrate all channels and channel updates
	return forEachChannel(kvBackend,
		func(channel *models.ChannelEdgeInfo,
			policy1 *models.ChannelEdgePolicy,
			policy2 *models.ChannelEdgePolicy) error {

			scid := channel.ChannelID

			// First, migrate the channel info along with its
			// policies.
			dbChanInfo, err := insertChannel(ctx, sqlDB, channel)
			if err != nil {
				return fmt.Errorf("could not migrate "+
					"channel(%d): %w", scid, err)
			}

			err = migrateChanPolicy(dbChanInfo, policy1)
			if err != nil {
				return fmt.Errorf("could not migrate "+
					"policy1(%d): %w", scid, err)
			}

			err = migrateChanPolicy(dbChanInfo, policy2)
			if err != nil {
				return fmt.Errorf("could not migrate "+
					"policy2(%d): %w", scid, err)
			}

			// Now, fetch the channel and its policies from the SQL
			// DB.
			row, err := sqlDB.GetChannelBySCIDWithPolicies(
				ctx, sqlc.GetChannelBySCIDWithPoliciesParams{
					Scid:    channelIDToBytes(scid),
					Version: int16(ProtocolV1),
				},
			)
			if err != nil {
				return fmt.Errorf("could not get channel "+
					"by SCID(%d): %w", scid, err)
			}

			migChan, migPol1, migPol2, err := getAndBuildChanAndPolicies(
				ctx, sqlDB, row, chain,
			)
			if err != nil {
				return fmt.Errorf("could not build "+
					"migrated channel and policies: %w",
					err)
			}

			// Finally, compare the original channel info and
			// policies with the migrated ones to ensure they match.
			err = compare(
				channel, migChan,
				fmt.Sprintf("channel %d", scid),
			)
			if err != nil {
				return err
			}

			err = compare(
				policy1, migPol1,
				fmt.Sprintf("policy1 for channel %d", scid),
			)
			if err != nil {
				return err
			}

			return compare(
				policy2, migPol2,
				fmt.Sprintf("policy2 for channel %d", scid),
			)
		},
	)
}

func getAndBuildChanAndPolicies(ctx context.Context, db SQLQueries,
	row sqlc.GetChannelBySCIDWithPoliciesRow,
	chain chainhash.Hash) (*models.ChannelEdgeInfo,
	*models.ChannelEdgePolicy, *models.ChannelEdgePolicy, error) {

	node1, node2, err := buildNodeVertices(
		row.Node.PubKey, row.Node_2.PubKey,
	)
	if err != nil {
		return nil, nil, nil, err
	}

	edge, err := getAndBuildEdgeInfo(
		ctx, db, chain, row.Channel.ID, row.Channel, node1, node2,
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to build channel "+
			"info: %w", err)
	}

	dbPol1, dbPol2, err := extractChannelPolicies(row)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to extract channel "+
			"policies: %w", err)
	}

	policy1, policy2, err := getAndBuildChanPolicies(
		ctx, db, dbPol1, dbPol2, edge.ChannelID, node1, node2,
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to build channel "+
			"policies: %w", err)
	}

	return edge, policy1, policy2, nil
}

func migratePruneLog(ctx context.Context, kvBackend kvdb.Backend,
	sqlDB SQLQueries) error {

	return forEachPruneLogEntry(kvBackend,
		func(height uint32, hash *chainhash.Hash) error {
			err := sqlDB.UpsertPruneLogEntry(
				ctx, sqlc.UpsertPruneLogEntryParams{
					BlockHeight: int64(height),
					BlockHash:   hash[:],
				},
			)
			if err != nil {
				return fmt.Errorf("unable to insert prune log "+
					"entry for height %d: %w", height, err)
			}

			// Now, check that the entry was inserted correctly.
			migratedHash, err := sqlDB.GetPruneHashByHeight(
				ctx, int64(height),
			)
			if err != nil {
				return fmt.Errorf("could not get prune hash "+
					"by height %d: %w", height, err)
			}

			return compare(
				hash[:], migratedHash,
				fmt.Sprintf("prune log entry at height %d",
					height),
			)
		},
	)
}

func migrateClosedSCIDIndex(ctx context.Context, kvBackend kvdb.Backend,
	sqlDB SQLQueries) error {

	return forEachClosedSCID(kvBackend,
		func(scid lnwire.ShortChannelID) error {
			chanIDB := channelIDToBytes(scid.ToUint64())

			err := sqlDB.InsertClosedChannel(ctx, chanIDB[:])
			if err != nil {
				return fmt.Errorf("could not insert closed "+
					"channel with SCID %s: %w", scid, err)
			}

			// Now, verify that the channel with the given SCID is
			// seen as closed.
			isClosed, err := sqlDB.IsClosedChannel(ctx, chanIDB)
			if err != nil {
				return fmt.Errorf("could not check if "+
					"channel %s is closed: %w", scid, err)
			}

			if !isClosed {
				return fmt.Errorf("channel %s should be "+
					"closed, but is not", scid)
			}

			return nil
		},
	)
}

func migrateZombieIndex(ctx context.Context, kvBackend kvdb.Backend,
	sqlDB SQLQueries) error {

	return forEachZombieEntry(kvBackend, func(chanID uint64, pubKey1,
		pubKey2 [33]byte) error {

		chanIDB := channelIDToBytes(chanID)

		// If it is in the closed SCID index, we don't need to
		// add it to the zombie index.
		isClosed, err := sqlDB.IsClosedChannel(ctx, chanIDB)
		if err != nil {
			return fmt.Errorf("could not check closed "+
				"channel: %w", err)
		}

		if isClosed {
			return nil
		}

		err = sqlDB.UpsertZombieChannel(
			ctx, sqlc.UpsertZombieChannelParams{
				Version:  int16(ProtocolV1),
				Scid:     chanIDB[:],
				NodeKey1: pubKey1[:],
				NodeKey2: pubKey2[:],
			},
		)
		if err != nil {
			return fmt.Errorf("could not upsert zombie "+
				"channel %d: %w", chanID, err)
		}

		// Finally, verify that the channel is indeed marked as a
		// zombie channel.
		isZombie, err := sqlDB.IsZombieChannel(
			ctx, sqlc.IsZombieChannelParams{
				Version: int16(ProtocolV1),
				Scid:    chanIDB,
			},
		)
		if err != nil {
			return fmt.Errorf("could not check if "+
				"channel %d is zombie: %w", chanID, err)
		}

		if !isZombie {
			return fmt.Errorf("channel %d should be "+
				"a zombie, but is not", chanID)
		}

		return nil
	})
}

// forEachPruneLogEntry iterates over all entries in the kvdb prune log
// and calls the provided callback function for each entry. The callback
func forEachPruneLogEntry(db kvdb.Backend, cb func(height uint32,
	hash *chainhash.Hash) error) error {

	return kvdb.View(db, func(tx kvdb.RTx) error {
		metaBucket := tx.ReadBucket(graphMetaBucket)
		if metaBucket == nil {
			return ErrGraphNotFound
		}

		pruneBucket := metaBucket.NestedReadBucket(pruneLogBucket)
		if pruneBucket == nil {
			// The graph has never been pruned and so, there are no
			// entries to iterate over.
			return nil
		}

		return pruneBucket.ForEach(func(k, v []byte) error {
			blockHeight := byteOrder.Uint32(k)
			var blockHash chainhash.Hash
			copy(blockHash[:], v)

			return cb(blockHeight, &blockHash)
		})
	}, func() {})
}

func forEachClosedSCID(db kvdb.Backend,
	cb func(lnwire.ShortChannelID) error) error {

	return kvdb.View(db, func(tx kvdb.RTx) error {
		closedScids := tx.ReadBucket(closedScidBucket)
		if closedScids == nil {
			return nil
		}

		return closedScids.ForEach(func(k, _ []byte) error {
			return cb(lnwire.NewShortChanIDFromInt(
				byteOrder.Uint64(k),
			))
		})
	}, func() {})
}

func forEachZombieEntry(db kvdb.Backend, cb func(chanID uint64, pubKey1,
	pubKey2 [33]byte) error) error {

	return kvdb.View(db, func(tx kvdb.RTx) error {
		edges := tx.ReadBucket(edgeBucket)
		if edges == nil {
			return ErrGraphNoEdgesFound
		}
		zombieIndex := edges.NestedReadBucket(zombieBucket)
		if zombieIndex == nil {
			return nil
		}

		return zombieIndex.ForEach(func(k, v []byte) error {
			var pubKey1, pubKey2 [33]byte
			copy(pubKey1[:], v[:33])
			copy(pubKey2[:], v[33:])

			return cb(byteOrder.Uint64(k), pubKey1, pubKey2)
		})
	}, func() {})
}
