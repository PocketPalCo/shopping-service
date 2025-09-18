package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/PocketPalCo/shopping-service/internal/core/stt"
	"github.com/PocketPalCo/shopping-service/internal/core/telegram/commands"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// InternalUser represents the bot's internal user structure (alias for BotInternalUser)
type InternalUser = BotInternalUser

// CoreMessageHandler handles all core message processing
type CoreMessageHandler struct {
	BaseHandler
	sttService              *stt.Client
	commandRegistry         *commands.CommandRegistry
	stateManager            *StateManager
	receiptsCallbackHandler *ReceiptsCallbackHandler
}

// NewCoreMessageHandler creates a new core message handler
func NewCoreMessageHandler(base BaseHandler, sttService *stt.Client, commandRegistry *commands.CommandRegistry, stateManager *StateManager, receiptsCallbackHandler *ReceiptsCallbackHandler) *CoreMessageHandler {
	return &CoreMessageHandler{
		BaseHandler:             base,
		sttService:              sttService,
		commandRegistry:         commandRegistry,
		stateManager:            stateManager,
		receiptsCallbackHandler: receiptsCallbackHandler,
	}
}

// HandleCommand processes incoming commands
func (h *CoreMessageHandler) HandleCommand(ctx context.Context, message *tgbotapi.Message, user *InternalUser) {
	parts := strings.Fields(message.Text)
	if len(parts) == 0 {
		return
	}

	commandText := parts[0]
	args := parts[1:]

	// Remove '@botname' suffix if present
	if strings.Contains(commandText, "@") {
		commandText = strings.Split(commandText, "@")[0]
	}

	// Remove the leading '/'
	if strings.HasPrefix(commandText, "/") {
		commandText = strings.TrimPrefix(commandText, "/")
	}

	h.logger.Info("Processing command",
		"command", commandText,
		"args_count", len(args),
		"user_id", user.TelegramID,
		"chat_id", message.Chat.ID,
		"is_authorized", user.IsAuthorized,
		"is_admin", user.IsAdmin)

	err := h.commandRegistry.ExecuteCommand(ctx, commandText, message.Chat.ID, user.User, args)
	if err != nil {
		h.logger.Error("Failed to execute command",
			"command", commandText,
			"error", err,
			"user_id", user.TelegramID,
			"chat_id", message.Chat.ID)
	}
}

// HandleTextMessage processes regular text messages
func (h *CoreMessageHandler) HandleTextMessage(ctx context.Context, message *tgbotapi.Message, user *InternalUser) {
	if !user.IsAuthorized {
		h.logger.Debug("Ignoring text message from unauthorized user",
			"user_id", user.TelegramID,
			"message", message.Text)
		return
	}

	h.logger.Info("Processing text message",
		"user_id", user.TelegramID,
		"chat_id", message.Chat.ID,
		"message_length", len(message.Text))

	// Check for user states first
	if familyIDStr, hasState := h.stateManager.GetUserState(user.TelegramID, "creating_list_for_family"); hasState {
		messageHandler := NewMessageHandler(h.BaseHandler, h.stateManager)
		messageHandler.HandleShoppingListNameInput(ctx, message, user.User, familyIDStr)
		h.stateManager.ClearUserState(user.TelegramID, "creating_list_for_family")
		return
	}

	if listIDStr, hasState := h.stateManager.GetUserState(user.TelegramID, "adding_item_to_list"); hasState {
		messageHandler := NewMessageHandler(h.BaseHandler, h.stateManager)
		messageHandler.HandleAddItemInput(ctx, message, user.User, listIDStr)
		h.stateManager.ClearUserState(user.TelegramID, "adding_item_to_list")
		return
	}

	if _, hasState := h.stateManager.GetUserState(user.TelegramID, "creating_custom_productlist"); hasState {
		// Need to get product list handler from somewhere
		h.logger.Warn("Product list handler not available in core message handler", "user_id", user.TelegramID)
		h.stateManager.ClearUserState(user.TelegramID, "creating_custom_productlist")
		return
	}

	// Default message handling
	messageHandler := NewMessageHandler(h.BaseHandler, h.stateManager)
	messageHandler.HandleAuthorizedMessage(ctx, message, user.User)
}

