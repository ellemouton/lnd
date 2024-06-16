package graphdb

import (
	"encoding/binary"
	"io"

	"github.com/btcsuite/btcd/wire"
)

// writeOutpoint writes an outpoint to the passed writer using the minimal
// amount of bytes possible.
func writeOutpoint(w io.Writer, o *wire.OutPoint) error {
	if _, err := w.Write(o.Hash[:]); err != nil {
		return err
	}

	return binary.Write(w, byteOrder, o.Index)
}

// readOutpoint reads an outpoint from the passed reader that was previously
// written using the writeOutpoint struct.
func readOutpoint(r io.Reader, o *wire.OutPoint) error {
	if _, err := io.ReadFull(r, o.Hash[:]); err != nil {
		return err
	}

	return binary.Read(r, byteOrder, &o.Index)
}
