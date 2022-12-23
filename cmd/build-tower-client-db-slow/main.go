package main

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/signal"
	"github.com/lightningnetwork/lnd/watchtower/blob"
	"github.com/lightningnetwork/lnd/watchtower/wtdb"
	"github.com/lightningnetwork/lnd/watchtower/wtpolicy"
)

var interceptor signal.Interceptor

func main() {
	// Hook interceptor for os signals.
	shutdownInterceptor, err := signal.Intercept()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	t := newThings()
	t.setup()

	err = t.do()
	if err != nil {
		fmt.Printf("could not do things: %v\n", err)
	}

	<-shutdownInterceptor.ShutdownChannel()
	fmt.Println("received shutdown signal")

	if err := t.stop(); err != nil {
		log.Fatalln(err)
	}

	fmt.Println("Done")
}

type session struct {
	id     wtdb.SessionID
	seqNum uint16
}

type channel struct {
	id           lnwire.ChannelID
	commitHeight uint64
}

type things struct {
	db    *wtdb.ClientDB
	tower *wtdb.Tower

	channels []*channel
	chansMu  sync.Mutex

	activeSession *session
	sessionMu     sync.Mutex

	wg   sync.WaitGroup
	quit chan error
}

func newThings() *things {
	return &things{
		quit: make(chan error),
	}
}

func (t *things) setup() error {
	db, err := kvdb.GetBoltBackend(
		&kvdb.BoltBackendConfig{
			DBPath:         "/tmp/towerdb",
			DBFileName:     "wtclient.db",
			DBTimeout:      kvdb.DefaultDBTimeout,
			NoFreelistSync: true,
			AutoCompact:    false,
		},
	)
	if err != nil {
		return fmt.Errorf("error opening tower client DB: %v", err)
	}

	clientDb, err := wtdb.OpenClientDB(db)
	if err != nil {
		return err
	}
	t.db = clientDb

	// Create a tower if needed.
	towers, err := clientDb.ListTowers()
	if err != nil {
		return err
	}

	if len(towers) > 0 {
		t.tower = towers[0]
		return nil
	}

	towerAddr, err := net.ResolveTCPAddr("tcp", "0.0.0.0:80")
	if err != nil {
		return err
	}

	t.tower, err = clientDb.CreateTower(&lnwire.NetAddress{
		IdentityKey: randomPubKey(),
		Address:     towerAddr,
		ChainNet:    wire.MainNet,
	})
	if err != nil {
		return err
	}

	return nil
}

func (t *things) stop() error {
	close(t.quit)
	t.wg.Wait()

	return t.db.Close()
}

func (t *things) do() error {
	go t.openAndCloseChannelsForever()
	go t.makePaymentsForever()

	return nil
}

func (t *things) makePaymentsForever() error {
	t.wg.Add(1)
	defer t.wg.Done()
	/*
		- Choose random channel for the update. Do update and commit to
		  the active session.
		- Do this until the session is full (1024 updates).
		- Create new session and repeat.
	*/
	totalUpdates := 0

	for {
		select {
		case <-t.quit:
			return nil
		default:
		}

		// pick random channel.
		t.chansMu.Lock()
		if len(t.channels) == 0 {
			t.chansMu.Unlock()

			time.Sleep(time.Second)
			continue
		}

		i := rand.Intn(len(t.channels))
		channel := t.channels[i]

		t0 := time.Now()
		// Do 100 payments over this channel.
		for p := 0; p < 100; p++ {
			channel.commitHeight++

			backupID := wtdb.BackupID{
				ChanID:       channel.id,
				CommitHeight: channel.commitHeight,
			}

			err := t.ackUpdates(backupID)
			if err != nil {
				return err
			}

			totalUpdates++
		}
		t.chansMu.Unlock()
		fmt.Printf("100 acks took %f seconds\n", time.Since(t0).Seconds())

		fmt.Printf("done %d updates across %d sessions\n",
			totalUpdates, totalUpdates/1024)
	}
}

