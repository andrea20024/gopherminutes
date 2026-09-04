// Package mongo provides GridFS client for storing audio files.
package mongo

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// GridFSClient provides methods for storing and retrieving files via MongoDB GridFS.
type GridFSClient struct {
	db     *mongo.Database
	bucket string
}

// NewGridFSClient creates a new GridFSClient for the given database and bucket name.
func NewGridFSClient(db *mongo.Database, bucketName string) *GridFSClient {
	return &GridFSClient{db: db, bucket: bucketName}
}

// UploadFile uploads a local file to GridFS and returns the file ID.
func (c *GridFSClient) UploadFile(ctx context.Context, filePath string) (interface{}, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	// Store only the file name, not the full path
	fileName := filepath.Base(filePath)
	bucket := c.db.GridFSBucket(options.GridFSBucket().SetName(c.bucket))
	stream, err := bucket.OpenUploadStream(ctx, fileName)
	if err != nil {
		return nil, fmt.Errorf("open upload stream: %w", err)
	}

	n, err := io.Copy(stream, f)
	if err != nil {
		stream.Close()
		return nil, fmt.Errorf("copy file data (%d bytes): %w", n, err)
	}
	if err := stream.Close(); err != nil {
		return nil, fmt.Errorf("close upload stream: %w", err)
	}

	return stream.FileID, nil
}

// UploadFromReader uploads data from a reader to GridFS with the given filename.
func (c *GridFSClient) UploadFromReader(ctx context.Context, reader io.Reader, filename string) (interface{}, error) {
	bucket := c.db.GridFSBucket(options.GridFSBucket().SetName(c.bucket))
	stream, err := bucket.OpenUploadStream(ctx, filename)
	if err != nil {
		return nil, fmt.Errorf("open upload stream: %w", err)
	}

	if _, err := io.Copy(stream, reader); err != nil {
		stream.Close()
		return nil, fmt.Errorf("copy data: %w", err)
	}

	if err := stream.Close(); err != nil {
		return nil, fmt.Errorf("close upload stream: %w", err)
	}

	return stream.FileID, nil
}

// UploadFromMultipart uploads a multipart form file to GridFS.
func (c *GridFSClient) UploadFromMultipart(ctx context.Context, header *multipart.FileHeader) (interface{}, error) {
	f, err := header.Open()
	if err != nil {
		return nil, fmt.Errorf("open multipart file: %w", err)
	}
	defer f.Close()

	bucket := c.db.GridFSBucket(options.GridFSBucket().SetName(c.bucket))
	stream, err := bucket.OpenUploadStream(ctx, header.Filename)
	if err != nil {
		return nil, fmt.Errorf("open upload stream: %w", err)
	}

	if _, err := io.Copy(stream, f); err != nil {
		stream.Close()
		return nil, fmt.Errorf("copy file data: %w", err)
	}

	if err := stream.Close(); err != nil {
		return nil, fmt.Errorf("close upload stream: %w", err)
	}

	return stream.FileID, nil
}

// DownloadFile downloads a file from GridFS by ID and writes it to the given path.
func (c *GridFSClient) DownloadFile(ctx context.Context, fileID interface{}, destPath string) error {
	bucket := c.db.GridFSBucket(options.GridFSBucket().SetName(c.bucket))
	stream, err := bucket.OpenDownloadStream(ctx, fileID)
	if err != nil {
		return fmt.Errorf("open download stream: %w", err)
	}
	defer stream.Close()

	dest, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create destination file: %w", err)
	}
	defer dest.Close()

	if _, err := io.Copy(dest, stream); err != nil {
		return fmt.Errorf("copy file data: %w", err)
	}

	return nil
}

// DownloadToReader downloads a file from GridFS and returns its data.
func (c *GridFSClient) DownloadToReader(ctx context.Context, fileID interface{}) ([]byte, error) {
	bucket := c.db.GridFSBucket(options.GridFSBucket().SetName(c.bucket))
	stream, err := bucket.OpenDownloadStream(ctx, fileID)
	if err != nil {
		return nil, fmt.Errorf("open download stream: %w", err)
	}
	defer stream.Close()

	data, err := io.ReadAll(stream)
	if err != nil {
		return nil, fmt.Errorf("read file data: %w", err)
	}

	return data, nil
}

// DeleteFile removes a file from GridFS by ID.
func (c *GridFSClient) DeleteFile(ctx context.Context, fileID interface{}) error {
	oid, ok := fileID.(bson.ObjectID)
	if !ok {
		return fmt.Errorf("invalid file ID type, expected ObjectID")
	}

	coll := c.db.Collection(c.bucket + ".files")
	_, err := coll.DeleteOne(ctx, bson.M{"_id": oid})
	if err != nil {
		return fmt.Errorf("delete file: %w", err)
	}

	return nil
}

// ParseGridFSID parses a GridFS ObjectID string.
// Handles both raw hex format and MongoDB's string representation: ObjectID("hex").
func ParseGridFSID(raw string) (bson.ObjectID, error) {
	raw = strings.TrimPrefix(raw, "ObjectID(\"")
	raw = strings.TrimSuffix(raw, "\")")
	return bson.ObjectIDFromHex(raw)
}

// FileExists checks if a file exists in GridFS by ID.
func (c *GridFSClient) FileExists(ctx context.Context, fileID interface{}) bool {
	oid, ok := fileID.(bson.ObjectID)
	if !ok {
		return false
	}

	coll := c.db.Collection(c.bucket + ".files")
	count, err := coll.CountDocuments(ctx, bson.M{"_id": oid})
	if err != nil {
		return false
	}

	return count > 0
}
