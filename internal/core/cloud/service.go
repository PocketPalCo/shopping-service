package cloud

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/PocketPalCo/shopping-service/config"
)

// Service provides business logic for cloud storage operations
type Service struct {
	provider Provider
	logger   *slog.Logger
	config   Config
}

// NewService creates a new cloud storage service
func NewService(cfg config.CloudConfig, logger *slog.Logger) (*Service, error) {
	// Convert config types
	cloudConfig := Config{
		Provider: cfg.Provider,
		Azure: AzureConfig{
			StorageAccountName: cfg.Azure.StorageAccountName,
			StorageAccountKey:  cfg.Azure.StorageAccountKey,
			ConnectionString:   cfg.Azure.ConnectionString,
			ContainerName:      cfg.Azure.ContainerName,
			BaseURL:            cfg.Azure.BaseURL,
			UseHTTPS:           cfg.Azure.UseHTTPS,
		},
	}

	// Validate configuration
	if err := ValidateConfig(cloudConfig); err != nil {
		return nil, fmt.Errorf("invalid cloud storage configuration: %w", err)
	}

	// Create provider
	provider, err := NewProvider(cloudConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create cloud storage provider: %w", err)
	}

	return &Service{
		provider: provider,
		logger:   logger,
		config:   cloudConfig,
	}, nil
}

// UploadFileFromTelegram uploads a file from Telegram with metadata
func (s *Service) UploadFileFromTelegram(ctx context.Context, userID int64, chatID int64, fileName string, content io.Reader, contentType string, contentLength int64) (*FileUploadResult, error) {
	// Generate metadata
	metadata := map[string]string{
		"source":   "telegram",
		"user_id":  fmt.Sprintf("%d", userID),
		"chat_id":  fmt.Sprintf("%d", chatID),
		"uploaded": time.Now().UTC().Format(time.RFC3339),
	}

	// Generate tags for organization
	tags := map[string]string{
		"source": "telegram",
		"type":   s.getFileType(fileName, contentType),
	}

	// Prepare upload request
	uploadReq := &UploadRequest{
		FileName:      fileName,
		ContentType:   contentType,
		Content:       content,
		ContentLength: contentLength,
		Metadata:      metadata,
		Tags:          tags,
	}

	// Upload file
	uploadResp, err := s.provider.UploadFile(ctx, uploadReq)
	if err != nil {
		s.logger.Error("Failed to upload file from Telegram",
			"user_id", userID,
			"chat_id", chatID,
			"file_name", fileName,
			"error", err)
		return nil, fmt.Errorf("upload failed: %w", err)
	}

	result := &FileUploadResult{
		FileID:      uploadResp.FileID,
		PublicURL:   uploadResp.PublicURL,
		Size:        uploadResp.Size,
		ContentType: uploadResp.ContentType,
		UploadedAt:  uploadResp.UploadedAt,
		Metadata:    metadata,
	}

	s.logger.Info("Successfully uploaded file from Telegram",
		"user_id", userID,
		"chat_id", chatID,
		"file_name", fileName,
		"file_id", uploadResp.FileID,
		"size", uploadResp.Size,
		"public_url", uploadResp.PublicURL)

	return result, nil
}

// GetSharedFileURL generates a shareable URL for a file
func (s *Service) GetSharedFileURL(ctx context.Context, fileID string) (string, error) {
	url, err := s.provider.GetFileURL(ctx, fileID)
	if err != nil {
		s.logger.Error("Failed to generate shared URL", "file_id", fileID, "error", err)
		return "", fmt.Errorf("failed to get file URL: %w", err)
	}

	s.logger.Info("Generated shared file URL", "file_id", fileID, "url", url)
	return url, nil
}

// GetTemporaryFileURL generates a temporary presigned URL for secure file access
func (s *Service) GetTemporaryFileURL(ctx context.Context, fileID string, expiration time.Duration) (string, error) {
	url, err := s.provider.GetPresignedURL(ctx, fileID, expiration)
	if err != nil {
		s.logger.Error("Failed to generate temporary URL",
			"file_id", fileID,
			"expiration", expiration,
			"error", err)
		return "", fmt.Errorf("failed to get presigned URL: %w", err)
	}

	s.logger.Info("Generated temporary file URL",
		"file_id", fileID,
		"url", url,
		"expiration", expiration)
	return url, nil
}

