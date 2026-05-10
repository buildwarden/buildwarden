package main

import "warden/ledger"

// Re-export reader types and constants from warden/ledger.
type Header = ledger.Header
type Record = ledger.Record
type HeaderMeta = ledger.HeaderMeta
type VerifyResult = ledger.VerifyResult

const (
	RecordOpen       = ledger.RecordOpen
	RecordCheckpoint = ledger.RecordCheckpoint
	RecordClose      = ledger.RecordClose
	RecordArtifact   = ledger.RecordArtifact
	SchemaNoMetadata = ledger.SchemaNoMetadata
)

var ReadHeader = ledger.ReadHeader
var ReadRecord = ledger.ReadRecord
var Verify = ledger.Verify
var IsValidLedger = ledger.IsValidLedger
