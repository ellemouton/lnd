#!/bin/bash

# stop the script if an error is returned
set -e

COMPOSE="docker-compose -f network.yaml -p regtest"

source "./helpers.sh"

# Spin up the network. The `-d` means it is spun up in detached mode
# and so ensures that the script doesn't get stuck.
$COMPOSE up -d

# Wait for nodes to start.
echo "waiting for nodes to start"
for container in alice $BOB charlie dave; do
  echo "checking $container"
  while ! $container getinfo | grep -q identity_pubkey; do
    sleep 1
  done
done
echo "all nodes are started"

# Print out the commit hash of the running nodes.
echo "Alice is running on $(alice version | jq -r '.lnd.commit_hash' | sed 's/^[ \t]*//')"
echo "Bob is running on $( $BOB version | jq -r '.lnd.commit_hash' | sed 's/^[ \t]*//')"

## Set-up channels.
setup

echo "Wait for all nodes to see all channels"
for container in alice $BOB charlie dave; do
  echo "Checking $container..."

  while :; do
    num_channels=$($container getnetworkinfo | jq -r '.num_channels')

    # Ensure num_channels is a valid number before proceeding
    if [[ "$num_channels" =~ ^[0-9]+$ ]]; then
      echo "$container sees $num_channels channels..."

      if [[ "$num_channels" -eq 3 ]]; then
        break  # Exit loop when num_channels reaches 3
      fi
    fi

    sleep 1
  done
done
echo "All nodes detect the expected number of channels!"

## Let Bob send to Dave (multi-hop send test via Charlie).
PAY_REQ=$(dave addinvoice 10000 | jq .payment_request -r)
$BOB payinvoice --force "$PAY_REQ" > /dev/null
echo "Bob can send!"

## Let Bob receive a payment from Dave.
PAY_REQ=$( $BOB addinvoice 10000 | jq .payment_request -r)
dave payinvoice --force "$PAY_REQ" > /dev/null
echo "Bob can receive!"

## Now let Bob route a payment by sending from Alice to Dave.
PAY_REQ=$(dave addinvoice 10000 | jq .payment_request -r)
alice payinvoice --force "$PAY_REQ" > /dev/null
echo "Bob can route!"

# Stop this version of Bob.
$COMPOSE stop bob

# Merge the two compose files such that Bob is now running local version.
docker-compose -f network.yaml -f network2.yaml -p regtest up -d bob2

# Update the BOB variable to point to the new Bob.
BOB=bob2

# Wait for Bob to start.
echo "waiting for nodes to start"
for container in $BOB; do
  echo "checking $container"
  while ! $container getinfo | grep -q identity_pubkey; do
    sleep 1
  done
done
echo "Bob has restarted"

# Print out the commit hash of the running nodes to show that
# Bob is now running the local version.
echo "Alice is running on $(alice version | jq -r '.lnd.commit_hash' | sed 's/^[ \t]*//')"
echo "Bob is running on $( $BOB version | jq -r '.lnd.commit_hash' | sed 's/^[ \t]*//')"

## Wait for all Bob's channels to be active.
## Let Bob receive, send and route.
echo "Waiting for Bob to have exactly 2 active channels..."
while :; do
    # Get Bob's list of channels
    ACTIVE_COUNT=$( $BOB --network=regtest listchannels | jq '[.channels[] | select(.active == true)] | length')

    # Ensure ACTIVE_COUNT is a valid number
    if [[ "$ACTIVE_COUNT" =~ ^[0-9]+$ ]]; then
        echo "Bob sees $ACTIVE_COUNT active channels..."

        # Exit loop only if exactly 2 channels are active
        if [[ "$ACTIVE_COUNT" -eq 2 ]]; then
            break
        fi
    fi

    sleep 1
done
echo "✅ Bob now has exactly 2 active channels!"

## Let Bob send to Dave (multi-hop send test via Charlie).
PAY_REQ=$(dave addinvoice 10000 | jq .payment_request -r)
$BOB payinvoice --force "$PAY_REQ" > /dev/null
echo "Bob can still send!"

## Let Bob receive a payment from Dave.
PAY_REQ=$( $BOB addinvoice 10000 | jq .payment_request -r)
dave payinvoice --force "$PAY_REQ" > /dev/null
echo "Bob can still receive!"

## Now let Bob route a payment by sending from Alice to Dave.
PAY_REQ=$(dave addinvoice 10000 | jq .payment_request -r)
alice payinvoice --force "$PAY_REQ" > /dev/null
echo "Bob can still route!"

$COMPOSE down --volumes --remove-orphans
