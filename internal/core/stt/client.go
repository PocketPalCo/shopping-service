package stt

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Microsoft/cognitive-services-speech-sdk-go/audio"
	"github.com/Microsoft/cognitive-services-speech-sdk-go/common"
	"github.com/Microsoft/cognitive-services-speech-sdk-go/speech"
	"github.com/PocketPalCo/shopping-service/pkg/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	api "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("stt-client")

type Client struct {
	subscriptionKey string
	region          string

	// Metrics
	processRequestsTotal   api.Int64Counter
	processRequestDuration api.Float64Histogram
	processRequestErrors   api.Int64Counter
	audioLengthHistogram   api.Float64Histogram
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

func NewClient(subscriptionKey, region string) *Client {
	meter := otel.Meter("azure_speech_service")

	// Initialize metrics
	processRequestsTotal, _ := meter.Int64Counter(
		"azure_speech_process_requests_total",
		api.WithDescription("Total number of speech-to-text processing requests"),
		api.WithUnit("1"),
	)

	processRequestDuration, _ := meter.Float64Histogram(
		"azure_speech_process_duration_seconds",
		api.WithDescription("Duration of speech-to-text processing requests"),
		api.WithUnit("s"),
	)

	processRequestErrors, _ := meter.Int64Counter(
		"azure_speech_process_errors_total",
		api.WithDescription("Total number of speech-to-text processing errors"),
		api.WithUnit("1"),
	)

	audioLengthHistogram, _ := meter.Float64Histogram(
		"azure_speech_audio_length_seconds",
		api.WithDescription("Length of audio files processed"),
		api.WithUnit("s"),
	)

	return &Client{
		subscriptionKey: subscriptionKey,
		region:          region,

		// Metrics
		processRequestsTotal:   processRequestsTotal,
		processRequestDuration: processRequestDuration,
		processRequestErrors:   processRequestErrors,
		audioLengthHistogram:   audioLengthHistogram,
	}
}

func (c *Client) ProcessAudio(ctx context.Context, req STTRequest) (*STTResponse, error) {
	startTime := time.Now()

	// Record processing request metrics
	attrs := []attribute.KeyValue{
		attribute.String("session_id", req.SessionID),
		attribute.Int("chunk_id", req.ChunkID),
		attribute.String("language", req.Language),
		attribute.String("target_language", req.TargetLanguage),
		attribute.Int("audio_size_bytes", len(req.AudioData)),
		attribute.String("filename", req.Filename),
	}
	c.processRequestsTotal.Add(ctx, 1, api.WithAttributes(attrs...))

	// Record audio length (estimated from size - rough approximation)
	estimatedAudioLength := float64(len(req.AudioData)) / 16000.0 // Assuming 16kHz mono
	c.audioLengthHistogram.Record(ctx, estimatedAudioLength, api.WithAttributes(attrs...))

	// If no language specified, try common languages in order of likelihood
	languages := []string{req.Language}
	if req.Language == "" {
		languages = []string{"ru-RU", "uk-UA", "en-US"} // Try Russian first, then Ukrainian, then English
	}

	// Try each language until we get a successful recognition
	var lastError error
	var failedLanguages []string

	for _, lang := range languages {
		if lang == "" {
			continue
		}

		result, err := c.processAudioWithLanguage(ctx, req, lang)
		if err == nil && result != nil && result.RawText != "" {
			// Successful recognition - record success metrics
			duration := time.Since(startTime)
			successAttrs := append(attrs,
				attribute.String("outcome", "success"),
				attribute.String("detected_language", result.DetectedLanguage),
				attribute.Int("text_length", len(result.RawText)),
				attribute.Float64("processing_time_s", result.ProcessingTimeS),
			)
			c.processRequestDuration.Record(ctx, duration.Seconds(), api.WithAttributes(successAttrs...))

			return result, nil
		}

		// Store the error for detailed logging
		lastError = err
		failedLanguages = append(failedLanguages, lang)

		// Log the failed attempt with detailed error information
		span := trace.SpanFromContext(ctx)
		span.SetAttributes(
			attribute.String("attempted_language", lang),
			attribute.String("attempt_result", "failed"),
			attribute.String("attempt_error", err.Error()),
		)

		// Log detailed failure information
		slog.Debug("STT language attempt failed",
			"language", lang,
			"error", err,
			"audio_size_bytes", len(req.AudioData),
			"session_id", req.SessionID,
			"component", "stt_processing")
	}

	// All languages failed, return error with detailed information
	duration := time.Since(startTime)
	errorAttrs := append(attrs,
		attribute.String("outcome", "failed"),
		attribute.String("error_type", "recognition_failed"),
		attribute.String("error_category", "no_languages_matched"),
		attribute.StringSlice("failed_languages", failedLanguages),
		attribute.String("last_error", lastError.Error()),
	)
	c.processRequestErrors.Add(ctx, 1, api.WithAttributes(errorAttrs...))
	c.processRequestDuration.Record(ctx, duration.Seconds(), api.WithAttributes(errorAttrs...))

	// Enhanced error message with debugging information
	errorMsg := fmt.Sprintf("speech recognition failed for all attempted languages %v. Last error: %v. Audio size: %d bytes, Duration: %dms",
		failedLanguages, lastError, len(req.AudioData), duration.Milliseconds())

	return nil, fmt.Errorf(errorMsg)
}

func (c *Client) processAudioWithLanguage(ctx context.Context, req STTRequest, language string) (*STTResponse, error) {
	ctx, span := tracer.Start(ctx, "stt.ProcessAudio")
	defer span.End()

	// Debug log for each language attempt
	slog.Debug("STT processing attempt",
		"language", language,
		"audio_size_bytes", len(req.AudioData),
		"session_id", req.SessionID,
		"component", "stt_processing")

	// Set span attributes
	span.SetAttributes(
		attribute.String("session_id", req.SessionID),
		attribute.Int("chunk_id", req.ChunkID),
		attribute.String("language", req.Language),
		attribute.String("target_language", req.TargetLanguage),
		attribute.Int("audio_size_bytes", len(req.AudioData)),
		attribute.String("attempting_language", language),
	)

	start := time.Now()

	// Validate inputs
	if c.subscriptionKey == "" || c.region == "" {
		err := errors.New("Azure Speech subscription key and region are required")
		span.RecordError(err)
		if telemetry.TelegramMessagesTotal != nil {
			telemetry.TelegramMessagesTotal.Add(ctx, 1,
				api.WithAttributes(
					attribute.String("type", "voice"),
					attribute.String("status", "stt_config_error"),
				),
			)
		}
		return nil, err
	}

	// Create speech config
	speechConfig, err := speech.NewSpeechConfigFromSubscription(c.subscriptionKey, c.region)
	if err != nil {
		span.RecordError(err)
		if telemetry.TelegramMessagesTotal != nil {
			telemetry.TelegramMessagesTotal.Add(ctx, 1,
				api.WithAttributes(
					attribute.String("type", "voice"),
					attribute.String("status", "stt_config_error"),
				),
			)
		}
		return nil, fmt.Errorf("failed to create speech config: %w", err)
	}
	defer speechConfig.Close()

	// Set the specified language for recognition
	err = speechConfig.SetSpeechRecognitionLanguage(language)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to set recognition language: %w", err)
	}

	// Create compressed audio format for OGG OPUS (Telegram voice messages)
	format, err := audio.GetCompressedFormat(audio.OGGOPUS)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get OGG OPUS audio format: %w", err)
	}
	defer format.Close()

	// Create push audio stream for compressed format
	stream, err := audio.CreatePushAudioInputStreamFromFormat(format)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to create audio stream: %w", err)
	}
	defer stream.Close()

	// Write audio data to stream
	stream.Write(req.AudioData)
	stream.CloseStream()

	// Create audio config from stream
	audioConfig, err := audio.NewAudioConfigFromStreamInput(stream)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to create audio config: %w", err)
	}
	defer audioConfig.Close()

	// Create speech recognizer
	recognizer, err := speech.NewSpeechRecognizerFromConfig(speechConfig, audioConfig)
	if err != nil {
		span.RecordError(err)
		if telemetry.TelegramMessagesTotal != nil {
			telemetry.TelegramMessagesTotal.Add(ctx, 1,
				api.WithAttributes(
					attribute.String("type", "voice"),
					attribute.String("status", "stt_recognizer_error"),
				),
			)
		}
		return nil, fmt.Errorf("failed to create speech recognizer: %w", err)
	}
	defer recognizer.Close()

	// Perform recognition
	resultChan := recognizer.RecognizeOnceAsync()
	outcome := <-resultChan

	// Record duration
	duration := time.Since(start)
	span.SetAttributes(attribute.Float64("stt_request_duration_ms", float64(duration.Milliseconds())))

	// Check if there was an error
	if outcome.Error != nil {
		span.RecordError(outcome.Error)
		if telemetry.TelegramMessagesTotal != nil {
			telemetry.TelegramMessagesTotal.Add(ctx, 1,
				api.WithAttributes(
					attribute.String("type", "voice"),
					attribute.String("status", "stt_recognition_error"),
				),
			)
		}
		return nil, fmt.Errorf("speech recognition error: %w", outcome.Error)
	}

	result := outcome.Result
	defer result.Close()

	// Check recognition result with detailed logging
	if result.Reason != common.RecognizedSpeech {
		var reasonStr string
		switch result.Reason {
		case common.NoMatch:
			reasonStr = "NoMatch - No speech could be recognized"
		case common.Canceled:
			reasonStr = "Canceled - Speech recognition was canceled"
		default:
			reasonStr = fmt.Sprintf("Unknown(%d) - Possible audio format or configuration issue", result.Reason)
		}

		// Enhanced debug logging
		slog.Warn("STT recognition failed",
			"language", language,
			"reason", reasonStr,
			"result_text", result.Text,
			"audio_size_bytes", len(req.AudioData),
			"duration_ms", duration.Milliseconds(),
			"session_id", req.SessionID,
			"component", "stt_processing")

		err := fmt.Errorf("speech recognition failed: %s", reasonStr)
		span.RecordError(err)
		span.SetAttributes(
			attribute.String("result_reason", reasonStr),
			attribute.String("result_text", result.Text),
			attribute.String("language", language),
			attribute.String("requested_language", req.Language),
		)

		if telemetry.TelegramMessagesTotal != nil {
			telemetry.TelegramMessagesTotal.Add(ctx, 1,
				api.WithAttributes(
					attribute.String("type", "voice"),
					attribute.String("status", "stt_recognition_failed"),
					attribute.Int("reason", int(result.Reason)),
					attribute.String("reason_text", reasonStr),
				),
			)
		}
		return nil, err
	}

	recognizedText := result.Text
	detectedLanguage := language // Use the language we set for recognition

	// Success debug logging
	slog.Debug("STT recognition successful",
		"language", language,
		"text", recognizedText,
		"audio_size_bytes", len(req.AudioData),
		"duration_ms", duration.Milliseconds(),
		"session_id", req.SessionID,
		"component", "stt_processing")

	// Build response
	sttResp := &STTResponse{
		SessionID:        req.SessionID,
		ChunkID:          req.ChunkID,
		RawText:          recognizedText,
		Translation:      recognizedText, // For now, no translation - just return the recognized text
		ProcessingTimeS:  duration.Seconds(),
		DetectedLanguage: detectedLanguage,
	}

	// Set response attributes
	span.SetAttributes(
		attribute.Int("raw_text_length", len(sttResp.RawText)),
		attribute.Int("translation_length", len(sttResp.Translation)),
		attribute.Float64("stt_processing_time_s", sttResp.ProcessingTimeS),
		attribute.String("detected_language", sttResp.DetectedLanguage),
	)

	// Record success metric
	if telemetry.TelegramMessagesTotal != nil {
		telemetry.TelegramMessagesTotal.Add(ctx, 1,
			api.WithAttributes(
				attribute.String("type", "voice"),
				attribute.String("status", "stt_success"),
				attribute.String("language", req.Language),
				attribute.String("detected_language", detectedLanguage),
			),
		)
	}

	return sttResp, nil
}
