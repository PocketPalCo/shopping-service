package cloud

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
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

	provider := &AzureProvider{
		client:        client,
		containerName: config.ContainerName,
		config:        config,
	}

	// Ensure container exists
	ctx := context.Background()
	containerClient := client.ServiceClient().NewContainerClient(config.ContainerName)
	_, err = containerClient.Create(ctx, &container.CreateOptions{
		Metadata: map[string]*string{
			"created_by": to.Ptr("PocketPal"),
			"purpose":    to.Ptr("file_storage"),
		},
	})
	if err != nil {
		// Container might already exist, which is fine
		// We only log this as info, not error
		if !strings.Contains(err.Error(), "ContainerAlreadyExists") {
			return nil, &CloudError{
				Code:    "CONTAINER_CREATE_FAILED",
				Message: "failed to create container in Azure Blob Storage",
				Cause:   err,
			}
		}
	}

	return provider, nil
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
	fileName := req.FileName
	if fileName == "" {
		fileName = "file"
	}

	// Construct full path with folder structure
	var fullPath string
	if req.FolderPath != "" {
		// Ensure folder path doesn't start or end with /
		folderPath := strings.Trim(req.FolderPath, "/")
		if req.FileID != "" {
			fullPath = folderPath + "/" + req.FileID
		} else {
			// Generate unique filename with UUID
			fileID := uuid.New().String()
			if fileName != "" && strings.Contains(fileName, ".") {
				parts := strings.Split(fileName, ".")
				if len(parts) > 1 {
					extension := parts[len(parts)-1]
					fileID = fileID + "." + extension
				}
			}
			fullPath = folderPath + "/" + fileID
		}
	} else {
		if req.FileID != "" {
			fullPath = req.FileID
		} else {
			fileID := uuid.New().String()
			if fileName != "" && strings.Contains(fileName, ".") {
				parts := strings.Split(fileName, ".")
				if len(parts) > 1 {
					extension := parts[len(parts)-1]
					fileID = fileID + "." + extension
				}
			}
			fullPath = fileID
		}
	}

	// Prepare metadata
	metadata := make(map[string]*string)
	if req.FileName != "" {
		metadata["filename"] = to.Ptr(req.FileName)
	}
	if req.FolderPath != "" {
		metadata["folder_path"] = to.Ptr(req.FolderPath)
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
		uploadOptions.HTTPHeaders = &blob.HTTPHeaders{
			BlobContentType: to.Ptr(req.ContentType),
		}
	}

	// Upload the file
	uploadResponse, err := p.client.UploadStream(ctx, p.containerName, fullPath, req.Content, uploadOptions)
	if err != nil {
		return nil, &CloudError{
			Code:    "UPLOAD_FAILED",
			Message: "failed to upload file to Azure Blob Storage",
			Cause:   err,
		}
	}

	// Generate public URL
	publicURL := p.generatePublicURL(fullPath)

	response := &UploadResponse{
		FileID:      fullPath,
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
	blobClient := p.client.ServiceClient().NewContainerClient(p.containerName).NewBlobClient(fileID)
	_, err := blobClient.GetProperties(ctx, nil)
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

	blobClient := p.client.ServiceClient().NewContainerClient(p.containerName).NewBlobClient(fileID)
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

	// Set prefix based on folder path or explicit prefix
	var prefix string
	if req.FolderPath != "" {
		folderPath := strings.Trim(req.FolderPath, "/")
		if req.Recursive {
			prefix = folderPath + "/"
		} else {
			prefix = folderPath + "/"
		}
	}
	if req.Prefix != "" {
		if prefix != "" {
			prefix = prefix + req.Prefix
		} else {
			prefix = req.Prefix
		}
	}

	if prefix != "" {
		listOptions.Prefix = to.Ptr(prefix)
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

			// Parse folder path and filename from blob name
			blobName := *blob.Name
			folderPath, fileName := p.parseBlobPath(blobName)

			fileInfo := &FileInfo{
				FileID:       blobName,
				FolderPath:   folderPath,
				FileName:     fileName,
				RelativePath: blobName,
				Size:         0,
				PublicURL:    p.generatePublicURL(blobName),
				Metadata:     make(map[string]string),
				Tags:         make(map[string]string),
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
						if k == "filename" && fileInfo.FileName == "" {
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

	blobClient := p.client.ServiceClient().NewContainerClient(p.containerName).NewBlobClient(fileID)

	// Get blob properties
	props, err := blobClient.GetProperties(ctx, nil)
	if err != nil {
		return nil, &CloudError{
			Code:    "FILE_NOT_FOUND",
			Message: "file not found in Azure Blob Storage",
			Cause:   err,
		}
	}

	// Parse folder path and filename from fileID
	folderPath, fileName := p.parseBlobPath(fileID)

	fileInfo := &FileInfo{
		FileID:       fileID,
		FolderPath:   folderPath,
		FileName:     fileName,
		RelativePath: fileID,
		Size:         *props.ContentLength,
		PublicURL:    p.generatePublicURL(fileID),
		Metadata:     make(map[string]string),
		Tags:         make(map[string]string),
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
			if k == "filename" && fileInfo.FileName == "" {
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

// CreateFolder creates a virtual folder by creating a marker blob
func (p *AzureProvider) CreateFolder(ctx context.Context, folderPath string) error {
	if folderPath == "" {
		return &CloudError{
			Code:    "INVALID_FOLDER_PATH",
			Message: "folder path cannot be empty",
		}
	}

	// Normalize folder path
	folderPath = strings.Trim(folderPath, "/") + "/"

	// Create a marker blob to represent the folder
	markerBlobName := folderPath + ".folder_marker"

	metadata := map[string]*string{
		"folder_marker": to.Ptr("true"),
		"created_at":    to.Ptr(time.Now().UTC().Format(time.RFC3339)),
	}

	uploadOptions := &azblob.UploadStreamOptions{
		Metadata: metadata,
	}

	// Upload empty content as folder marker
	emptyContent := strings.NewReader("")
	_, err := p.client.UploadStream(ctx, p.containerName, markerBlobName, emptyContent, uploadOptions)
	if err != nil {
		return &CloudError{
			Code:    "FOLDER_CREATE_FAILED",
			Message: "failed to create folder in Azure Blob Storage",
			Cause:   err,
		}
	}

	return nil
}

// ListFolders lists folders within a path
func (p *AzureProvider) ListFolders(ctx context.Context, parentPath string) ([]*FolderInfo, error) {
	// Normalize parent path
	if parentPath != "" {
		parentPath = strings.Trim(parentPath, "/") + "/"
	}

	listOptions := &container.ListBlobsFlatOptions{
		Include: container.ListBlobsInclude{
			Metadata: true,
		},
		Prefix: to.Ptr(parentPath),
	}

	containerClient := p.client.ServiceClient().NewContainerClient(p.containerName)
	pager := containerClient.NewListBlobsFlatPager(listOptions)

	folderMap := make(map[string]*FolderInfo)

	if pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, &CloudError{
				Code:    "LIST_FOLDERS_FAILED",
				Message: "failed to list folders from Azure Blob Storage",
				Cause:   err,
			}
		}

		for _, blob := range page.Segment.BlobItems {
			if blob.Name == nil {
				continue
			}

			blobName := *blob.Name

			// Skip if this blob is not in a deeper folder
			if !strings.HasPrefix(blobName, parentPath) {
				continue
			}

			relativePath := strings.TrimPrefix(blobName, parentPath)

			// Find the immediate folder
			parts := strings.Split(relativePath, "/")
			if len(parts) > 1 {
				folderName := parts[0]
				fullFolderPath := parentPath + folderName

				// Initialize or update folder info
				if folderInfo, exists := folderMap[fullFolderPath]; exists {
					folderInfo.FileCount++
					if blob.Properties != nil && blob.Properties.ContentLength != nil {
						folderInfo.TotalSize += *blob.Properties.ContentLength
					}
					if blob.Properties != nil && blob.Properties.LastModified != nil {
						if blob.Properties.LastModified.After(folderInfo.LastModified) {
							folderInfo.LastModified = *blob.Properties.LastModified
						}
					}
				} else {
					folderInfo := &FolderInfo{
						FolderPath:   fullFolderPath,
						FolderName:   folderName,
						ParentPath:   strings.TrimSuffix(parentPath, "/"),
						FileCount:    1,
						TotalSize:    0,
						CreatedAt:    time.Now().UTC(),
						LastModified: time.Now().UTC(),
						Metadata:     make(map[string]string),
					}

					if blob.Properties != nil && blob.Properties.ContentLength != nil {
						folderInfo.TotalSize = *blob.Properties.ContentLength
					}
					if blob.Properties != nil && blob.Properties.LastModified != nil {
						folderInfo.LastModified = *blob.Properties.LastModified
						folderInfo.CreatedAt = *blob.Properties.LastModified
					}

					folderMap[fullFolderPath] = folderInfo
				}
			}
		}
	}

	// Convert map to slice and sort
	folders := make([]*FolderInfo, 0, len(folderMap))
	for _, folder := range folderMap {
		folders = append(folders, folder)
	}

	// Sort folders by name
	sort.Slice(folders, func(i, j int) bool {
		return folders[i].FolderName < folders[j].FolderName
	})

	return folders, nil
}

// DeleteFolder deletes a folder and all its contents
func (p *AzureProvider) DeleteFolder(ctx context.Context, folderPath string) error {
	if folderPath == "" {
		return &CloudError{
			Code:    "INVALID_FOLDER_PATH",
			Message: "folder path cannot be empty",
		}
	}

	// Normalize folder path
	folderPath = strings.Trim(folderPath, "/") + "/"

	// List all blobs in the folder
	listOptions := &container.ListBlobsFlatOptions{
		Prefix: to.Ptr(folderPath),
	}

	containerClient := p.client.ServiceClient().NewContainerClient(p.containerName)
	pager := containerClient.NewListBlobsFlatPager(listOptions)

	var deletionErrors []string

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return &CloudError{
				Code:    "DELETE_FOLDER_FAILED",
				Message: "failed to list folder contents for deletion",
				Cause:   err,
			}
		}

		// Delete each blob in the folder
		for _, blob := range page.Segment.BlobItems {
			if blob.Name == nil {
				continue
			}

			blobClient := containerClient.NewBlobClient(*blob.Name)
			_, err := blobClient.Delete(ctx, nil)
			if err != nil {
				deletionErrors = append(deletionErrors, fmt.Sprintf("failed to delete %s: %v", *blob.Name, err))
			}
		}
	}

	if len(deletionErrors) > 0 {
		return &CloudError{
			Code:    "DELETE_FOLDER_PARTIAL_FAILURE",
			Message: "some files in folder could not be deleted: " + strings.Join(deletionErrors, "; "),
		}
	}

	return nil
}

// DownloadFile downloads file content directly from Azure Blob Storage
func (p *AzureProvider) DownloadFile(ctx context.Context, fileID string) ([]byte, error) {
	if fileID == "" {
		return nil, ErrInvalidFileID
	}

	// Get blob client
	blobClient := p.client.ServiceClient().NewContainerClient(p.containerName).NewBlobClient(fileID)

	// Download blob content
	response, err := blobClient.DownloadStream(ctx, nil)
	if err != nil {
		return nil, &CloudError{
			Code:    "DOWNLOAD_FAILED",
			Message: "failed to download file from Azure Blob Storage",
			Cause:   err,
		}
	}
	defer func() {
		if response.Body != nil {
			response.Body.Close()
		}
	}()

	// Read all content
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, &CloudError{
			Code:    "READ_FAILED",
			Message: "failed to read file content",
			Cause:   err,
		}
	}

	return data, nil
}

// parseBlobPath splits a blob name into folder path and filename
func (p *AzureProvider) parseBlobPath(blobName string) (folderPath, fileName string) {
	if !strings.Contains(blobName, "/") {
		return "", blobName
	}

	lastSlash := strings.LastIndex(blobName, "/")
	folderPath = blobName[:lastSlash]
	fileName = blobName[lastSlash+1:]
	return folderPath, fileName
}

// getSharedKeyCredential returns the shared key credential for SAS generation
func (p *AzureProvider) getSharedKeyCredential() *azblob.SharedKeyCredential {
	credential, _ := azblob.NewSharedKeyCredential(p.config.StorageAccountName, p.config.StorageAccountKey)
	return credential
}
