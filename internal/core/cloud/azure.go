package cloud

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/sas"
	"github.com/google/uuid"
)

// AzureProvider implements the Provider interface for Azure Blob Storage
type AzureProvider struct {
	client        *azblob.Client
	containerName string
	config        AzureConfig
}

// NewAzureProvider creates a new Azure Blob Storage provider
func NewAzureProvider(config AzureConfig) (*AzureProvider, error) {
	if err := ValidateAzureConfig(config); err != nil {
		return nil, err
	}

	var client *azblob.Client
	var err error

	// Create client using connection string or account name/key
	if config.ConnectionString != "" {
		client, err = azblob.NewClientFromConnectionString(config.ConnectionString, nil)
	} else {
		serviceURL := fmt.Sprintf("https://%s.blob.core.windows.net/", config.StorageAccountName)
		credential, credErr := azblob.NewSharedKeyCredential(config.StorageAccountName, config.StorageAccountKey)
		if credErr != nil {
			return nil, &CloudError{
				Code:    "AZURE_CREDENTIAL_ERROR",
				Message: "failed to create Azure credentials",
				Cause:   credErr,
			}
		}
		client, err = azblob.NewClientWithSharedKeyCredential(serviceURL, credential, nil)
	}

	if err != nil {
		return nil, &CloudError{
			Code:    "AZURE_CLIENT_ERROR",
			Message: "failed to create Azure Blob Storage client",
			Cause:   err,
		}
	}

	// Set default values
	if config.UseHTTPS == false && config.ConnectionString == "" {
		config.UseHTTPS = true // Default to HTTPS
	}

	return &AzureProvider{
		client:        client,
		containerName: config.ContainerName,
		config:        config,
	}, nil
}

// UploadFile uploads a file to Azure Blob Storage
func (p *AzureProvider) UploadFile(ctx context.Context, req *UploadRequest) (*UploadResponse, error) {
	if req == nil {
		return nil, &CloudError{
			Code:    "INVALID_REQUEST",
			Message: "upload request cannot be nil",
		}
	}

	// Generate file ID if not provided
	fileID := req.FileID
	if fileID == "" {
		fileID = uuid.New().String()
		// Add file extension if available
		if req.FileName != "" && strings.Contains(req.FileName, ".") {
			parts := strings.Split(req.FileName, ".")
			if len(parts) > 1 {
				extension := parts[len(parts)-1]
				fileID = fileID + "." + extension
			}
		}
	}

	// Prepare metadata
	metadata := make(map[string]*string)
	if req.FileName != "" {
		metadata["filename"] = to.Ptr(req.FileName)
	}
	for k, v := range req.Metadata {
		metadata[k] = to.Ptr(v)
	}

	// Prepare tags
	tags := make(map[string]string)
	for k, v := range req.Tags {
		tags[k] = v
	}

	// Upload options
	uploadOptions := &azblob.UploadStreamOptions{
		Metadata: metadata,
		Tags:     tags,
	}

	if req.ContentType != "" {
		uploadOptions.HTTPHeaders = &azblob.HTTPHeaders{
			BlobContentType: to.Ptr(req.ContentType),
		}
	}

	// Upload the file
	uploadResponse, err := p.client.UploadStream(ctx, p.containerName, fileID, req.Content, uploadOptions)
	if err != nil {
		return nil, &CloudError{
			Code:    "UPLOAD_FAILED",
			Message: "failed to upload file to Azure Blob Storage",
			Cause:   err,
		}
	}

	// Generate public URL
	publicURL := p.generatePublicURL(fileID)

	response := &UploadResponse{
		FileID:      fileID,
		PublicURL:   publicURL,
		ContentType: req.ContentType,
		UploadedAt:  time.Now().UTC(),
	}

	// Set ETag if available
	if uploadResponse.ETag != nil {
		response.ETag = string(*uploadResponse.ETag)
	}

	// Get file size from content length if provided
	if req.ContentLength > 0 {
		response.Size = req.ContentLength
	}

	return response, nil
}

