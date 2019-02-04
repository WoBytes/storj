// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package bwagreement

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"strings"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/gtank/cryptopasta"
	"github.com/zeebo/errs"
	"go.uber.org/zap"
	monkit "gopkg.in/spacemonkeygo/monkit.v2"

	"storj.io/storj/pkg/auth"
	"storj.io/storj/pkg/identity"
	"storj.io/storj/pkg/pb"
	"storj.io/storj/pkg/peertls"
	"storj.io/storj/pkg/storj"
	"storj.io/storj/pkg/uplinkdb"
)

var (
	// Error the default bwagreement errs class
	Error = errs.Class("bwagreement error")
	mon   = monkit.Package()
)

// Config is a configuration struct that is everything you need to start an
// agreement receiver responsibility
type Config struct {
}

//UplinkStat contains information about an uplink's returned bandwidth agreement
type UplinkStat struct {
	NodeID            storj.NodeID
	TotalBytes        int64
	PutActionCount    int
	GetActionCount    int
	TotalTransactions int
}

// DB stores bandwidth agreements.
type DB interface {
	// CreateAgreement adds a new bandwidth agreement.
	CreateAgreement(context.Context, *pb.RenterBandwidthAllocation) error
	// GetTotalsSince returns the sum of each bandwidth type after (exluding) a given date range
	GetTotals(context.Context, time.Time, time.Time) (map[storj.NodeID][]int64, error)
	//GetTotals returns stats about an uplink
	GetUplinkStats(context.Context, time.Time, time.Time) ([]UplinkStat, error)
}

// Server is an implementation of the pb.BandwidthServer interface
type Server struct {
	db       DB
	uplinkdb uplinkdb.DB
	NodeID   storj.NodeID
	logger   *zap.Logger
}

// NewServer creates instance of Server
func NewServer(db DB, upldb uplinkdb.DB, logger *zap.Logger, nodeID storj.NodeID) *Server {
	// TODO: reorder arguments, rename logger -> log
	return &Server{db: db, uplinkdb: upldb, logger: logger, NodeID: nodeID}
}

// Close closes resources
func (s *Server) Close() error { return nil }

// BandwidthAgreements receives and stores bandwidth agreements from storage nodes
func (s *Server) BandwidthAgreements(ctx context.Context, rba *pb.RenterBandwidthAllocation) (reply *pb.AgreementsSummary, err error) {
	defer mon.Task()(&ctx)(&err)
	s.logger.Debug("Received Agreement...")
	reply = &pb.AgreementsSummary{
		Status: pb.AgreementsSummary_REJECTED,
	}
	pba := rba.PayerAllocation
	//verify message content
	pi, err := identity.PeerIdentityFromContext(ctx)
	if err != nil || rba.StorageNodeId != pi.ID {
		return reply, auth.ErrBadID.New("Storage Node ID: %v vs %v", rba.StorageNodeId, pi.ID)
	}
	//todo:  use whitelist for uplinks?
	if pba.SatelliteId != s.NodeID {
		return reply, pb.ErrPayer.New("Satellite ID: %v vs %v", pba.SatelliteId, s.NodeID)
	}
	exp := time.Unix(pba.GetExpirationUnixSec(), 0).UTC()
	if exp.Before(time.Now().UTC()) {
		return reply, pb.ErrPayer.Wrap(auth.ErrExpired.New("%v vs %v", exp, time.Now().UTC()))
	}
	//verify message crypto
	if err := auth.VerifyMsg(rba, pba.UplinkId); err != nil {
		return reply, pb.ErrRenter.Wrap(err)
	}

	if err := auth.VerifyMsg(&pba, pba.SatelliteId); err != nil {
		return reply, pb.ErrPayer.Wrap(err)
	}

	if err = s.verifySignature(ctx, rba); err != nil {
		return reply, err
	}

	//save and return rersults
	if err = s.db.CreateAgreement(ctx, rba); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") ||
			strings.Contains(err.Error(), "violates unique constraint") {
			return reply, pb.ErrPayer.Wrap(auth.ErrSerial.Wrap(err))
		}
		reply.Status = pb.AgreementsSummary_FAIL
		return reply, pb.ErrPayer.Wrap(err)
	}
	reply.Status = pb.AgreementsSummary_OK
	s.logger.Debug("Stored Agreement...")
	return reply, nil
}

func (s *Server) verifySignature(ctx context.Context, rba *pb.RenterBandwidthAllocation) error {
	pba := rba.GetPayerAllocation()

	// Get renter's public key from uplink agreement db
	uplinkInfo, err := s.uplinkdb.GetSignature(ctx, pba.GetSerialNumber())
	if err != nil {
		return Error.New("Failed to unmarshal PayerBandwidthAllocation: %+v", err)
	}

	pubkey, err := x509.ParsePKIXPublicKey(uplinkInfo.Signature)
	if err != nil {
		return Error.New("Failed to extract Public Key from RenterBandwidthAllocation: %+v", err)
	}

	// Typecast public key
	k, ok := pubkey.(*ecdsa.PublicKey)
	if !ok {
		return peertls.ErrUnsupportedKey.New("%T", pubkey)
	}

	signatureLength := k.Curve.Params().P.BitLen() / 8
	if len(rba.GetSignature()) < signatureLength {
		return Error.New("Invalid Renter's Signature Length")
	}

	// verify Renter's (uplink) signature
	rbad := rba
	rbad.SetSignature(nil)
	rbadBytes, err := proto.Marshal(rbad)
	if err != nil {
		return Error.New("marshalling error: %+v", err)
	}

	if ok := cryptopasta.Verify(rbadBytes, rba.GetSignature(), k); !ok {
		return Error.New("Failed to verify Renter's Signature")
	}

	// retrieve satellite public key
	pi, err := identity.PeerIdentityFromContext(ctx)
	if err != nil {
		return err
	}

	k, ok = pi.Leaf.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return peertls.ErrUnsupportedKey.New("%T", pi.Leaf.PublicKey)
	}

	signatureLength = k.Curve.Params().P.BitLen() / 8
	if len(pba.GetSignature()) < signatureLength {
		return Error.New("Inavalid Payer's Signature Length")
	}

	// verify Payer's (satellite) signature
	pbad := pba
	pbad.SetSignature(nil)
	pbadBytes, err := proto.Marshal(&pbad)
	if err != nil {
		return Error.New("marshalling error: %+v", err)
	}

	if ok := cryptopasta.Verify(pbadBytes, pba.GetSignature(), k); !ok {
		return Error.New("Failed to verify Payer's Signature")
	}
	return nil
}
