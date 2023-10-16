package lnwire

import (
	"fmt"
	"time"
)

type Timestamp interface {
	Before(timestamp Timestamp) (bool, error)
	Int() uint64
}

type UnixTimestamp int64

func (u *UnixTimestamp) Int() uint64 {
	return uint64(*u)
}

func (u *UnixTimestamp) Before(other Timestamp) (bool, error) {
	o, ok := other.(*UnixTimestamp)
	if !ok {
		return false, fmt.Errorf("wrong Timestamp type")
	}

	t1 := time.Unix(int64(*u), 0)
	t2 := time.Unix(int64(*o), 0)

	return t1.Before(t2), nil
}

type BlockHeight uint32

func (b *BlockHeight) Int() uint64 {
	return uint64(*b)
}

func (b *BlockHeight) Before(other Timestamp) (bool, error) {
	o, ok := other.(*BlockHeight)
	if !ok {
		return false, fmt.Errorf("wrong Timestamp type")
	}

	return *b < *o, nil
}
