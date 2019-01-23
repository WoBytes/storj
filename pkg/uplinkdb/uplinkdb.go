// Copyright (C) 2018 Storj Labs, Inc.
// See LICENSE for copying information.

package uplinkdb

import (
	"context"
	"crypto"
	"time"

	"go.uber.org/zap"
)

// DB stores bandwidth agreements.
type DB interface {
	// CreateAgreement adds a new bandwidth agreement.
	CreateAgreement(context.Context, string, Agreement) error
	// GetAgreements gets all bandwidth agreements.
	GetAgreements(context.Context) ([]Agreement, error)
	// GetSignature gets the public key of uplink corresponding to serial number
	GetSignature(ctx context.Context, serialnum string) (*Agreement, error)
	// GetAgreementsSince gets all bandwidth agreements since specific time.
	GetAgreementsSince(context.Context, time.Time) ([]Agreement, error)
}

// Server is an implementation of the pb.BandwidthServer interface
type Server struct {
	db     DB
	pkey   crypto.PublicKey
	logger *zap.Logger
}

// Agreement is a struct that contains a uplinks agreement info
type Agreement struct {
	Agreement []byte // uplink id
	Signature []byte // uplink public key
	CreatedAt time.Time
}

// NewServer creates instance of Server
func NewServer(db DB, logger *zap.Logger, pkey crypto.PublicKey) *Server {
	return &Server{
		db:     db,
		logger: logger,
		pkey:   pkey,
	}
}
