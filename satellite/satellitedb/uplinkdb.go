// Copyright (C) 2018 Storj Labs, Inc.
// See LICENSE for copying information.

package satellitedb

import (
	"context"
	"time"

	"storj.io/storj/pkg/uplinkdb"
	dbx "storj.io/storj/satellite/satellitedb/dbx"
)

type uplinkDB struct {
	db *dbx.DB
}

func (b *uplinkDB) CreateAgreement(ctx context.Context, serialNum string, agreement uplinkdb.Agreement) error {
	_, err := b.db.Create_UplinkDB(
		ctx,
		dbx.UplinkDB_Signature(agreement.Signature),
		dbx.UplinkDB_Serialnum(serialNum),
		dbx.UplinkDB_Data(agreement.Agreement),
	)
	return err
}

func (b *uplinkDB) GetAgreements(ctx context.Context) ([]uplinkdb.Agreement, error) {
	rows, err := b.db.All_UplinkDB(ctx)
	if err != nil {
		return nil, err
	}

	agreements := make([]uplinkdb.Agreement, len(rows))
	for i, entry := range rows {
		agreement := &agreements[i]
		agreement.Signature = entry.Signature
		agreement.Agreement = entry.Data
		agreement.CreatedAt = entry.CreatedAt
	}
	return agreements, nil
}

func (b *uplinkDB) GetSignature(ctx context.Context, serialnum string) (*uplinkdb.Agreement, error) {
	dbxInfo, err := b.db.Get_UplinkDB_By_Serialnum(ctx, dbx.UplinkDB_Serialnum(serialnum))
	if err != nil {
		return &uplinkdb.Agreement{}, err
	}

	return &uplinkdb.Agreement{
		Agreement: dbxInfo.Data,      // Uplink ID
		Signature: dbxInfo.Signature, // Uplink Public Key
	}, nil
}

func (b *uplinkDB) GetAgreementsSince(ctx context.Context, since time.Time) ([]uplinkdb.Agreement, error) {
	rows, err := b.db.All_UplinkDB_By_CreatedAt_Greater(ctx, dbx.UplinkDB_CreatedAt(since))
	if err != nil {
		return nil, err
	}

	agreements := make([]uplinkdb.Agreement, len(rows))
	for i, entry := range rows {
		agreement := &agreements[i]
		agreement.Signature = entry.Signature
		agreement.Agreement = entry.Data
		agreement.CreatedAt = entry.CreatedAt
	}
	return agreements, nil
}
