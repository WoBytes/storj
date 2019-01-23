// Copyright (C) 2018 Storj Labs, Inc.
// See LICENSE for copying information.

package bwagreement

import (
	"context"

	"github.com/zeebo/errs"
	"go.uber.org/zap"
	monkit "gopkg.in/spacemonkeygo/monkit.v2"

	"storj.io/storj/pkg/pb"
	"storj.io/storj/pkg/provider"
	"storj.io/storj/pkg/uplinkdb"
)

var (
	mon = monkit.Package()
)

// Config is a configuration struct that is everything you need to start an
// agreement receiver responsibility
type Config struct {
}

// Run implements the provider.Responsibility interface
func (c Config) Run(ctx context.Context, server *provider.Provider) (err error) {
	defer mon.Task()(&ctx)(&err)
	k := server.Identity().Leaf.PublicKey

	zap.S().Debug("Starting Bandwidth Agreement Receiver...")

	db, ok := ctx.Value("masterdb").(interface {
		BandwidthAgreement() DB
		UplinkDB() uplinkdb.DB
	})
	if !ok {
		return errs.New("unable to get satellite master db instance")
	}
	pb.RegisterBandwidthServer(server.GRPC(), NewServer(db.BandwidthAgreement(), db.UplinkDB(), zap.L(), k))

	return server.Run(ctx)
}
