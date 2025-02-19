#!/bin/bash

COMPOSE="docker-compose -f network.yaml -p regtest"
BOB=bob

source "./helpers.sh"

# Spin up the network. The `-d` means it is spun up in detached mode
# and so ensures that the script doesn't get stuck.
$COMPOSE up -d

# Wait for nodes to start.
echo "waiting for nodes to start"
for container in alice bob; do
  echo "checking $container"
  while ! $container getinfo | grep -q identity_pubkey; do
    sleep 1
  done
done
echo "all nodes are started"

# Print out the commit hash of the running nodes.
echo "Alice is running on $(alice version | jq -r '.lnd.commit_hash' | sed 's/^[ \t]*//')"
echo "Bob is running on $( $BOB version | jq -r '.lnd.commit_hash' | sed 's/^[ \t]*//')"

# Stop this version of Bob.
$COMPOSE stop bob

# Merge the two compose files such that Bob is now running local version.
docker-compose -f network.yaml -f network2.yaml -p regtest up -d bob2

# Update the BOB variable to point to the new Bob.
BOB=bob2

# Wait for nodes to start.
echo "waiting for nodes to start"
for container in alice $BOB; do
  echo "checking $container"
  while ! $container getinfo | grep -q identity_pubkey; do
    sleep 1
  done
done
echo "all nodes are started"

# Print out the commit hash of the running nodes to show that
# Bob is now running the local version.
echo "Alice is running on $(alice version | jq -r '.lnd.commit_hash' | sed 's/^[ \t]*//')"
echo "Bob is running on $( $BOB version | jq -r '.lnd.commit_hash' | sed 's/^[ \t]*//')"

$COMPOSE down --volumes --remove-orphans
