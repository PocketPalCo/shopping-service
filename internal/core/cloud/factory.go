package cloud

import (
	"fmt"
	"strings"
)

// NewProvider creates a new cloud storage provider based on the configuration
func NewProvider(config Config) (Provider, error) {
	provider := strings.ToLower(config.Provider)

	switch provider {
	case "azure":
		return NewAzureProvider(config.Azure)
	case "aws":
		return nil, fmt.Errorf("AWS S3 provider not yet implemented")
	case "gcp":
		return nil, fmt.Errorf("GCP Cloud Storage provider not yet implemented")
	default:
		return nil, &CloudError{
			Code:    "INVALID_PROVIDER",
			Message: fmt.Sprintf("unsupported cloud provider: %s", provider),
		}
	}
}

// ValidateConfig validates the cloud provider configuration
func ValidateConfig(config Config) error {
	if config.Provider == "" {
		return &CloudError{
			Code:    "MISSING_PROVIDER",
			Message: "cloud provider must be specified",
		}
	}

	provider := strings.ToLower(config.Provider)

	switch provider {
	case "azure":
		return ValidateAzureConfig(config.Azure)
	case "aws":
		return ValidateAWSConfig(config.AWS)
	case "gcp":
		return ValidateGCPConfig(config.GCP)
	default:
		return &CloudError{
			Code:    "INVALID_PROVIDER",
			Message: fmt.Sprintf("unsupported cloud provider: %s", provider),
		}
	}
}

// ValidateAzureConfig validates Azure Blob Storage configuration
func ValidateAzureConfig(config AzureConfig) error {
	if config.ConnectionString == "" {
		if config.StorageAccountName == "" {
			return &CloudError{
				Code:    "MISSING_AZURE_ACCOUNT",
				Message: "Azure storage account name or connection string is required",
			}
		}
		if config.StorageAccountKey == "" {
			return &CloudError{
				Code:    "MISSING_AZURE_KEY",
				Message: "Azure storage account key is required when not using connection string",
			}
		}
	}

	if config.ContainerName == "" {
		return &CloudError{
			Code:    "MISSING_AZURE_CONTAINER",
			Message: "Azure blob container name is required",
		}
	}

	return nil
}

// ValidateAWSConfig validates AWS S3 configuration (for future use)
func ValidateAWSConfig(config AWSConfig) error {
	if config.AccessKeyID == "" {
		return &CloudError{
			Code:    "MISSING_AWS_ACCESS_KEY",
			Message: "AWS access key ID is required",
		}
	}

	if config.SecretAccessKey == "" {
		return &CloudError{
			Code:    "MISSING_AWS_SECRET_KEY",
			Message: "AWS secret access key is required",
		}
	}

	if config.Region == "" {
		return &CloudError{
			Code:    "MISSING_AWS_REGION",
			Message: "AWS region is required",
		}
	}

	if config.BucketName == "" {
		return &CloudError{
			Code:    "MISSING_AWS_BUCKET",
			Message: "AWS S3 bucket name is required",
		}
	}

	return nil
}

// ValidateGCPConfig validates GCP Cloud Storage configuration (for future use)
func ValidateGCPConfig(config GCPConfig) error {
	if config.ProjectID == "" {
		return &CloudError{
			Code:    "MISSING_GCP_PROJECT",
			Message: "GCP project ID is required",
		}
	}

	if config.BucketName == "" {
		return &CloudError{
			Code:    "MISSING_GCP_BUCKET",
			Message: "GCP Cloud Storage bucket name is required",
		}
	}

	if config.CredentialsJSON == "" && config.CredentialsFile == "" {
		return &CloudError{
			Code:    "MISSING_GCP_CREDENTIALS",
			Message: "GCP credentials JSON or credentials file is required",
		}
	}

	return nil
}
