package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jessevdk/go-flags"
	"github.com/lightningnetwork/lnd"
	graphsources "github.com/lightningnetwork/lnd/graph/sources"
	"github.com/lightningnetwork/lnd/routing/route"
	"github.com/lightningnetwork/lnd/signal"
)

func main() {
	// Hook interceptor for os signals.
	shutdownInterceptor, err := signal.Intercept()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	lndProviders := setupGraphSourceNode(shutdownInterceptor)
	setupDependentNode(shutdownInterceptor, lndProviders)
	<-shutdownInterceptor.ShutdownChannel()
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
	lndProviders lnd.Providers) {
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
	implCfg.GraphProvider = &graphProvider{lndProviders: lndProviders}
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
	lndProviders lnd.Providers
}

func (g *graphProvider) Graph(_ context.Context,
	dbs *lnd.DatabaseInstances) (graphsources.GraphSource, error) {

	graphSource, err := g.lndProviders.GraphSource()
	if err != nil {
		return nil, err
	}

	getLocalPub := func() (route.Vertex, error) {
		node, err := dbs.GraphDB.SourceNode()
		if err != nil {
			return route.Vertex{}, err
		}

		return route.NewVertexFromBytes(node.PubKeyBytes[:])
	}

	return graphsources.NewMux(
		graphsources.NewDBGSource(dbs.GraphDB), graphSource,
		getLocalPub,
	), nil
}

func graphConfig() *lnd.Config {
	cfg := lnd.DefaultConfig()
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
