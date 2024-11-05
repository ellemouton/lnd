package main

import (
	"context"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/jessevdk/go-flags"
	"github.com/lightningnetwork/lnd"
	"github.com/lightningnetwork/lnd/lnrpc/graphrpc"
	"github.com/lightningnetwork/lnd/macaroons"
	"github.com/lightningnetwork/lnd/signal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/macaroon.v2"
)

func main() {
	// Hook interceptor for os signals.
	shutdownInterceptor, err := signal.Intercept()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	_ = setupGraphSourceNode(shutdownInterceptor)

	graphConn, err := connectToGraphRPC()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	setupDependentNode(shutdownInterceptor, graphConn)
	<-shutdownInterceptor.ShutdownChannel()
}

func connectToGraphRPC() (graphrpc.GraphClient, error) {
	macPath := "/Users/elle/.lnd-dev-graph/data/chain/bitcoin/regtest/admin.macaroon"
	tlsCertPath := "/Users/elle/.lnd-dev-graph/tls.cert"

	tlsCert, err := os.ReadFile(tlsCertPath)
	if err != nil {
		return nil, fmt.Errorf("could not load TLS cert file: %v", err)
	}

	cp := x509.NewCertPool()
	if !cp.AppendCertsFromPEM(tlsCert) {
		return nil, fmt.Errorf("credentials: failed to append certificate")
	}
	creds := credentials.NewClientTLSFromCert(cp, "")

	macBytes, err := os.ReadFile(macPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read macaroon path (check "+
			"the network setting!): %v", err)
	}
	mac := &macaroon.Macaroon{}
	if err = mac.UnmarshalBinary(macBytes); err != nil {
		return nil, fmt.Errorf("unable to decode macaroon: %w", err)
	}
	// Now we append the macaroon credentials to the dial options.
	cred, err := macaroons.NewMacaroonCredential(mac)
	if err != nil {
		return nil, err
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
		grpc.WithPerRPCCredentials(cred),
	}

	conn, err := grpc.Dial("localhost:10020", opts...)
	if err != nil {
		return nil, err
	}

	return graphrpc.NewGraphClient(conn), nil
}

func setupGraphSourceNode(interceptor signal.Interceptor) lnd.Providers {
	preCfg := graphConfig()

	cfg, err := lnd.LoadConfigNoFlags(*preCfg, interceptor)
	if err != nil {
		os.Exit(1)
	}

	implCfg := cfg.ImplementationConfig(interceptor)

	lndProviders := make(chan lnd.Providers, 1)

	go func() {
		if err := lnd.Main(
			cfg, lnd.ListenerCfg{}, implCfg, interceptor,
			lndProviders,
		); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
		}
		fmt.Println("Graph node has stopped")
		os.Exit(1)
	}()

	return <-lndProviders
}

func setupDependentNode(interceptor signal.Interceptor,
	graphRPCConn graphrpc.GraphClient) {

	// Load the configuration, and parse any command line options. This
	// function will also set up logging properly.
	loadedConfig, err := lnd.LoadConfig(interceptor)
	if err != nil {
		if e, ok := err.(*flags.Error); !ok || e.Type != flags.ErrHelp {
			// Print error if not due to help request.
			err = fmt.Errorf("failed to load config: %w", err)
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		// Help was requested, exit normally.
		os.Exit(0)
	}
	loadedConfig.Gossip.NoSync = true
	implCfg := loadedConfig.ImplementationConfig(interceptor)
	implCfg.GraphProvider = &graphProvider{remoteGraphConn: graphRPCConn}

	// Call the "real" main in a nested manner so the defers will properly
	// be executed in the case of a graceful shutdown.

	if err = lnd.Main(
		loadedConfig, lnd.ListenerCfg{}, implCfg, interceptor, nil,
	); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type graphProvider struct {
	remoteGraphConn graphrpc.GraphClient
}

func (g *graphProvider) Graph(_ context.Context, dbs *lnd.DatabaseInstances) (
	lnd.GraphSource, error) {

	return NewGraphBackend(dbs.GraphDB, &remoteWrapper{
		conn:  g.remoteGraphConn,
		local: dbs.GraphDB,
	}), nil
}

func graphConfig() *lnd.Config {
	cfg := lnd.DefaultConfig()
	//if _, err := flags.Parse(&cfg); err != nil {
	//	os.Exit(1)
	//}

	cfg.Bitcoin.RegTest = true
	cfg.LndDir = "/Users/elle/.lnd-dev-graph"
	cfg.BitcoindMode.RPCHost = "localhost:18443"
	cfg.Bitcoin.Node = "bitcoind"
	cfg.RawRPCListeners = []string{"localhost:10020"}
	cfg.BitcoindMode.RPCUser = "lightning"
	cfg.BitcoindMode.RPCPass = "lightning"
	cfg.BitcoindMode.ZMQPubRawBlock = "tcp://localhost:28332"
	cfg.BitcoindMode.ZMQPubRawTx = "tcp://localhost:28333"
	cfg.TrickleDelay = 50
	cfg.NoSeedBackup = true
	cfg.RawRESTListeners = []string{"localhost:11020"}
	cfg.RawListeners = []string{"localhost:9736"}
	cfg.DebugLevel = "debug"
	cfg.LogConfig.Console.Disable = true

	return &cfg
}
