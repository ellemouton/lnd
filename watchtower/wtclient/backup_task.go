package wtclient

import (
	"bytes"
	"fmt"
	"io"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/txsort"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/channeldb"
	"github.com/lightningnetwork/lnd/input"
	"github.com/lightningnetwork/lnd/lnwallet"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/lightningnetwork/lnd/watchtower/blob"
	"github.com/lightningnetwork/lnd/watchtower/wtdb"
)

const (
	idType         tlv.Type = 1
	breachInfoType tlv.Type = 2
	chanTypeType   tlv.Type = 3

	toLocalKeyType           tlv.Type = 1
	toRemoteKeyType          tlv.Type = 2
	revocationKeyType        tlv.Type = 3
	csvDelayType             tlv.Type = 4
	breachTxHashType         tlv.Type = 5
	remoteOutpointType       tlv.Type = 6
	remoteOutputSignDescType tlv.Type = 7
	localOutpointType        tlv.Type = 8
	localOutputSignDescType  tlv.Type = 9
)

// backupTask is an internal struct for computing the justice transaction for a
// particular revoked state. A backupTask functions as a scratch pad for storing
// computing values of the transaction itself, such as the final split in
// balance if the justice transaction will give a reward to the tower. The
// backup task has three primary phases:
//  1. Init: Determines which inputs from the breach transaction will be spent,
//     and the total amount contained in the inputs.
//  2. Bind: Asserts that the revoked state is eligible under a given session's
//     parameters. Certain states may be ineligible due to fee rates, too little
//     input amount, etc. Backup of these states can be deferred to a later time
//     or session with more favorable parameters. If the session is bound
//     successfully, the final session-dependent values to the justice
//     transaction are solidified.
//  3. Send: Once the task is bound, it will be queued to send to a specific
//     tower corresponding to the session in which it was bound. The justice
//     transaction will be assembled by examining the parameters left as a
//     result of the binding. After the justice transaction is signed, the
//     necessary components are stripped out and encrypted before being sent to
//     the tower in a StateUpdate.
type backupTask struct {
	id         wtdb.BackupID
	chanType   channeldb.ChannelType
	breachInfo *breachInfoSummary

	// state-dependent variables

	toLocalInput  input.Input
	toRemoteInput input.Input
	totalAmt      btcutil.Amount
	sweepPkScript []byte // can get this from channel summary.

	// session-dependent variables

	blobType blob.Type
	outputs  []*wire.TxOut
}

func serialiseBackupTask(w io.Writer, task *backupTask) error {
	id := task.id
	chanType := uint64(task.chanType)

	breachInfoSize, err := recordSize(breachInfoEncoder, task.breachInfo)
	if err != nil {
		return err
	}

	records := []tlv.Record{
		tlv.MakeDynamicRecord(
			idType, &id, id.EncodedSize, backupIDEncoder,
			backupIDDecoder,
		),
		tlv.MakeDynamicRecord(
			breachInfoType, task.breachInfo, breachInfoSize,
			breachInfoEncoder, breachInfoDecoder,
		),
		tlv.MakePrimitiveRecord(chanTypeType, &chanType),
	}

	stream, err := tlv.NewStream(records...)
	if err != nil {
		return err
	}

	return stream.Encode(w)
}

func deserialiseBackupTask(r io.Reader) (*backupTask, error) {
	var (
		id       = wtdb.BackupID{}
		info     = breachInfoSummary{}
		chanType = uint64(0)
	)
	records := []tlv.Record{
		tlv.MakeDynamicRecord(
			idType, &id, id.EncodedSize, backupIDEncoder,
			backupIDDecoder,
		),
		tlv.MakeDynamicRecord(
			breachInfoType, &info, nil, breachInfoEncoder,
			breachInfoDecoder,
		),
		tlv.MakePrimitiveRecord(chanTypeType, &chanType),
	}

	stream, err := tlv.NewStream(records...)
	if err != nil {
		return nil, err
	}

	err = stream.Decode(r)
	if err != nil {
		return nil, err
	}

	return &backupTask{
		id:         id,
		breachInfo: &info,
		chanType:   channeldb.ChannelType(chanType),
	}, nil
}

