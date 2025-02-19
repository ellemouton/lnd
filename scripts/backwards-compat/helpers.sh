#!/bin/bash

BOB=bob

function bitcoin() {
  docker exec -i -u bitcoin bitcoind bitcoin-cli -regtest -rpcuser=lightning -rpcpassword=lightning "$@"
}

function alice() {
  docker exec -i alice lncli --network regtest "$@"
}

function bob() {
  docker exec -i bob lncli --network regtest "$@"
}

function bob2() {
  docker exec -i bob2 lncli --network regtest "$@"
}

function charlie() {
  docker exec -i charlie lncli --network regtest "$@"
}

function dave() {
  docker exec -i dave lncli --network regtest "$@"
}

function setup() {
  echo "creating wallet"
  bitcoin createwallet miner

  ADDR_BTC=$(bitcoin getnewaddress "" legacy)
  echo "generating blocks to $ADDR_BTC"
  bitcoin generatetoaddress 106 "$ADDR_BTC" > /dev/null
  bitcoin getbalance

  fund_nodes

  connect_nodes

  mine

  ALICE=$(alice getinfo | jq .identity_pubkey -r)
  BOB_KEY=$( $BOB getinfo | jq .identity_pubkey -r)
  CHARLIE_KEY=$(charlie getinfo | jq .identity_pubkey -r)
  DAVE_KEY=$(dave getinfo | jq .identity_pubkey -r)

  echo "Opening Channels"

  # alice -> bob
  alice openchannel --node_key "$BOB_KEY" --local_amt 15000000 --push_amt 7000000

  # bob -> charlie
  $BOB openchannel --node_key "$CHARLIE_KEY" --local_amt 15000000 --push_amt 7000000

  # charlie -> dave
  charlie openchannel --node_key "$DAVE_KEY" --local_amt 15000000 --push_amt 7000000

  mine 7
}

function fund_nodes() {
  echo "Getting addresses.."
  ADDR_ALICE=$(alice newaddress p2wkh | jq .address -r)
  ADDR_BOB=$( $BOB newaddress p2wkh | jq .address -r)
  ADDR_CHARLIE=$(charlie newaddress p2wkh | jq .address -r)
  ADDR_DAVE=$(dave newaddress p2wkh | jq .address -r)

  echo "Sending funds.."
  bitcoin sendtoaddress "$ADDR_ALICE" 5
  bitcoin sendtoaddress "$ADDR_BOB" 5
  bitcoin sendtoaddress "$ADDR_CHARLIE" 5
  bitcoin sendtoaddress "$ADDR_DAVE" 5
}

function connect_nodes() {
  echo "Connecting nodes.."
  ALICE_KEY=$(alice getinfo | jq .identity_pubkey -r)
  BOB_KEY=$( $BOB getinfo | jq .identity_pubkey -r)
  CHARLIE_KEY=$(charlie getinfo | jq .identity_pubkey -r)
  DAVE_KEY=$(dave getinfo | jq .identity_pubkey -r)

  # connect up all the nodes
  alice connect "$BOB_KEY"@bob:9735
  bob connect "$CHARLIE_KEY"@charlie:9735
  charlie connect "$DAVE_KEY"@dave:9735
}

function mine() {
  # try to use the value of the first argument passed to the function
  # if `mine` was called without any argument use the default value (6)
  NUMBLOCKS="${1-6}"
  bitcoin generatetoaddress "$NUMBLOCKS" "$(bitcoin getnewaddress "" legacy)" > /dev/null
}