func (t *things) ackUpdates(backupID wtdb.BackupID) error {
	t.sessionMu.Lock()
	defer t.sessionMu.Unlock()

	if t.activeSession == nil || t.activeSession.seqNum == 1024 {
		t.createNewSession()
	}

	t.activeSession.seqNum++
	la, err := t.db.CommitUpdate(&t.activeSession.id, &wtdb.CommittedUpdate{
		SeqNum: t.activeSession.seqNum,
		CommittedUpdateBody: wtdb.CommittedUpdateBody{
			BackupID:      backupID,
			Hint:          randomBreachHint(),
			EncryptedBlob: []byte{2, 5, 6, 1},
		},
	})
	if err != nil {
		return err
	}

	return t.db.AckUpdate(&t.activeSession.id, t.activeSession.seqNum, la)
}

func (t *things) openAndCloseChannelsForever() {
	t.wg.Add(1)
	defer t.wg.Done()
	// Try always keep about 20 channels active at once.
	// Randomly tick: if num chans > 20 - close one
	// 		  if num chans < 20 - open one
	//                if num chans == 20 - close one OR skip

	t.openNewChannel()

	randomTicker := time.NewTicker(5 * time.Second)
	for {
		select {
		case <-randomTicker.C:
			t.chansMu.Lock()
			switch {
			case len(t.channels) < 20:
				t.openNewChannel()

			case len(t.channels) > 20:
				t.closeRandomChannel()

			default:
				i := rand.Intn(2)
				if i%2 == 0 {
					t.closeRandomChannel()
				}
			}
			fmt.Println("num channels: ", len(t.channels))

			t.chansMu.Unlock()

			i := rand.Intn(20) + 1
			randomTicker.Reset(time.Duration(i) * time.Second)

		case <-t.quit:
			return
		}
	}
}

func (t *things) openNewChannel() (lnwire.ChannelID, error) {
	chanID := newChannelID()
	fmt.Println("new channel", chanID)
	err := t.db.RegisterChannel(chanID, []byte{5, 6, 7})
	if err != nil {
		return lnwire.ChannelID{}, err
	}

	t.channels = append(t.channels, &channel{
		id:           chanID,
		commitHeight: 0,
	})

	return chanID, nil
}

func (t *things) closeRandomChannel() {
	if len(t.channels) == 0 {
		return
	}

	i := rand.Intn(len(t.channels))
	fmt.Println("closing channel", t.channels[i].id)
	t.channels = append(t.channels[:i], t.channels[i+1:]...)
}

func (t *things) createNewSession() error {
	keyIndex, err := t.db.NextSessionKeyIndex(
		t.tower.ID, blob.TypeAltruistCommit,
	)
	if err != nil {
		return fmt.Errorf("unable to reserve session key index "+
			"for tower=%x: %v", t.tower.IdentityKey, err)
	}

	sessionID := newSessionID()
	err = t.db.CreateClientSession(&wtdb.ClientSession{
		ID: sessionID,
		ClientSessionBody: wtdb.ClientSessionBody{
			TowerID:        t.tower.ID,
			KeyIndex:       keyIndex,
			Policy:         wtpolicy.DefaultPolicy(),
			Status:         1,
			RewardPkScript: []byte{1, 2, 3, 4, 5},
		},
	})
	if err != nil {
		return err
	}

	t.activeSession = &session{
		id:     sessionID,
		seqNum: 0,
	}

	return nil
}

func randomPubKey() *btcec.PublicKey {
	pk, err := btcec.NewPrivateKey()
	if err != nil {
		panic(err)
	}

	return pk.PubKey()
}

func randomBreachHint() blob.BreachHint {
	var hint blob.BreachHint
	b := randomBytes(16)
	copy(hint[:], b)

	return hint
}

func newChannelID() lnwire.ChannelID {
	var id lnwire.ChannelID
	b := randomBytes(32)
	copy(id[:], b)

	return id
}

func newSessionID() wtdb.SessionID {
	var s wtdb.SessionID
	b := randomBytes(33)
	copy(s[:], b)

	return s
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}

	return b
}
