package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"
)

// TileStorage provides tile-specific storage operations
type TileStorage struct {
	*S3Storage
}

func NewTileStorage(s3Storage *S3Storage) *TileStorage {
	return &TileStorage{S3Storage: s3Storage}
}

// TileKey generates an S3 key for a tile
func (t *TileStorage) TileKey(layer string, z, x, y int, tenantID *string) string {
	if tenantID != nil {
		return fmt.Sprintf("tiles/%s/%s/%d/%d/%d.pbf", *tenantID, layer, z, x, y)
	}
	return fmt.Sprintf("tiles/public/%s/%d/%d/%d.pbf", layer, z, x, y)
}

// ExportKey generates an S3 key for an export
func (t *TileStorage) ExportKey(tenantID, exportID, kind string) string {
	return fmt.Sprintf("exports/%s/%s.%s", tenantID, exportID, kind)
}

// GetTileURL generates a presigned URL for a tile
func (t *TileStorage) GetTileURL(ctx context.Context, layer string, z, x, y int, tenantID *string, duration time.Duration) (string, error) {
	key := t.TileKey(layer, z, x, y, tenantID)
	return t.GeneratePresignedURL(ctx, key, duration)
}

// GetExportURL generates a presigned URL for an export
func (t *TileStorage) GetExportURL(ctx context.Context, tenantID, exportID, kind string, duration time.Duration) (string, error) {
	key := t.ExportKey(tenantID, exportID, kind)
	return t.GeneratePresignedURL(ctx, key, duration)
}

// UploadTile uploads a tile to S3
func (t *TileStorage) UploadTile(ctx context.Context, layer string, z, x, y int, tenantID *string, data []byte) error {
	key := t.TileKey(layer, z, x, y, tenantID)
	return t.Upload(ctx, key, bytesReader(data), "application/x-protobuf")
}

// UploadExport uploads an export to S3
func (t *TileStorage) UploadExport(ctx context.Context, tenantID, exportID, kind string, data []byte, contentType string) error {
	key := t.ExportKey(tenantID, exportID, kind)
	return t.Upload(ctx, key, bytesReader(data), contentType)
}

// TileExists checks if a tile exists
func (t *TileStorage) TileExists(ctx context.Context, layer string, z, x, y int, tenantID *string) (bool, error) {
	key := t.TileKey(layer, z, x, y, tenantID)
	return t.Exists(ctx, key)
}

// bytesReader creates an io.Reader from bytes
func bytesReader(data []byte) io.Reader {
	return bytes.NewReader(data)
}
