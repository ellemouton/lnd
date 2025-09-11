package netann

import (
	"bytes"
	"errors"
	"fmt"
	"image/color"
	"net"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/davecgh/go-spew/spew"
	"github.com/lightningnetwork/lnd/fn/v2"
	"github.com/lightningnetwork/lnd/keychain"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/tlv"
)

const (
	nodeAnn2MsgName = "node_announcement_2"

	nodeAnn2SigField = "signature"
)

func applyNodeOptions(ann lnwire.NodeAnnouncement,
	options ...NodeAnnModifier) error {

	var opts nodeAnnOptions
	for _, o := range options {
		o(&opts)
	}

	switch a := ann.(type) {
	case *lnwire.NodeAnnouncement1:
		opts.Alias.WhenSome(func(alias lnwire.NodeAlias) {
			a.Alias = alias
		})
		opts.color.WhenSome(func(rgba color.RGBA) {
			a.RGBColor = rgba
		})
		opts.features.WhenSome(func(f lnwire.RawFeatureVector) {
			a.Features = &f
		})
		if len(opts.addrs) > 0 {
			err := a.SetAddrs(opts.addrs)
			if err != nil {
				return err
			}
		}
		opts.timestamp.WhenSome(func(t time.Time) {
			newTimestamp := uint32(t.Unix())
			if newTimestamp <= a.Timestamp {
				// Increment the prior value to  ensure the timestamp
				// monotonically increases, otherwise the announcement won't
				// propagate.
				newTimestamp = a.Timestamp + 1
			}
			a.Timestamp = newTimestamp
		})

	case *lnwire.NodeAnnouncement2:
		opts.Alias.WhenSome(func(alias lnwire.NodeAlias) {
			aliasBytes := []byte(alias.String())
			aliasRecord := tlv.ZeroRecordT[tlv.TlvType3, []byte]()
			aliasRecord.Val = aliasBytes
			a.Alias = tlv.SomeRecordT(aliasRecord)
		})
		opts.color.WhenSome(func(rgba color.RGBA) {
			colorRecord := tlv.ZeroRecordT[tlv.TlvType1, lnwire.Color]()
			colorRecord.Val = lnwire.Color(rgba)
			a.Color = tlv.SomeRecordT(colorRecord)
		})
		if len(opts.addrs) > 0 {
			err := a.SetAddrs(opts.addrs)
			if err != nil {
				return err
			}
		}
		opts.features.WhenSome(func(f lnwire.RawFeatureVector) {
			a.Features.Val = f
		})
		opts.blockHeight.WhenSome(func(u uint32) {
			newBlockHeight := u
			if newBlockHeight <= a.BlockHeight.Val {
				// Increment the prior value to  ensure the timestamp
				// monotonically increases, otherwise the announcement won't
				// propagate.
				newBlockHeight = a.BlockHeight.Val + 1
			}
			a.BlockHeight.Val = newBlockHeight
		})

	default:
		return fmt.Errorf("unknown node announcement version: %v",
			ann.MsgType())
	}

	if opts.annModifier != nil {
		if err := opts.annModifier(ann); err != nil {
			return err
		}
	}

	return nil
}

type nodeAnnOptions struct {
	Alias       fn.Option[lnwire.NodeAlias]
	color       fn.Option[color.RGBA]
	addrs       []net.Addr
	features    fn.Option[lnwire.RawFeatureVector]
	timestamp   fn.Option[time.Time]
	blockHeight fn.Option[uint32]
	annModifier func(lnwire.NodeAnnouncement) error
}

// NodeAnnModifier is a closure that makes in-place modifications to an
// lnwire.NodeAnnouncement1.
type NodeAnnModifier func(*nodeAnnOptions)

func NodeAnnModifierFunc(f func(lnwire.NodeAnnouncement) error) NodeAnnModifier {
	return func(opts *nodeAnnOptions) {
		opts.annModifier = f
	}
}

// NodeAnnSetAlias is a functional option that sets the alias of the
// given node announcement.
func NodeAnnSetAlias(alias lnwire.NodeAlias) NodeAnnModifier {
	return func(opts *nodeAnnOptions) {
		opts.Alias = fn.Some(alias)
	}
}

// NodeAnnSetAddrs is a functional option that allows updating the addresses of
// the given node announcement.
func NodeAnnSetAddrs(addrs []net.Addr) NodeAnnModifier {
	return func(opts *nodeAnnOptions) {
		opts.addrs = addrs
	}
}

// NodeAnnSetColor is a functional option that sets the color of the
// given node announcement.
func NodeAnnSetColor(newColor color.RGBA) NodeAnnModifier {
	return func(opts *nodeAnnOptions) {
		opts.color = fn.Some(newColor)
	}
}

// NodeAnnSetFeatures is a functional option that allows updating the features of
// the given node announcement.
func NodeAnnSetFeatures(features *lnwire.RawFeatureVector) NodeAnnModifier {
	return func(opts *nodeAnnOptions) {
		opts.features = fn.Some(*features)
	}
}

// NodeAnnSetTimestamp is a functional option that sets the timestamp of the
// announcement to the current time, or increments it if the timestamp is
// already in the future.
func NodeAnnSetTimestamp(t time.Time) NodeAnnModifier {
	return func(opts *nodeAnnOptions) {
		opts.timestamp = fn.Some(t)
	}
}

func NodeAnnSetBlockHeight(h uint32) NodeAnnModifier {
	return func(opts *nodeAnnOptions) {
		opts.blockHeight = fn.Some(h)
	}
}