// GetFileURL generates a public URL for accessing a file
func (p *AzureProvider) GetFileURL(ctx context.Context, fileID string) (string, error) {
	if fileID == "" {
		return "", ErrInvalidFileID
	}

	return p.generatePublicURL(fileID), nil
}

// GetPresignedURL generates a temporary presigned URL for file access
func (p *AzureProvider) GetPresignedURL(ctx context.Context, fileID string, expiration time.Duration) (string, error) {
	if fileID == "" {
		return "", ErrInvalidFileID
	}

	// Check if file exists
	_, err := p.client.NewBlobClient(p.containerName, fileID).GetProperties(ctx, nil)
	if err != nil {
		return "", &CloudError{
			Code:    "FILE_NOT_FOUND",
			Message: "file not found in Azure Blob Storage",
			Cause:   err,
		}
	}

	// Generate SAS token for the blob
	expiryTime := time.Now().UTC().Add(expiration)

	// Create SAS query parameters
	sasQueryParams, err := sas.BlobSignatureValues{
		Protocol:      sas.ProtocolHTTPS,
		ExpiryTime:    expiryTime,
		ContainerName: p.containerName,
		BlobName:      fileID,
		Permissions:   to.Ptr(sas.BlobPermissions{Read: true}).String(),
	}.SignWithSharedKey(p.getSharedKeyCredential())

	if err != nil {
		return "", &CloudError{
			Code:    "SAS_GENERATION_FAILED",
			Message: "failed to generate SAS token",
			Cause:   err,
		}
	}

	// Construct the full URL with SAS token
	blobURL := p.generatePublicURL(fileID)
	sasURL := blobURL + "?" + sasQueryParams.Encode()

	return sasURL, nil
}

// DeleteFile removes a file from Azure Blob Storage
func (p *AzureProvider) DeleteFile(ctx context.Context, fileID string) error {
	if fileID == "" {
		return ErrInvalidFileID
	}

	blobClient := p.client.NewBlobClient(p.containerName, fileID)
	_, err := blobClient.Delete(ctx, nil)
	if err != nil {
		return &CloudError{
			Code:    "DELETE_FAILED",
			Message: "failed to delete file from Azure Blob Storage",
			Cause:   err,
		}
	}

	return nil
}

// ListFiles lists files with optional prefix filtering
func (p *AzureProvider) ListFiles(ctx context.Context, req *ListFilesRequest) (*ListFilesResponse, error) {
	if req == nil {
		req = &ListFilesRequest{}
	}

	listOptions := &container.ListBlobsFlatOptions{
		Include: container.ListBlobsInclude{
			Metadata: true,
			Tags:     true,
		},
	}

	if req.Prefix != "" {
		listOptions.Prefix = to.Ptr(req.Prefix)
	}

	if req.MaxResults > 0 {
		listOptions.MaxResults = to.Ptr(int32(req.MaxResults))
	}

	if req.ContinuationToken != "" {
		listOptions.Marker = to.Ptr(req.ContinuationToken)
	}

	containerClient := p.client.ServiceClient().NewContainerClient(p.containerName)
	pager := containerClient.NewListBlobsFlatPager(listOptions)

	response := &ListFilesResponse{
		Files: []*FileInfo{},
	}

	if pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, &CloudError{
				Code:    "LIST_FAILED",
				Message: "failed to list files from Azure Blob Storage",
				Cause:   err,
			}
		}

		// Convert Azure blob items to FileInfo
		for _, blob := range page.Segment.BlobItems {
			if blob.Name == nil {
				continue
			}

			fileInfo := &FileInfo{
				FileID:    *blob.Name,
				Size:      0,
				PublicURL: p.generatePublicURL(*blob.Name),
				Metadata:  make(map[string]string),
				Tags:      make(map[string]string),
			}

			// Set file size
			if blob.Properties != nil && blob.Properties.ContentLength != nil {
				fileInfo.Size = *blob.Properties.ContentLength
			}

			// Set content type
			if blob.Properties != nil && blob.Properties.ContentType != nil {
				fileInfo.ContentType = *blob.Properties.ContentType
			}

			// Set last modified
			if blob.Properties != nil && blob.Properties.LastModified != nil {
				fileInfo.LastModified = *blob.Properties.LastModified
			}

			// Set ETag
			if blob.Properties != nil && blob.Properties.ETag != nil {
				fileInfo.ETag = string(*blob.Properties.ETag)
			}

			// Convert metadata
			if blob.Metadata != nil {
				for k, v := range blob.Metadata {
					if v != nil {
						if k == "filename" {
							fileInfo.FileName = *v
						}
						fileInfo.Metadata[k] = *v
					}
				}
			}

			// Convert tags
			if blob.BlobTags != nil && blob.BlobTags.BlobTagSet != nil {
				for _, tag := range blob.BlobTags.BlobTagSet {
					if tag.Key != nil && tag.Value != nil {
						fileInfo.Tags[*tag.Key] = *tag.Value
					}
				}
			}

			response.Files = append(response.Files, fileInfo)
		}

		// Set pagination info
		if page.NextMarker != nil {
			response.NextContinuationToken = *page.NextMarker
			response.IsTruncated = true
		}
	}

	return response, nil
}

