package stt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/PocketPalCo/shopping-service/pkg/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	api "go.opentelemetry.io/otel/metric"
)

var tracer = otel.Tracer("stt-client")

type Client struct {
	baseURL    string
	httpClient *http.Client
}

type STTRequest struct {
	SessionID      string
	ChunkID        int
	Language       string
	TargetLanguage string
	AudioData      []byte
	Filename       string
}

type STTResponse struct {
	SessionID        string  `json:"session_id"`
	ChunkID          int     `json:"chunk_id"`
	RawText          string  `json:"raw_text"`
	Translation      string  `json:"translation"`
	ProcessingTimeS  float64 `json:"processing_time_s"`
	DetectedLanguage string  `json:"detected_language"`
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // STT processing can take time
		},
	}
}

func (c *Client) ProcessAudio(ctx context.Context, req STTRequest) (*STTResponse, error) {
	ctx, span := tracer.Start(ctx, "stt.ProcessAudio")
	defer span.End()

	// Set span attributes
	span.SetAttributes(
		attribute.String("session_id", req.SessionID),
		attribute.Int("chunk_id", req.ChunkID),
		attribute.String("language", req.Language),
		attribute.String("target_language", req.TargetLanguage),
		attribute.Int("audio_size_bytes", len(req.AudioData)),
	)

	start := time.Now()

	// Create multipart form data
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add form fields
	writer.WriteField("session_id", req.SessionID)
	writer.WriteField("chunk_id", fmt.Sprintf("%d", req.ChunkID))
	writer.WriteField("language", req.Language)
	writer.WriteField("target_language", req.TargetLanguage)

	// Add file field
	fileWriter, err := writer.CreateFormFile("file", req.Filename)
	if err != nil {
		span.RecordError(err)
		// Record error metric
		if telemetry.TelegramMessagesTotal != nil {
			telemetry.TelegramMessagesTotal.Add(ctx, 1,
				api.WithAttributes(
					attribute.String("type", "voice"),
					attribute.String("status", "stt_form_error"),
				),
			)
		}
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}

	_, err = fileWriter.Write(req.AudioData)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to write audio data: %w", err)
	}

	writer.Close()

	// Make HTTP request
	url := fmt.Sprintf("%s/chunk/", c.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, &buf)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		span.RecordError(err)
		// Record error metric
		if telemetry.TelegramMessagesTotal != nil {
			telemetry.TelegramMessagesTotal.Add(ctx, 1,
				api.WithAttributes(
					attribute.String("type", "voice"),
					attribute.String("status", "stt_request_error"),
				),
			)
		}
		return nil, fmt.Errorf("failed to make STT request: %w", err)
	}
	defer resp.Body.Close()

	// Record duration
	duration := time.Since(start)
	span.SetAttributes(attribute.Float64("stt_request_duration_ms", float64(duration.Milliseconds())))

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		span.RecordError(fmt.Errorf("STT service returned status %d", resp.StatusCode))
		// Record error metric
		if telemetry.TelegramMessagesTotal != nil {
			telemetry.TelegramMessagesTotal.Add(ctx, 1,
				api.WithAttributes(
					attribute.String("type", "voice"),
					attribute.String("status", "stt_service_error"),
					attribute.Int("status_code", resp.StatusCode),
				),
			)
		}
		return nil, fmt.Errorf("STT service returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse response
	var sttResp STTResponse
	if err := json.NewDecoder(resp.Body).Decode(&sttResp); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to decode STT response: %w", err)
	}

	// Set response attributes
	span.SetAttributes(
		attribute.Int("raw_text_length", len(sttResp.RawText)),
		attribute.Int("translation_length", len(sttResp.Translation)),
		attribute.Float64("stt_processing_time_s", sttResp.ProcessingTimeS),
	)

	// Record success metric
	if telemetry.TelegramMessagesTotal != nil {
		telemetry.TelegramMessagesTotal.Add(ctx, 1,
			api.WithAttributes(
				attribute.String("type", "voice"),
				attribute.String("status", "stt_success"),
				attribute.String("language", req.Language),
				attribute.String("target_language", req.TargetLanguage),
			),
		)
	}

	return &sttResp, nil
}
