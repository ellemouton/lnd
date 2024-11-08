package models

import "github.com/btcsuite/btcd/btcutil"

type NetworkStats struct {
	Diameter             uint32
	MaxChanOut           uint32
	NumNodes             uint32
	NumChannels          uint32
	TotalNetworkCapacity btcutil.Amount
	MinChanSize          btcutil.Amount
	MaxChanSize          btcutil.Amount
	MedianChanSize       btcutil.Amount
	NumZombies           uint64
}