func backupIDEncoder(w io.Writer, val interface{}, _ *[8]byte) error {
	if v, ok := val.(*wtdb.BackupID); ok {
		return v.Encode(w)
	}

	return tlv.NewTypeForEncodingErr(val, "*wtdb.BackupID")
}

func backupIDDecoder(r io.Reader, val interface{}, _ *[8]byte, l uint64) error {
	if v, ok := val.(*wtdb.BackupID); ok {
		return v.Decode(r)
	}

	return tlv.NewTypeForDecodingErr(val, "*wtdb.BackupID", l, l)
}

type breachInfoSummary struct {
	toLocalKey    *btcec.PublicKey
	toRemoteKey   *btcec.PublicKey
	revocationKey *btcec.PublicKey

	csvDelay     uint32
	breachTxHash chainhash.Hash

	remoteOutpoint       wire.OutPoint
	remoteOutputSignDesc *input.SignDescriptor

	localOutpoint       wire.OutPoint
	localOutputSignDesc *input.SignDescriptor
}

func breachInfoEncoder(w io.Writer, val interface{}, _ *[8]byte) error {
	if v, ok := val.(*breachInfoSummary); ok {

		var txHash [32]byte
		copy(txHash[:], v.breachTxHash[:])

		remoteOutpointBuff := bytes.NewBuffer(nil)
		err := channeldb.WriteOutpoint(
			remoteOutpointBuff, &v.remoteOutpoint,
		)
		if err != nil {
			return err
		}
		remoteOutpointByte := remoteOutpointBuff.Bytes()

		records := []tlv.Record{
			tlv.MakePrimitiveRecord(toLocalKeyType, &v.toLocalKey),
			tlv.MakePrimitiveRecord(
				toRemoteKeyType, &v.toRemoteKey,
			),
			tlv.MakePrimitiveRecord(
				revocationKeyType, &v.revocationKey,
			),
			tlv.MakePrimitiveRecord(csvDelayType, &v.csvDelay),
			tlv.MakePrimitiveRecord(breachTxHashType, &txHash),
			tlv.MakePrimitiveRecord(
				remoteOutpointType, &remoteOutpointByte,
			),
		}

		if v.remoteOutputSignDesc != nil {
			descSize, err := recordSize(
				signDescEncoder, v.remoteOutputSignDesc,
			)
			if err != nil {
				return err
			}

			records = append(records, tlv.MakeDynamicRecord(
				remoteOutputSignDescType,
				v.remoteOutputSignDesc, descSize,
				signDescEncoder, signDescDecoder,
			))
		}

		localOutpointBuff := bytes.NewBuffer(nil)
		err = channeldb.WriteOutpoint(
			localOutpointBuff, &v.localOutpoint,
		)
		if err != nil {
			return err
		}
		localOutpointBytes := localOutpointBuff.Bytes()

		records = append(records, tlv.MakePrimitiveRecord(
			localOutpointType, &localOutpointBytes,
		))

		if v.localOutputSignDesc != nil {
			descSize, err := recordSize(
				signDescEncoder, v.localOutputSignDesc,
			)
			if err != nil {
				return err
			}

			records = append(records, tlv.MakeDynamicRecord(
				localOutputSignDescType,
				v.localOutputSignDesc, descSize,
				signDescEncoder, signDescDecoder,
			))
		}

		tlvStream, err := tlv.NewStream(records...)
		if err != nil {
			return err
		}

		var summaryBytes bytes.Buffer
		err = tlvStream.Encode(&summaryBytes)
		if err != nil {
			return err
		}

		_, err = w.Write(summaryBytes.Bytes())

		return err
	}

	return tlv.NewTypeForEncodingErr(val, "*breachInfoSummary")
}