// DeleteFile removes a file from cloud storage
func (s *Service) DeleteFile(ctx context.Context, fileID string) error {
	err := s.provider.DeleteFile(ctx, fileID)
	if err != nil {
		s.logger.Error("Failed to delete file", "file_id", fileID, "error", err)
		return fmt.Errorf("failed to delete file: %w", err)
	}

	s.logger.Info("Successfully deleted file", "file_id", fileID)
	return nil
}

// ListUserFiles lists files uploaded by a specific user
func (s *Service) ListUserFiles(ctx context.Context, userID int64, maxResults int) ([]*FileInfo, error) {
	// Use prefix filtering to get files for this user
	listReq := &ListFilesRequest{
		MaxResults: maxResults,
	}

	listResp, err := s.provider.ListFiles(ctx, listReq)
	if err != nil {
		s.logger.Error("Failed to list user files", "user_id", userID, "error", err)
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	// Filter files by user ID from metadata
	var userFiles []*FileInfo
	for _, file := range listResp.Files {
		if file.Metadata != nil {
			if fileUserID, exists := file.Metadata["user_id"]; exists && fileUserID == fmt.Sprintf("%d", userID) {
				userFiles = append(userFiles, file)
			}
		}
	}

	s.logger.Info("Listed user files",
		"user_id", userID,
		"total_files", len(listResp.Files),
		"user_files", len(userFiles))

	return userFiles, nil
}

// GetFileInfo retrieves detailed information about a file
func (s *Service) GetFileInfo(ctx context.Context, fileID string) (*FileInfo, error) {
	fileInfo, err := s.provider.GetFileInfo(ctx, fileID)
	if err != nil {
		s.logger.Error("Failed to get file info", "file_id", fileID, "error", err)
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	s.logger.Info("Retrieved file info", "file_id", fileID, "size", fileInfo.Size)
	return fileInfo, nil
}

// ValidateFileForTelegram checks if a file is suitable for Telegram sharing
func (s *Service) ValidateFileForTelegram(fileInfo *FileInfo) error {
	// Telegram file size limits
	const maxFileSizeForBots = 50 * 1024 * 1024 // 50MB for bots

	if fileInfo.Size > maxFileSizeForBots {
		return &CloudError{
			Code:    "FILE_TOO_LARGE",
			Message: fmt.Sprintf("file size %d bytes exceeds Telegram limit of %d bytes", fileInfo.Size, maxFileSizeForBots),
		}
	}

	// Check if content type is supported
	supportedTypes := []string{
		"image/jpeg", "image/png", "image/gif", "image/webp",
		"video/mp4", "video/avi", "video/mov",
		"audio/mpeg", "audio/wav", "audio/ogg",
		"application/pdf", "text/plain",
		"application/zip", "application/x-zip-compressed",
	}

	if fileInfo.ContentType != "" {
		for _, supportedType := range supportedTypes {
			if strings.HasPrefix(fileInfo.ContentType, supportedType) {
				return nil
			}
		}

		s.logger.Warn("Unsupported content type for Telegram",
			"file_id", fileInfo.FileID,
			"content_type", fileInfo.ContentType)
	}

	return nil
}

// getFileType determines file type from filename and content type
func (s *Service) getFileType(fileName, contentType string) string {
	// First try to determine from content type
	if contentType != "" {
		switch {
		case strings.HasPrefix(contentType, "image/"):
			return "image"
		case strings.HasPrefix(contentType, "video/"):
			return "video"
		case strings.HasPrefix(contentType, "audio/"):
			return "audio"
		case strings.HasPrefix(contentType, "text/"):
			return "text"
		case contentType == "application/pdf":
			return "document"
		}
	}

	// Fall back to file extension
	if fileName != "" {
		ext := strings.ToLower(filepath.Ext(fileName))
		switch ext {
		case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp":
			return "image"
		case ".mp4", ".avi", ".mov", ".mkv", ".webm":
			return "video"
		case ".mp3", ".wav", ".ogg", ".aac", ".flac":
			return "audio"
		case ".pdf", ".doc", ".docx", ".txt", ".rtf":
			return "document"
		case ".zip", ".rar", ".7z", ".tar", ".gz":
			return "archive"
		}
	}

	return "unknown"
}

// FileUploadResult contains the result of a file upload operation
type FileUploadResult struct {
	FileID      string
	PublicURL   string
	Size        int64
	ContentType string
	UploadedAt  time.Time
	Metadata    map[string]string
}
