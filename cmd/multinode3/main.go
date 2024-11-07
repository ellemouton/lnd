package main

import (
	"context"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"github.com/jessevdk/go-flags"
	"github.com/lightningnetwork/lnd"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/graphrpc"
	"github.com/lightningnetwork/lnd/macaroons"
	"github.com/lightningnetwork/lnd/signal"
	"github.com/lightningnetwork/lnd/tor"
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

	conn, err := connectToGraphNode()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	provider := &graphProvider{
		remoteGraphConn: graphrpc.NewGraphClient(conn),
		remoteLNConn:    lnrpc.NewLightningClient(conn),
	}
	graphrpc.NewGraphClient(conn)

	setupDependentNode(shutdownInterceptor, provider)
	<-shutdownInterceptor.ShutdownChannel()
}

func connectToGraphNode() (*grpc.ClientConn, error) {
	macPath := "/Users/elle/.lnd-dev-sam/data/chain/bitcoin/regtest/admin.macaroon"
	tlsCertPath := "/Users/elle/.lnd-dev-sam/tls.cert"

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

	return grpc.Dial("localhost:10020", opts...)
}

func setupDependentNode(interceptor signal.Interceptor,
	graphProvider *graphProvider) {

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
	loadedConfig.Caches.RPCGraphCacheDuration = time.Second * 30
	implCfg := loadedConfig.ImplementationConfig(interceptor)
	implCfg.GraphProvider = graphProvider

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
	remoteLNConn    lnrpc.LightningClient
}

func (g *graphProvider) Graph(_ context.Context, dbs *lnd.DatabaseInstances) (
	lnd.GraphSource, error) {

	return NewGraphBackend(dbs.GraphDB, &remoteWrapper{
		graphConn: g.remoteGraphConn,
		lnConn:    g.remoteLNConn,
		local:     dbs.GraphDB,
		net:       &tor.ClearNet{},
	}), nil
}