func breachInfoDecoder(r io.Reader, val interface{}, _ *[8]byte,
	l uint64) error {

	if v, ok := val.(*breachInfoSummary); ok {
		// Create a new limited reader that'll return an EOF once the
		// end has been reached so the stream stops consuming bytes.
		innerTlvReader := io.LimitedReader{
			R: r,
			N: int64(l),
		}

		var (
			txHashBytes         [32]byte
			remoteOutpointBytes []byte
			localOutpointBytes  []byte
			localSignDesc       input.SignDescriptor
			remoteSignDesc      input.SignDescriptor
		)

		records := []tlv.Record{
			tlv.MakePrimitiveRecord(
				toLocalKeyType, &v.toLocalKey,
			),
			tlv.MakePrimitiveRecord(
				toRemoteKeyType, &v.toRemoteKey,
			),
			tlv.MakePrimitiveRecord(
				revocationKeyType, &v.revocationKey,
			),
			tlv.MakePrimitiveRecord(csvDelayType, &v.csvDelay),
			tlv.MakePrimitiveRecord(breachTxHashType, &txHashBytes),
			tlv.MakePrimitiveRecord(
				remoteOutpointType, &remoteOutpointBytes,
			),
			tlv.MakeDynamicRecord(
				remoteOutputSignDescType, &remoteSignDesc, nil,
				signDescEncoder, signDescDecoder,
			),
			tlv.MakePrimitiveRecord(
				localOutpointType, &localOutpointBytes,
			),
			tlv.MakeDynamicRecord(
				localOutputSignDescType, &localSignDesc, nil,
				signDescEncoder, signDescDecoder,
			),
		}

		tlvStream, err := tlv.NewStream(records...)
		if err != nil {
			return err
		}

		parsedTypes, err := tlvStream.DecodeWithParsedTypes(
			&innerTlvReader,
		)
		if err != nil {
			return err
		}

		copy(v.breachTxHash[:], txHashBytes[:])

		t, ok := parsedTypes[localOutputSignDescType]
		if ok && t == nil {
			v.localOutputSignDesc = &localSignDesc
		}

		t, ok = parsedTypes[remoteOutputSignDescType]
		if ok && t == nil {
			v.remoteOutputSignDesc = &remoteSignDesc
		}

		r := bytes.NewBuffer(localOutpointBytes)
		err = channeldb.ReadOutpoint(r, &v.localOutpoint)
		if err != nil {
			return err
		}

		r = bytes.NewBuffer(remoteOutpointBytes)
		err = channeldb.ReadOutpoint(r, &v.remoteOutpoint)
		if err != nil {
			return err
		}

		return nil
	}

	return tlv.NewTypeForDecodingErr(val, "breachInfoSummary", l, l)
}

func signDescEncoder(w io.Writer, val interface{}, _ *[8]byte) error {
	if v, ok := val.(*input.SignDescriptor); ok {
		return input.WriteSignDescriptor(w, v)
	}

	return tlv.NewTypeForEncodingErr(val, "*input.SignDescriptor")
}

func signDescDecoder(r io.Reader, val interface{}, _ *[8]byte, l uint64) error {
	if v, ok := val.(*input.SignDescriptor); ok {
		return input.ReadSignDescriptor(r, v)
	}

	return tlv.NewTypeForDecodingErr(val, "*input.SignDescriptor", l, l)
}

func getBreachInfoSummary(bi *lnwallet.BreachRetribution) *breachInfoSummary {
	return &breachInfoSummary{
		toLocalKey:           bi.KeyRing.ToLocalKey,
		toRemoteKey:          bi.KeyRing.ToRemoteKey,
		revocationKey:        bi.KeyRing.RevocationKey,
		csvDelay:             bi.RemoteDelay,
		breachTxHash:         bi.BreachTxHash,
		remoteOutpoint:       bi.RemoteOutpoint,
		remoteOutputSignDesc: bi.RemoteOutputSignDesc,
		localOutpoint:        bi.LocalOutpoint,
		localOutputSignDesc:  bi.LocalOutputSignDesc,
	}
}

