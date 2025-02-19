#!/bin/bash

function bitcoin() {
  docker exec -ti -u bitcoin bitcoind bitcoin-cli -regtest -rpcuser=lightning -rpcpassword=lightning "$@"
}

function alice() {
  docker exec -ti alice lncli --network regtest "$@"
}

function bob() {
  docker exec -ti bob lncli --network regtest "$@"
}

function bob2() {
  docker exec -ti bob2 lncli --network regtest "$@"
}