// HandleAudioMessage processes voice messages
func (h *CoreMessageHandler) HandleAudioMessage(ctx context.Context, message *tgbotapi.Message, user *InternalUser) {
	// Add panic recovery to prevent crashing the bot
	defer func() {
		if r := recover(); r != nil {
			h.logger.Error("❌ Panic in HandleAudioMessage",
				"panic", r,
				"user_id", user.TelegramID,
				"message_id", message.MessageID)
		}
	}()
	if !user.IsAuthorized {
		h.logger.Debug("Ignoring audio message from unauthorized user",
			"user_id", user.TelegramID,
			"chat_id", message.Chat.ID,
			"message_id", message.MessageID)
		return
	}

	// Get file info and comprehensive audio details
	var fileID string
	var audioType string
	var duration int
	var fileSize int
	var mimeType string
	var fileName string

	if message.Voice != nil {
		fileID = message.Voice.FileID
		audioType = "voice"
		duration = message.Voice.Duration
		fileSize = message.Voice.FileSize
		mimeType = message.Voice.MimeType
		fileName = fmt.Sprintf("voice_%d.ogg", message.MessageID)

		h.logger.Info("🎤 Audio message detected - Voice Message",
			"user_id", user.TelegramID,
			"telegram_id", user.TelegramID,
			"username", message.From.UserName,
			"first_name", message.From.FirstName,
			"last_name", message.From.LastName,
			"chat_id", message.Chat.ID,
			"message_id", message.MessageID,
			"audio_type", audioType,
			"file_id", fileID,
			"duration_seconds", duration,
			"file_size_bytes", fileSize,
			"mime_type", mimeType,
			"file_name", fileName,
			"user_locale", user.User.Locale,
			"is_authorized", user.IsAuthorized,
			"is_admin", user.IsAdmin,
			"timestamp", message.Date)
	} else if message.Audio != nil {
		fileID = message.Audio.FileID
		audioType = "audio"
		duration = message.Audio.Duration
		fileSize = message.Audio.FileSize
		mimeType = message.Audio.MimeType
		fileName = message.Audio.FileName
		if fileName == "" {
			fileName = fmt.Sprintf("audio_%d.mp3", message.MessageID)
		}

		h.logger.Info("🎤 Audio message detected - Audio File",
			"user_id", user.TelegramID,
			"telegram_id", user.TelegramID,
			"username", message.From.UserName,
			"first_name", message.From.FirstName,
			"last_name", message.From.LastName,
			"chat_id", message.Chat.ID,
			"message_id", message.MessageID,
			"audio_type", audioType,
			"file_id", fileID,
			"duration_seconds", duration,
			"file_size_bytes", fileSize,
			"mime_type", mimeType,
			"file_name", fileName,
			"title", message.Audio.Title,
			"performer", message.Audio.Performer,
			"user_locale", user.User.Locale,
			"is_authorized", user.IsAuthorized,
			"is_admin", user.IsAdmin,
			"timestamp", message.Date)
	} else {
		h.logger.Warn("Audio message without voice or audio data",
			"user_id", user.TelegramID,
			"chat_id", message.Chat.ID,
			"message_id", message.MessageID)
		return
	}

	h.logger.Info("🔄 Starting audio file processing",
		"user_id", user.TelegramID,
		"audio_type", audioType,
		"file_id", fileID,
		"step", "get_file_info")

	file, err := h.bot.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		h.logger.Error("❌ Failed to get file info",
			"error", err,
			"file_id", fileID,
			"user_id", user.TelegramID,
			"audio_type", audioType)
		return
	}

	h.logger.Info("✅ File info retrieved",
		"user_id", user.TelegramID,
		"file_id", fileID,
		"file_path", file.FilePath,
		"file_size_telegram", file.FileSize,
		"step", "file_info_success")

	// Download the file
	fileURL := file.Link(h.bot.Token)
	h.logger.Info("🔄 Downloading audio file",
		"user_id", user.TelegramID,
		"file_url", fileURL,
		"step", "download_start")

	resp, err := http.Get(fileURL)
	if err != nil {
		h.logger.Error("❌ Failed to download audio file",
			"error", err,
			"url", fileURL,
			"user_id", user.TelegramID,
			"audio_type", audioType)
		return
	}
	defer resp.Body.Close()

	h.logger.Info("📥 Download response received",
		"user_id", user.TelegramID,
		"status_code", resp.StatusCode,
		"content_type", resp.Header.Get("Content-Type"),
		"content_length", resp.Header.Get("Content-Length"),
		"step", "download_response")

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		h.logger.Error("❌ Failed to read audio data",
			"error", err,
			"user_id", user.TelegramID,
			"audio_type", audioType)
		return
	}

	h.logger.Info("✅ Audio file downloaded successfully",
		"user_id", user.TelegramID,
		"downloaded_size_bytes", len(audioData),
		"expected_size_bytes", file.FileSize,
		"content_type", resp.Header.Get("Content-Type"),
		"file_name", fileName,
		"audio_type", audioType,
		"download_size_match", len(audioData) == file.FileSize,
		"step", "download_complete")

	// Convert audio to text using STT service
	if h.sttService != nil {
		h.logger.Info("🔄 Starting audio transcription",
			"user_id", user.TelegramID,
			"audio_type", audioType,
			"step", "stt_start")

		// Create STT request with automatic language detection
		sessionID := fmt.Sprintf("telegram_%d_%d", user.TelegramID, message.MessageID)

		// Use empty language to enable auto-detection in Azure Speech SDK
		// This allows users to speak in any supported language regardless of their locale
		sttReq := stt.STTRequest{
			SessionID:      sessionID,
			ChunkID:        1,
			Language:       "", // Empty for auto-detection
			TargetLanguage: "", // Empty for auto-detection
			AudioData:      audioData,
			Filename:       fileName,
		}

		h.logger.Info("🔄 Sending to STT service",
			"user_id", user.TelegramID,
			"session_id", sessionID,
			"audio_size_bytes", len(audioData),
			"filename", fileName,
			"user_locale", user.User.Locale,
			"language_detection", "auto",
			"step", "stt_request")

		// Create a timeout context for STT processing
		sttCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		sttResp, err := h.sttService.ProcessAudio(sttCtx, sttReq)
		if err != nil {
			h.logger.Error("❌ Failed to transcribe audio",
				"error", err,
				"user_id", user.TelegramID,
				"session_id", sessionID,
				"audio_type", audioType,
				"audio_size_bytes", len(audioData),
				"step", "stt_error")

			// Send error message to user
			errorMsg := "❌ Sorry, I couldn't understand your voice message. Please try again or send a text message."
			msg := tgbotapi.NewMessage(message.Chat.ID, errorMsg)
			if _, err := h.bot.Send(msg); err != nil {
				h.logger.Error("Failed to send transcription error message", "error", err)
			}
			return
		}

		if sttResp.RawText == "" {
			h.logger.Info("⚠️ Empty transcription received",
				"user_id", user.TelegramID,
				"session_id", sessionID,
				"audio_type", audioType,
				"audio_duration_seconds", duration,
				"audio_size_bytes", len(audioData),
				"step", "stt_empty")

			// Send message about empty transcription
			emptyMsg := "🎤 I received your voice message but couldn't detect any speech. Please try again."
			msg := tgbotapi.NewMessage(message.Chat.ID, emptyMsg)
			if _, err := h.bot.Send(msg); err != nil {
				h.logger.Error("Failed to send empty transcription message", "error", err)
			}
			return
		}

		h.logger.Info("✅ Successfully transcribed audio",
			"user_id", user.TelegramID,
			"session_id", sessionID,
			"audio_type", audioType,
			"audio_duration_seconds", duration,
			"audio_size_bytes", len(audioData),
			"transcription", sttResp.RawText,
			"transcription_length", len(sttResp.RawText),
			"translation", sttResp.Translation,
			"processing_time_seconds", sttResp.ProcessingTimeS,
			"whisper_detected_language", sttResp.DetectedLanguage,
			"step", "stt_success")

		// Use Whisper's language detection directly (more accurate for audio)
		detectedLanguage := sttResp.DetectedLanguage
		if detectedLanguage == "" {
			h.logger.Warn("⚠️ No language detected by Whisper, falling back to user locale",
				"user_id", user.TelegramID,
				"transcription", sttResp.RawText,
				"user_locale", user.User.Locale,
				"step", "language_fallback")
			detectedLanguage = user.User.Locale
		}

		h.logger.Info("✅ Using Whisper-detected language",
			"user_id", user.TelegramID,
			"transcription", sttResp.RawText,
			"whisper_detected_language", detectedLanguage,
			"user_locale", user.User.Locale,
			"language_match", detectedLanguage == user.User.Locale,
			"step", "language_detection_success")

		// Process the transcription as a text message with detected language context
		textMessage := *message
		textMessage.Text = sttResp.RawText

		// Store detected language in a way that can be accessed by downstream processing
		// We'll modify the user context temporarily for this processing
		originalLocale := user.User.Locale
		user.User.Locale = detectedLanguage

		h.logger.Info("🔄 Sending transcription to user",
			"user_id", user.TelegramID,
			"transcription", sttResp.RawText,
			"step", "send_transcription")

		// Create a message to show transcription
		transcriptionMsg := "🎤 " + sttResp.RawText
		msg := tgbotapi.NewMessage(message.Chat.ID, transcriptionMsg)
		if _, err := h.bot.Send(msg); err != nil {
			h.logger.Error("❌ Failed to send transcription message",
				"error", err,
				"user_id", user.TelegramID,
				"transcription", sttResp.RawText)
		} else {
			h.logger.Info("✅ Transcription sent to user",
				"user_id", user.TelegramID,
				"step", "transcription_sent")
		}

		h.logger.Info("🔄 Processing transcription as text message",
			"user_id", user.TelegramID,
			"transcription", sttResp.RawText,
			"whisper_detected_language", detectedLanguage,
			"original_locale", originalLocale,
			"step", "text_processing")

		// Process as text message with the detected language context
		h.HandleTextMessage(ctx, &textMessage, user)

		// Restore original user locale after processing
		user.User.Locale = originalLocale

		h.logger.Info("✅ Audio message processing completed",
			"user_id", user.TelegramID,
			"audio_type", audioType,
			"transcription", sttResp.RawText,
			"whisper_detected_language", detectedLanguage,
			"restored_locale", originalLocale,
			"step", "processing_complete")
	} else {
		h.logger.Warn("⚠️ STT service not available",
			"user_id", user.TelegramID,
			"audio_type", audioType,
			"step", "stt_unavailable")

		// Send message about STT service unavailability
		unavailableMsg := "🎤 Voice message processing is temporarily unavailable. Please send a text message instead."
		msg := tgbotapi.NewMessage(message.Chat.ID, unavailableMsg)
		if _, err := h.bot.Send(msg); err != nil {
			h.logger.Error("Failed to send STT unavailable message", "error", err)
		}
	}
}

