package cloud

import (
	"context"
	"io"
	"time"
)

// Provider defines the interface for cloud storage providers
type Provider interface {
	// UploadFile uploads a file to cloud storage and returns the file ID and public URL
	UploadFile(ctx context.Context, req *UploadRequest) (*UploadResponse, error)

	// GetFileURL generates a public URL for accessing a file
	GetFileURL(ctx context.Context, fileID string) (string, error)

	// GetPresignedURL generates a temporary presigned URL for file access
	GetPresignedURL(ctx context.Context, fileID string, expiration time.Duration) (string, error)

	// DeleteFile removes a file from cloud storage
	DeleteFile(ctx context.Context, fileID string) error

	// ListFiles lists files with optional prefix filtering
	ListFiles(ctx context.Context, req *ListFilesRequest) (*ListFilesResponse, error)

	// GetFileInfo retrieves metadata about a file
	GetFileInfo(ctx context.Context, fileID string) (*FileInfo, error)
}

// UploadRequest contains parameters for file upload
type UploadRequest struct {
	// FileID is the unique identifier for the file (can be auto-generated if empty)
	FileID string

	// FileName is the original filename
	FileName string

	// ContentType is the MIME type of the file
	ContentType string

	// Content is the file content to upload
	Content io.Reader

	// ContentLength is the size of the file in bytes (-1 if unknown)
	ContentLength int64

	// Metadata contains custom metadata for the file
	Metadata map[string]string

	// Tags are used for organization and billing
	Tags map[string]string
}

// UploadResponse contains the result of a file upload
type UploadResponse struct {
	// FileID is the unique identifier for the uploaded file
	FileID string

	// PublicURL is the public URL for accessing the file
	PublicURL string

	// Size is the actual size of the uploaded file in bytes
	Size int64

	// ContentType is the detected/specified MIME type
	ContentType string

	// ETag is the entity tag for the file (for caching)
	ETag string

	// UploadedAt is the timestamp when the file was uploaded
	UploadedAt time.Time
}

// ListFilesRequest contains parameters for listing files
type ListFilesRequest struct {
	// Prefix filters files by prefix
	Prefix string

	// MaxResults limits the number of results (0 for no limit)
	MaxResults int

	// ContinuationToken for pagination
	ContinuationToken string
}

// ListFilesResponse contains the result of listing files
type ListFilesResponse struct {
	// Files is the list of file information
	Files []*FileInfo

	// NextContinuationToken for pagination (empty if no more results)
	NextContinuationToken string

	// IsTruncated indicates if there are more results
	IsTruncated bool
}

// FileInfo contains metadata about a file
type FileInfo struct {
	// FileID is the unique identifier for the file
	FileID string

	// FileName is the original filename
	FileName string

	// Size is the file size in bytes
	Size int64

	// ContentType is the MIME type
	ContentType string

	// ETag is the entity tag for the file
	ETag string

	// LastModified is the last modification timestamp
	LastModified time.Time

	// PublicURL is the public URL for accessing the file
	PublicURL string

	// Metadata contains custom metadata
	Metadata map[string]string

	// Tags are used for organization and billing
	Tags map[string]string
}

// Config contains cloud provider configuration
type Config struct {
	// Provider specifies which cloud provider to use (azure, aws, gcp)
	Provider string

	// Azure Blob Storage configuration
	Azure AzureConfig

	// AWS S3 configuration (for future use)
	AWS AWSConfig

	// GCP Cloud Storage configuration (for future use)
	GCP GCPConfig
}

// AzureConfig contains Azure Blob Storage specific configuration
type AzureConfig struct {
	// StorageAccountName is the Azure storage account name
	StorageAccountName string

	// StorageAccountKey is the Azure storage account key
	StorageAccountKey string

	// ConnectionString is the full connection string (alternative to name/key)
	ConnectionString string

	// ContainerName is the blob container name
	ContainerName string

	// BaseURL is the base URL for blob access (optional, auto-generated if empty)
	BaseURL string

	// UseHTTPS determines whether to use HTTPS for blob URLs
	UseHTTPS bool
}

// AWSConfig contains AWS S3 specific configuration (for future use)
type AWSConfig struct {
	// AccessKeyID is the AWS access key ID
	AccessKeyID string

	// SecretAccessKey is the AWS secret access key
	SecretAccessKey string

	// SessionToken for temporary credentials
	SessionToken string

	// Region is the AWS region
	Region string

	// BucketName is the S3 bucket name
	BucketName string

	// Endpoint is the S3 endpoint URL (for S3-compatible services)
	Endpoint string
}

// GCPConfig contains Google Cloud Storage specific configuration (for future use)
type GCPConfig struct {
	// ProjectID is the GCP project ID
	ProjectID string

	// BucketName is the GCS bucket name
	BucketName string

	// CredentialsJSON is the service account credentials JSON
	CredentialsJSON string

	// CredentialsFile is the path to the service account credentials file
	CredentialsFile string
}

// Error types for cloud operations
var (
	ErrFileNotFound     = &CloudError{Code: "FILE_NOT_FOUND", Message: "File not found"}
	ErrInvalidFileID    = &CloudError{Code: "INVALID_FILE_ID", Message: "Invalid file ID"}
	ErrUploadFailed     = &CloudError{Code: "UPLOAD_FAILED", Message: "File upload failed"}
	ErrAccessDenied     = &CloudError{Code: "ACCESS_DENIED", Message: "Access denied"}
	ErrQuotaExceeded    = &CloudError{Code: "QUOTA_EXCEEDED", Message: "Storage quota exceeded"}
	ErrInvalidConfig    = &CloudError{Code: "INVALID_CONFIG", Message: "Invalid configuration"}
	ErrProviderNotFound = &CloudError{Code: "PROVIDER_NOT_FOUND", Message: "Cloud provider not found"}
)

// CloudError represents a cloud storage error
type CloudError struct {
	Code    string
	Message string
	Cause   error
}

func (e *CloudError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *CloudError) Unwrap() error {
	return e.Cause
}