// SignNodeAnnouncement signs the lnwire.NodeAnnouncement1 provided, which
// should be the most recent, valid update, otherwise the timestamp may not
// monotonically increase from the prior.
func SignNodeAnnouncement(signer keychain.MessageSignerRing,
	keyLoc keychain.KeyLocator, nodeAnn lnwire.NodeAnnouncement,
	options ...NodeAnnModifier) error {

	err := applyNodeOptions(nodeAnn, options...)
	if err != nil {
		return err
	}

	switch ann := nodeAnn.(type) {
	case *lnwire.NodeAnnouncement1:
		// Create the DER-encoded ECDSA signature over the message digest.
		sig, err := SignAnnouncement(signer, keyLoc, nodeAnn)
		if err != nil {
			return err
		}

		// Parse the DER-encoded signature into a fixed-size 64-byte array.
		ann.Signature, err = lnwire.NewSigFromSignature(sig)
		if err != nil {
			return err
		}

	case *lnwire.NodeAnnouncement2:
		data, err := lnwire.SerialiseFieldsToSign(ann)
		if err != nil {
			return err
		}

		sig, err := signer.SignMessageSchnorr(
			keyLoc, data, false, nil, NodeAnn2DigestTag(),
		)
		if err != nil {
			return err
		}

		ann.Signature.Val, err = lnwire.NewSigFromSignature(sig)
		if err != nil {
			return err
		}

	default:
		return fmt.Errorf("unknown node announcement version: %v",
			nodeAnn.MsgType())

	}

	return nil
}

// ValidateNodeAnn validates the fields and signature of a node announcement.
func ValidateNodeAnn(a lnwire.NodeAnnouncement) error {
	err := ValidateNodeAnnFields(a)
	if err != nil {
		return fmt.Errorf("invalid node announcement fields: %w", err)
	}

	return ValidateNodeAnnSignature(a)
}

// ValidateNodeAnnFields validates the fields of a node announcement.
func ValidateNodeAnnFields(ann lnwire.NodeAnnouncement) error {

	switch a := ann.(type) {
	case *lnwire.NodeAnnouncement1:
		// Check that it only has at most one DNS address.
		hasDNSAddr := false
		for _, addr := range a.Addresses {
			dnsAddr, ok := addr.(*lnwire.DNSAddress)
			if !ok {
				continue
			}
			if hasDNSAddr {
				return errors.New("node announcement contains " +
					"multiple DNS addresses. Only one is allowed")
			}

			hasDNSAddr = true

			err := lnwire.ValidateDNSAddr(dnsAddr.Hostname, dnsAddr.Port)
			if err != nil {
				return err
			}
		}

	case *lnwire.NodeAnnouncement2:
		// TODO(elle): add DNS TLV to NodeAnnouncement2 and validate it
		// here.
	}

	return nil
}

// ValidateNodeAnnSignature validates the node announcement by ensuring that the
// attached signature is needed a signature of the node announcement under the
// specified node public key.
func ValidateNodeAnnSignature(a lnwire.NodeAnnouncement) error {
	switch ann := a.(type) {
	case *lnwire.NodeAnnouncement1:
		return validateNodeAnn1Sig(ann)

	case *lnwire.NodeAnnouncement2:
		return validateNodeAnn2Sig(ann)

	default:
		return fmt.Errorf("unknown node announcement version: %v",
			a.MsgType())
	}
}

func validateNodeAnn1Sig(a *lnwire.NodeAnnouncement1) error {
	// Reconstruct the data of announcement which should be covered by the
	// signature so we can verify the signature shortly below
	data, err := a.DataToSign()
	if err != nil {
		return err
	}

	nodeSig, err := a.Signature.ToSignature()
	if err != nil {
		return err
	}
	nodeKey, err := btcec.ParsePubKey(a.NodeID[:])
	if err != nil {
		return err
	}

	// Finally ensure that the passed signature is valid, if not we'll
	// return an error so this node announcement can be rejected.
	dataHash := chainhash.DoubleHashB(data)
	if !nodeSig.Verify(dataHash, nodeKey) {
		var msgBuf bytes.Buffer
		if _, err := lnwire.WriteMessage(&msgBuf, a, 0); err != nil {
			return err
		}

		return fmt.Errorf("signature on NodeAnnouncement1(%x) is "+
			"invalid: %x", nodeKey.SerializeCompressed(),
			msgBuf.Bytes())
	}

	return nil
}

func validateNodeAnn2Sig(a *lnwire.NodeAnnouncement2) error {
	digest, err := nodeAnn2DigestToSign(a)
	if err != nil {
		return fmt.Errorf("unable to reconstruct message data: %w", err)
	}

	nodeSig, err := a.Signature.Val.ToSignature()
	if err != nil {
		return err
	}

	pubKey, err := btcec.ParsePubKey(a.NodeID.Val[:])
	if err != nil {
		return err
	}

	if !nodeSig.Verify(digest, pubKey) {
		return fmt.Errorf("invalid signature for node ann %v",
			spew.Sdump(a))
	}

	return nil
}

func NodeAnn2DigestTag() []byte {
	return MsgTag(nodeAnn2MsgName, nodeAnn2SigField)
}

func nodeAnn2DigestToSign(c *lnwire.NodeAnnouncement2) ([]byte, error) {
	data, err := lnwire.SerialiseFieldsToSign(c)
	if err != nil {
		return nil, err
	}

	hash := MsgHash(nodeAnn2MsgName, nodeAnn2SigField, data)

	return hash[:], nil
}
