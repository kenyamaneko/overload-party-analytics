package exporter

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Checkpoint represents the export state for a table
type Checkpoint struct {
	Table           string    `firestore:"table"`
	LastExportTime  time.Time `firestore:"last_export_time"`
	LastRowCount    int64     `firestore:"last_row_count"`
	UpdatedAt       time.Time `firestore:"updated_at"`
}

// CheckpointStore manages export checkpoints in Firestore
type CheckpointStore struct {
	client *firestore.Client
}

// NewCheckpointStore creates a new CheckpointStore
func NewCheckpointStore(client *firestore.Client) *CheckpointStore {
	return &CheckpointStore{client: client}
}

// Get retrieves the checkpoint for a table
func (s *CheckpointStore) Get(ctx context.Context, table string) (*Checkpoint, error) {
	doc, err := s.client.Collection("export_checkpoints").Doc(table).Get(ctx)
	if err != nil {
		// If not found, return a default checkpoint (first time export)
		if status.Code(err) == codes.NotFound {
			return &Checkpoint{
				Table:          table,
				LastExportTime: time.Time{}, // Epoch
			}, nil
		}
		return nil, fmt.Errorf("get checkpoint: %w", err)
	}

	var cp Checkpoint
	if err := doc.DataTo(&cp); err != nil {
		return nil, fmt.Errorf("parse checkpoint: %w", err)
	}

	return &cp, nil
}

// Update updates the checkpoint for a table
func (s *CheckpointStore) Update(ctx context.Context, table string, cp *Checkpoint) error {
	cp.Table = table
	cp.UpdatedAt = time.Now()

	_, err := s.client.Collection("export_checkpoints").Doc(table).Set(ctx, cp)
	if err != nil {
		return fmt.Errorf("update checkpoint: %w", err)
	}

	return nil
}