// HandlePhotoMessage processes photo messages for receipt upload
func (h *CoreMessageHandler) HandlePhotoMessage(ctx context.Context, message *tgbotapi.Message, user *InternalUser) {
	if !user.IsAuthorized {
		h.logger.Debug("Ignoring photo message from unauthorized user",
			"user_id", user.TelegramID)
		return
	}

	h.logger.Info("Processing photo message",
		"user_id", user.TelegramID,
		"chat_id", message.Chat.ID,
		"photos_count", len(message.Photo))

	if len(message.Photo) == 0 {
		h.logger.Warn("Photo message with no photos",
			"user_id", user.TelegramID,
			"message_id", message.MessageID)
		return
	}

	// Get the largest photo (best quality)
	var largestPhoto *tgbotapi.PhotoSize
	for i := range message.Photo {
		photo := &message.Photo[i]
		if largestPhoto == nil || photo.FileSize > largestPhoto.FileSize {
			largestPhoto = photo
		}
	}

	h.logger.Info("Selected photo for receipt processing",
		"user_id", user.TelegramID,
		"file_id", largestPhoto.FileID,
		"width", largestPhoto.Width,
		"height", largestPhoto.Height,
		"file_size", largestPhoto.FileSize)

	// Use the receipts callback handler for photo processing
	err := h.receiptsCallbackHandler.HandleReceiptPhoto(ctx, message.Chat.ID, message.MessageID, user.User, largestPhoto)
	if err != nil {
		h.logger.Error("Failed to handle receipt photo",
			"error", err,
			"user_id", user.TelegramID,
			"file_id", largestPhoto.FileID)
	}
}