func (t *backupTask) fillIn(sweepPkScript []byte) {
	bi := t.breachInfo

	// Parse the non-dust outputs from the breach transaction,
	// simultaneously computing the total amount contained in the inputs
	// present. We can't compute the exact output values at this time
	// since the task has not been assigned to a session, at which point
	// parameters such as fee rate, number of outputs, and reward rate will
	// be finalized.
	var (
		totalAmt      int64
		toLocalInput  input.Input
		toRemoteInput input.Input
	)

	// Add the sign descriptors and outputs corresponding to the to-local
	// and to-remote outputs, respectively, if either input amount is
	// non-dust. Note that the naming here seems reversed, but both are
	// correct. For example, the to-remote output on the remote party's
	// commitment is an output that pays to us. Hence the retribution refers
	// to that output as local, though relative to their commitment, it is
	// paying to-the-remote party (which is us).
	if bi.remoteOutputSignDesc != nil {
		toLocalInput = input.NewBaseInput(
			&bi.remoteOutpoint, input.CommitmentRevoke,
			bi.remoteOutputSignDesc, 0,
		)
		totalAmt += bi.remoteOutputSignDesc.Output.Value
	}
	if bi.localOutputSignDesc != nil {
		var witnessType input.WitnessType
		switch {
		case t.chanType.HasAnchors():
			witnessType = input.CommitmentToRemoteConfirmed
		case t.chanType.IsTweakless():
			witnessType = input.CommitSpendNoDelayTweakless
		default:
			witnessType = input.CommitmentNoDelay
		}

		// Anchor channels have a CSV-encumbered to-remote output. We'll
		// construct a CSV input in that case and assign the proper CSV
		// delay of 1, otherwise we fallback to the a regular P2WKH
		// to-remote output for tweaked or tweakless channels.
		if t.chanType.HasAnchors() {
			toRemoteInput = input.NewCsvInput(
				&bi.localOutpoint, witnessType,
				bi.localOutputSignDesc, 0, 1,
			)
		} else {
			toRemoteInput = input.NewBaseInput(
				&bi.localOutpoint, witnessType,
				bi.localOutputSignDesc, 0,
			)
		}

		totalAmt += bi.localOutputSignDesc.Output.Value
	}

	t.toLocalInput = toLocalInput
	t.toRemoteInput = toRemoteInput
	t.totalAmt = btcutil.Amount(totalAmt)
	t.sweepPkScript = sweepPkScript
}

// newBackupTask initializes a new backupTask and populates all state-dependent
// variables.
func newBackupTask(chanID *lnwire.ChannelID,
	breachInfo *lnwallet.BreachRetribution,
	sweepPkScript []byte, chanType channeldb.ChannelType) *backupTask {

	task := &backupTask{
		id: wtdb.BackupID{
			ChanID:       *chanID,
			CommitHeight: breachInfo.RevokedStateNum,
		},
		chanType:   chanType,
		breachInfo: getBreachInfoSummary(breachInfo),
	}

	task.fillIn(sweepPkScript)

	return task
}

// inputs returns all non-dust inputs that we will attempt to spend from.
//
// NOTE: Ordering of the inputs is not critical as we sort the transaction with
// BIP69.
func (t *backupTask) inputs() map[wire.OutPoint]input.Input {
	inputs := make(map[wire.OutPoint]input.Input)
	if t.toLocalInput != nil {
		inputs[*t.toLocalInput.OutPoint()] = t.toLocalInput
	}
	if t.toRemoteInput != nil {
		inputs[*t.toRemoteInput.OutPoint()] = t.toRemoteInput
	}
	return inputs
}

// addrType returns the type of an address after parsing it and matching it to
// the set of known script templates.
func addrType(pkScript []byte) txscript.ScriptClass {
	// We pass in a set of dummy chain params here as they're only needed
	// to make the address struct, which we're ignoring anyway (scripts are
	// always the same, it's addresses that change across chains).
	scriptClass, _, _, _ := txscript.ExtractPkScriptAddrs(
		pkScript, &chaincfg.MainNetParams,
	)

	return scriptClass
}

// addScriptWeight parses the passed pkScript and adds the computed weight cost
// were the script to be added to the justice transaction.
func addScriptWeight(weightEstimate *input.TxWeightEstimator,
	pkScript []byte) error {

	switch addrType(pkScript) { //nolint: whitespace

	case txscript.WitnessV0PubKeyHashTy:
		weightEstimate.AddP2WKHOutput()

	case txscript.WitnessV0ScriptHashTy:
		weightEstimate.AddP2WSHOutput()

	case txscript.WitnessV1TaprootTy:
		weightEstimate.AddP2TROutput()

	default:
		return fmt.Errorf("invalid addr type: %v", addrType(pkScript))
	}

	return nil
}