// GetFileInfo retrieves metadata about a file
func (p *AzureProvider) GetFileInfo(ctx context.Context, fileID string) (*FileInfo, error) {
	if fileID == "" {
		return nil, ErrInvalidFileID
	}

	blobClient := p.client.NewBlobClient(p.containerName, fileID)

	// Get blob properties
	props, err := blobClient.GetProperties(ctx, nil)
	if err != nil {
		return nil, &CloudError{
			Code:    "FILE_NOT_FOUND",
			Message: "file not found in Azure Blob Storage",
			Cause:   err,
		}
	}

	fileInfo := &FileInfo{
		FileID:    fileID,
		Size:      *props.ContentLength,
		PublicURL: p.generatePublicURL(fileID),
		Metadata:  make(map[string]string),
		Tags:      make(map[string]string),
	}

	// Set content type
	if props.ContentType != nil {
		fileInfo.ContentType = *props.ContentType
	}

	// Set last modified
	if props.LastModified != nil {
		fileInfo.LastModified = *props.LastModified
	}

	// Set ETag
	if props.ETag != nil {
		fileInfo.ETag = string(*props.ETag)
	}

	// Convert metadata
	for k, v := range props.Metadata {
		if v != nil {
			if k == "filename" {
				fileInfo.FileName = *v
			}
			fileInfo.Metadata[k] = *v
		}
	}

	// Get blob tags
	tagsResponse, err := blobClient.GetTags(ctx, nil)
	if err == nil && tagsResponse.BlobTagSet != nil {
		for _, tag := range tagsResponse.BlobTagSet {
			if tag.Key != nil && tag.Value != nil {
				fileInfo.Tags[*tag.Key] = *tag.Value
			}
		}
	}

	return fileInfo, nil
}

// generatePublicURL creates a public URL for the blob
func (p *AzureProvider) generatePublicURL(fileID string) string {
	if p.config.BaseURL != "" {
		return fmt.Sprintf("%s/%s/%s", p.config.BaseURL, p.containerName, fileID)
	}

	protocol := "https"
	if !p.config.UseHTTPS {
		protocol = "http"
	}

	return fmt.Sprintf("%s://%s.blob.core.windows.net/%s/%s",
		protocol, p.config.StorageAccountName, p.containerName, url.QueryEscape(fileID))
}

// getSharedKeyCredential returns the shared key credential for SAS generation
func (p *AzureProvider) getSharedKeyCredential() *azblob.SharedKeyCredential {
	credential, _ := azblob.NewSharedKeyCredential(p.config.StorageAccountName, p.config.StorageAccountKey)
	return credential
}
