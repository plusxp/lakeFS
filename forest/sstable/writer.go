package sstable

import (
	"fmt"
	"hash"

	"github.com/cockroachdb/pebble/sstable"
	"github.com/treeverse/lakefs/graveler"
	"github.com/treeverse/lakefs/pyramid"
)

type DiskWriter struct {
	w      *sstable.Writer
	tierFS pyramid.FS

	first graveler.Key
	last  graveler.Key
	count int
	hash  hash.Hash

	fh *pyramid.File
}

func newDiskWriter(tierFS pyramid.FS, hash hash.Hash) (*DiskWriter, error) {
	fh, err := tierFS.Create(sstableTierFSNamespace)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}

	writer := sstable.NewWriter(fh, sstable.WriterOptions{
		Compression: sstable.SnappyCompression,
	})

	return &DiskWriter{
		w:      writer,
		fh:     fh,
		tierFS: tierFS,
		hash:   hash,
	}, nil
}

func (dw *DiskWriter) WriteRecord(record graveler.ValueRecord) error {
	keyBytes := []byte(record.Key)
	valBytes, err := serializeValue(*record.Value)
	if err != nil {
		return fmt.Errorf("serializing entry: %w", err)
	}
	if err := dw.w.Set(keyBytes, valBytes); err != nil {
		return fmt.Errorf("setting key and value: %w", err)
	}

	// updating stats
	if dw.count == 0 {
		dw.first = record.Key
	}
	dw.last = record.Identity
	dw.count++

	if _, err := dw.hash.Write(keyBytes); err != nil {
		return err
	}
	if _, err := dw.hash.Write(valBytes); err != nil {
		return err
	}

	return nil
}

func (dw *DiskWriter) Close() (*WriteResult, error) {
	if err := dw.w.Close(); err != nil {
		return nil, fmt.Errorf("sstable file write: %w", err)
	}

	sstableID := fmt.Sprintf("%x", dw.hash.Sum(nil))
	if err := dw.fh.Store(sstableID); err != nil {
		return nil, fmt.Errorf("error storing sstable: %w", err)
	}

	return &WriteResult{
		SSTableID: ID(sstableID),
		First:     dw.first,
		Last:      dw.last,
		Count:     dw.count,
	}, nil
}