// bindSession determines if the backupTask is compatible with the passed
// SessionInfo's policy. If no error is returned, the task has been bound to the
// session and can be queued to upload to the tower. Otherwise, the bind failed
// and should be rescheduled with a different session.
func (t *backupTask) bindSession(session *wtdb.ClientSessionBody) error {
	// First we'll begin by deriving a weight estimate for the justice
	// transaction. The final weight can be different depending on whether
	// the watchtower is taking a reward.
	var weightEstimate input.TxWeightEstimator

	// Next, add the contribution from the inputs that are present on this
	// breach transaction.
	if t.toLocalInput != nil {
		// An older ToLocalPenaltyWitnessSize constant used to
		// underestimate the size by one byte. The diferrence in weight
		// can cause different output values on the sweep transaction,
		// so we mimic the original bug and create signatures using the
		// original weight estimate. For anchor channels we'll go ahead
		// an use the correct penalty witness when signing our justice
		// transactions.
		if t.chanType.HasAnchors() {
			weightEstimate.AddWitnessInput(
				input.ToLocalPenaltyWitnessSize,
			)
		} else {
			weightEstimate.AddWitnessInput(
				input.ToLocalPenaltyWitnessSize - 1,
			)
		}
	}
	if t.toRemoteInput != nil {
		// Legacy channels (both tweaked and non-tweaked) spend from
		// P2WKH output. Anchor channels spend a to-remote confirmed
		// P2WSH  output.
		if t.chanType.HasAnchors() {
			weightEstimate.AddWitnessInput(input.ToRemoteConfirmedWitnessSize)
		} else {
			weightEstimate.AddWitnessInput(input.P2WKHWitnessSize)
		}
	}

	// All justice transactions will either use segwit v0 (p2wkh + p2wsh)
	// or segwit v1 (p2tr).
	if err := addScriptWeight(&weightEstimate, t.sweepPkScript); err != nil {
		return err
	}

	// If the justice transaction has a reward output, add the output's
	// contribution to the weight estimate.
	if session.Policy.BlobType.Has(blob.FlagReward) {
		err := addScriptWeight(&weightEstimate, session.RewardPkScript)
		if err != nil {
			return err
		}
	}

	if t.chanType.HasAnchors() != session.Policy.IsAnchorChannel() {
		log.Criticalf("Invalid task (has_anchors=%t) for session "+
			"(has_anchors=%t)", t.chanType.HasAnchors(),
			session.Policy.IsAnchorChannel())
	}

	// Now, compute the output values depending on whether FlagReward is set
	// in the current session's policy.
	outputs, err := session.Policy.ComputeJusticeTxOuts(
		t.totalAmt, int64(weightEstimate.Weight()),
		t.sweepPkScript, session.RewardPkScript,
	)
	if err != nil {
		return err
	}

	t.blobType = session.Policy.BlobType
	t.outputs = outputs

	return nil
}

