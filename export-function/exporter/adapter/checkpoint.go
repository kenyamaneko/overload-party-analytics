package adapter

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"export-to-bq/exporter/model"
)

type firestoreCheckpoint struct {
	Table          string    `firestore:"table"`
	LastExportTime time.Time `firestore:"last_export_time"`
	LastRowCount   int64     `firestore:"last_row_count"`
	UpdatedAt      time.Time `firestore:"updated_at"`
}

// FirestoreCheckpointStore manages export checkpoints in Firestore.
type FirestoreCheckpointStore struct {
	client *firestore.Client
}

// NewFirestoreCheckpoint creates a new FirestoreCheckpointStore.
func NewFirestoreCheckpoint(ctx context.Context, projectID string) (*FirestoreCheckpointStore, error) {
	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create Firestore client: %w", err)
	}
	return &FirestoreCheckpointStore{client: client}, nil
}

// Get retrieves the checkpoint for a table.
func (s *FirestoreCheckpointStore) Get(ctx context.Context, table string) (*model.Checkpoint, error) {
	doc, err := s.client.Collection("export_checkpoints").Doc(table).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return &model.Checkpoint{
				Table:          table,
				LastExportTime: time.Time{},
			}, nil
		}
		return nil, fmt.Errorf("get checkpoint: %w", err)
	}

	var fc firestoreCheckpoint
	if err := doc.DataTo(&fc); err != nil {
		return nil, fmt.Errorf("parse checkpoint: %w", err)
	}

	return &model.Checkpoint{
		Table:          fc.Table,
		LastExportTime: fc.LastExportTime,
		LastRowCount:   fc.LastRowCount,
		UpdatedAt:      fc.UpdatedAt,
	}, nil
}

// Update updates the checkpoint for a table.
func (s *FirestoreCheckpointStore) Update(ctx context.Context, table string, cp *model.Checkpoint) error {
	fc := &firestoreCheckpoint{
		Table:          table,
		LastExportTime: cp.LastExportTime,
		LastRowCount:   cp.LastRowCount,
		UpdatedAt:      time.Now(),
	}

	_, err := s.client.Collection("export_checkpoints").Doc(table).Set(ctx, fc)
	if err != nil {
		return fmt.Errorf("update checkpoint: %w", err)
	}

	return nil
}

// Close closes the underlying Firestore client.
func (s *FirestoreCheckpointStore) Close() error {
	return s.client.Close()
}