// HandleDocumentMessage processes document messages for receipt upload
func (h *CoreMessageHandler) HandleDocumentMessage(ctx context.Context, message *tgbotapi.Message, user *InternalUser) {
	if !user.IsAuthorized {
		h.logger.Debug("Ignoring document message from unauthorized user",
			"user_id", user.TelegramID)
		return
	}

	if message.Document == nil {
		h.logger.Warn("Document message with no document",
			"user_id", user.TelegramID,
			"message_id", message.MessageID)
		return
	}

	h.logger.Info("Processing document message",
		"user_id", user.TelegramID,
		"chat_id", message.Chat.ID,
		"document_file_id", message.Document.FileID,
		"document_size", message.Document.FileSize,
		"mime_type", message.Document.MimeType,
		"file_name", message.Document.FileName)

	// Use the receipts callback handler for document processing
	err := h.receiptsCallbackHandler.HandleReceiptDocument(ctx, message.Chat.ID, message.MessageID, user.User, message.Document)
	if err != nil {
		h.logger.Error("Failed to handle receipt document",
			"error", err,
			"user_id", user.TelegramID,
			"file_id", message.Document.FileID)
	}
}

// HandleUpdate processes Telegram updates
func (h *CoreMessageHandler) HandleUpdate(ctx context.Context, update tgbotapi.Update) {
	// This method would be called from the bot service to route updates
	if update.Message != nil {
		// Enhanced logging for audio message detection
		var messageTypes []string
		if update.Message.Text != "" {
			messageTypes = append(messageTypes, "text")
		}
		if update.Message.Voice != nil {
			messageTypes = append(messageTypes, "voice")
		}
		if update.Message.Audio != nil {
			messageTypes = append(messageTypes, "audio")
		}
		if update.Message.Photo != nil {
			messageTypes = append(messageTypes, "photo")
		}
		if update.Message.Document != nil {
			messageTypes = append(messageTypes, "document")
		}
		if update.Message.Video != nil {
			messageTypes = append(messageTypes, "video")
		}
		if update.Message.Sticker != nil {
			messageTypes = append(messageTypes, "sticker")
		}

		h.logger.Info("📨 Received Telegram message update",
			"message_id", update.Message.MessageID,
			"user_id", update.Message.From.ID,
			"username", update.Message.From.UserName,
			"first_name", update.Message.From.FirstName,
			"last_name", update.Message.From.LastName,
			"chat_id", update.Message.Chat.ID,
			"message_types", strings.Join(messageTypes, ","),
			"has_text", update.Message.Text != "",
			"has_voice", update.Message.Voice != nil,
			"has_audio", update.Message.Audio != nil,
			"timestamp", update.Message.Date)

		// Special logging for audio messages
		if update.Message.Voice != nil {
			h.logger.Info("🎤 AUDIO DETECTED - Voice Message",
				"message_id", update.Message.MessageID,
				"user_id", update.Message.From.ID,
				"username", update.Message.From.UserName,
				"first_name", update.Message.From.FirstName,
				"chat_id", update.Message.Chat.ID,
				"file_id", update.Message.Voice.FileID,
				"duration_seconds", update.Message.Voice.Duration,
				"file_size_bytes", update.Message.Voice.FileSize,
				"mime_type", update.Message.Voice.MimeType,
				"timestamp", update.Message.Date)
		}

		if update.Message.Audio != nil {
			h.logger.Info("🎤 AUDIO DETECTED - Audio File",
				"message_id", update.Message.MessageID,
				"user_id", update.Message.From.ID,
				"username", update.Message.From.UserName,
				"first_name", update.Message.From.FirstName,
				"chat_id", update.Message.Chat.ID,
				"file_id", update.Message.Audio.FileID,
				"duration_seconds", update.Message.Audio.Duration,
				"file_size_bytes", update.Message.Audio.FileSize,
				"mime_type", update.Message.Audio.MimeType,
				"title", update.Message.Audio.Title,
				"performer", update.Message.Audio.Performer,
				"file_name", update.Message.Audio.FileName,
				"timestamp", update.Message.Date)
		}
	} else if update.CallbackQuery != nil {
		h.logger.Debug("Received callback query",
			"callback_id", update.CallbackQuery.ID,
			"user_id", update.CallbackQuery.From.ID,
			"username", update.CallbackQuery.From.UserName,
			"data", update.CallbackQuery.Data)
	}
}