// craftSessionPayload is the final stage for a backupTask, and generates the
// encrypted payload and breach hint that should be sent to the tower. This
// method computes the final justice transaction using the bound
// session-dependent variables, and signs the resulting transaction. The
// required pieces from signatures, witness scripts, etc are then packaged into
// a JusticeKit and encrypted using the breach transaction's key.
func (t *backupTask) craftSessionPayload(
	signer input.Signer) (blob.BreachHint, []byte, error) {

	var hint blob.BreachHint

	// First, copy over the sweep pkscript, the pubkeys used to derive the
	// to-local script, and the remote CSV delay.
	justiceKit := &blob.JusticeKit{
		BlobType:         t.blobType,
		SweepAddress:     t.sweepPkScript,
		RevocationPubKey: toBlobPubKey(t.breachInfo.revocationKey),
		LocalDelayPubKey: toBlobPubKey(t.breachInfo.toLocalKey),
		CSVDelay:         t.breachInfo.csvDelay,
	}

	// If this commitment has an output that pays to us, copy the to-remote
	// pubkey into the justice kit. This serves as the indicator to the
	// tower that we expect the breaching transaction to have a non-dust
	// output to spend from.
	if t.toRemoteInput != nil {
		justiceKit.CommitToRemotePubKey = toBlobPubKey(
			t.breachInfo.toRemoteKey,
		)
	}

	// Now, begin construction of the justice transaction. We'll start with
	// a version 2 transaction.
	justiceTxn := wire.NewMsgTx(2)

	// Next, add the non-dust inputs that were derived from the breach
	// information. This will either be contain both the to-local and
	// to-remote outputs, or only be the to-local output.
	inputs := t.inputs()
	prevOutputFetcher := txscript.NewMultiPrevOutFetcher(nil)
	for prevOutPoint, inp := range inputs {
		prevOutputFetcher.AddPrevOut(
			prevOutPoint, inp.SignDesc().Output,
		)
		justiceTxn.AddTxIn(&wire.TxIn{
			PreviousOutPoint: prevOutPoint,
			Sequence:         inp.BlocksToMaturity(),
		})
	}

	// Add the sweep output paying directly to the user and possibly a
	// reward output, using the outputs computed when the task was bound.
	justiceTxn.TxOut = t.outputs

	// Sort the justice transaction according to BIP69.
	txsort.InPlaceSort(justiceTxn)

	// Check that the justice transaction meets basic validity requirements
	// before attempting to attach the witnesses.
	btx := btcutil.NewTx(justiceTxn)
	if err := blockchain.CheckTransactionSanity(btx); err != nil {
		return hint, nil, err
	}

	// Construct a sighash cache to improve signing performance.
	hashCache := txscript.NewTxSigHashes(justiceTxn, prevOutputFetcher)

	// Since the transaction inputs could have been reordered as a result of
	// the BIP69 sort, create an index mapping each prevout to it's new
	// index.
	inputIndex := make(map[wire.OutPoint]int)
	for i, txIn := range justiceTxn.TxIn {
		inputIndex[txIn.PreviousOutPoint] = i
	}

	// Now, iterate through the list of inputs that were initially added to
	// the transaction and store the computed witness within the justice
	// kit.
	for _, inp := range inputs {
		// Lookup the input's new post-sort position.
		i := inputIndex[*inp.OutPoint()]

		// Construct the full witness required to spend this input.
		inputScript, err := inp.CraftInputScript(
			signer, justiceTxn, hashCache, prevOutputFetcher, i,
		)
		if err != nil {
			return hint, nil, err
		}

		// Parse the DER-encoded signature from the first position of
		// the resulting witness. We trim an extra byte to remove the
		// sighash flag.
		witness := inputScript.Witness
		rawSignature := witness[0][:len(witness[0])-1]

		// Re-encode the DER signature into a fixed-size 64 byte
		// signature.
		signature, err := lnwire.NewSigFromRawSignature(rawSignature)
		if err != nil {
			return hint, nil, err
		}

		// Finally, copy the serialized signature into the justice kit,
		// using the input's witness type to select the appropriate
		// field.
		switch inp.WitnessType() {
		case input.CommitmentRevoke:
			copy(justiceKit.CommitToLocalSig[:], signature[:])

		case input.CommitSpendNoDelayTweakless:
			fallthrough
		case input.CommitmentNoDelay:
			fallthrough
		case input.CommitmentToRemoteConfirmed:
			copy(justiceKit.CommitToRemoteSig[:], signature[:])
		default:
			return hint, nil, fmt.Errorf("invalid witness type: %v",
				inp.WitnessType())
		}
	}

	breachTxID := t.breachInfo.breachTxHash

	// Compute the breach key as SHA256(txid).
	hint, key := blob.NewBreachHintAndKeyFromHash(&breachTxID)

	// Then, we'll encrypt the computed justice kit using the full breach
	// transaction id, which will allow the tower to recover the contents
	// after the transaction is seen in the chain or mempool.
	encBlob, err := justiceKit.Encrypt(key)
	if err != nil {
		return hint, nil, err
	}

	return hint, encBlob, nil
}

// toBlobPubKey serializes the given pubkey into a blob.PubKey that can be set
// as a field on a blob.JusticeKit.
func toBlobPubKey(pubKey *btcec.PublicKey) blob.PubKey {
	var blobPubKey blob.PubKey
	copy(blobPubKey[:], pubKey.SerializeCompressed())
	return blobPubKey
}

// recordSize returns the amount of bytes this TLV record will occupy when
// encoded.
func recordSize(encoder tlv.Encoder, v interface{}) (tlv.SizeFunc, error) {
	var (
		b   bytes.Buffer
		buf [8]byte
	)
	if err := encoder(&b, v, &buf); err != nil {
		return nil, err
	}

	return func() uint64 {
		return uint64(len(b.Bytes()))
	}, nil
}
