package lnd

import (
	"fmt"
	"testing"

	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/stretchr/testify/assert"
)

func TestGetAllPermissions(t *testing.T) {
	perms := GetAllPermissions()

	// Currently there are there are 16 entity:action pairs in use.
	assert.Equal(t, len(perms), 16)
}

func TestTemp(t *testing.T) {
	var (
		totalFeeBase uint32
		base         uint32 = 2000
		feeRate      uint32
	)

	numerator := (base * 1000000) + (totalFeeBase * (1000000 + feeRate))
	denominator := uint32(1000000)

	fmt.Println(numerator, denominator)
	fmt.Println(numerator / denominator)

	ceiling := (numerator + denominator - 1) / denominator
	fmt.Println(float32(numerator+denominator-1) / float32(denominator))

	fmt.Println(ceiling)

	//numerator := (base * 1000000) + (totalFeeBase * (1000000 + feeRate)) + 1000000 - 1
	//demoninator := 1000000
	//
	//fmt.Println(float32(numerator), float32(demoninator))
	//fmt.Println(float32(numerator) / float32(demoninator))
	//
	//answer := ((base * 1000000) + (totalFeeBase * (1000000 + feeRate)) + 1000000 - 1) / 1000000
	//
	//fmt.Println(answer)
}

func TestAnother(t *testing.T) {
	/*
	   fee_base_msat: 100
	   fee_proportional_millionths: 500
	   htlc_minimum_msat: 1000
	   cltv_expiry_delta: 144
	*/

	var (
		routeBaseFee lnwire.MilliSatoshi
		routePropFee lnwire.MilliSatoshi
		routeCLTV    uint32
	)
	for i := 0; i < 2; i++ {
		routeBaseFee = calcTotalBaseFee(100, routeBaseFee, 500)
		routePropFee = calcTotalPropFee(routePropFee, 500)
		routeCLTV += 144
	}

	fmt.Println(routeBaseFee)
	fmt.Println(routePropFee)
	fmt.Println(routeCLTV + 12)
}

func calcTotalBaseFee(feeBaseMsat, totalBaseFeePrevHop,
	feePropMills lnwire.MilliSatoshi) lnwire.MilliSatoshi {

	numerator := (feeBaseMsat * 1000000) +
		(totalBaseFeePrevHop * (1000000 + feePropMills))

	denominator := lnwire.MilliSatoshi(1000000)

	ceiling := (numerator + denominator - 1) / denominator

	return ceiling
}

// route_fee_proportional_millionths(n+1) = ((route_fee_proportional_millionths(n) + fee_proportional_millionths(n+1)) * 1000000 + route_fee_proportional_millionths(n) * fee_proportional_millionths(n+1) + 1000000 - 1) / 1000000
func calcTotalPropFee(totalPropFee, feePropMills lnwire.MilliSatoshi) lnwire.MilliSatoshi {

	// (route_fee_proportional_millionths(n) + fee_proportional_millionths(n+1)) * 1000000 + route_fee_proportional_millionths(n) * fee_proportional_millionths(n+1)
	numerator := ((totalPropFee + feePropMills) * 1000000) + (totalPropFee * feePropMills)

	denominator := lnwire.MilliSatoshi(1000000)

	ceiling := (numerator + denominator - 1) / denominator

	return ceiling
}
