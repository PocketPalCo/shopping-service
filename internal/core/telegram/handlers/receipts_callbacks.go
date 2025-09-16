package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/PocketPalCo/shopping-service/internal/core/receipts"
	"github.com/PocketPalCo/shopping-service/internal/core/users"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

// ReceiptsCallbackHandler handles receipts menu callbacks
type ReceiptsCallbackHandler struct {
	BaseHandler
	stateManager *StateManager
}

// NewReceiptsCallbackHandler creates a new receipts callback handler
func NewReceiptsCallbackHandler(base BaseHandler, stateManager *StateManager) *ReceiptsCallbackHandler {
	return &ReceiptsCallbackHandler{
		BaseHandler:  base,
		stateManager: stateManager,
	}
}

// HandleReceiptsCallback processes receipts menu button callbacks
func (h *ReceiptsCallbackHandler) HandleReceiptsCallback(ctx context.Context, callback *tgbotapi.CallbackQuery, parts []string, user *users.User) {
	if len(parts) < 2 {
		h.answerCallback(callback.ID, "❌ Invalid receipts action.")
		return
	}

	action := parts[1]

	h.logger.Info("Processing receipts callback",
		"action", action,
		"user_id", user.TelegramID,
		"callback_id", callback.ID)

	// Clear any existing receipts states
	if h.stateManager != nil {
		h.stateManager.ClearUserState(user.TelegramID, "awaiting_receipt_upload")
		h.stateManager.ClearUserState(user.TelegramID, "viewing_receipts")
	}

	switch action {
	case "upload":
		h.handleUploadReceipt(ctx, callback, user)
	case "view":
		h.handleViewReceipts(ctx, callback, user, parts)
	case "detail":
		h.handleReceiptDetail(ctx, callback, user, parts)
	case "taxes":
		h.handleTaxSummary(ctx, callback, user)
	case "stats":
		h.handleReceiptStats(ctx, callback, user)
	default:
		h.logger.Warn("Unknown receipts action", "action", action, "user_id", user.TelegramID)
		h.answerCallback(callback.ID, "❌ Unknown action.")
	}
}

// handleUploadReceipt handles the upload receipt action
func (h *ReceiptsCallbackHandler) handleUploadReceipt(ctx context.Context, callback *tgbotapi.CallbackQuery, user *users.User) {
	h.logger.Info("Handling upload receipt action", "user_id", user.TelegramID)

	// Set state to awaiting receipt upload and store message ID for editing
	if h.stateManager != nil {
		h.stateManager.SetUserState(user.TelegramID, "awaiting_receipt_upload", "true")
		h.stateManager.SetUserState(user.TelegramID, "upload_message_id", fmt.Sprintf("%d:%d", callback.Message.Chat.ID, callback.Message.MessageID))
	}

	// Render upload instruction message
	message, err := h.templateManager.RenderTemplate("upload_receipt_instruction", user.Locale, nil)
	if err != nil {
		h.logger.Error("Failed to render upload receipt instruction template", "error", err)
		message = "📸 Send me a photo of your receipt to upload and process it."
	}

	// Create keyboard with back button
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				h.templateManager.RenderButton("back", user.Locale),
				"receipts:menu",
			),
		),
	)

	// Update the message
	edit := tgbotapi.NewEditMessageText(callback.Message.Chat.ID, callback.Message.MessageID, message)
	edit.ReplyMarkup = &keyboard
	edit.ParseMode = "HTML"

	if _, err := h.bot.Send(edit); err != nil {
		h.logger.Error("Failed to update upload receipt message", "error", err)
	}

	h.answerCallback(callback.ID, "📸 Send me a photo of your receipt!")
}

// handleViewReceipts handles the view receipts action with pagination
func (h *ReceiptsCallbackHandler) handleViewReceipts(ctx context.Context, callback *tgbotapi.CallbackQuery, user *users.User, parts []string) {
	h.logger.Info("Handling view receipts action", "user_id", user.TelegramID)

	// Parse page number (defaults to 1)
	page := 1
	if len(parts) > 2 {
		if parsedPage, err := strconv.Atoi(parts[2]); err == nil && parsedPage > 0 {
			page = parsedPage
		}
	}

	// Get receipts with pagination (5 per page)
	limit := 5
	offset := (page - 1) * limit

	receipts, err := h.receiptsService.GetUserReceipts(ctx, user.ID, limit, offset)
	if err != nil {
		h.logger.Error("Failed to get user receipts", "error", err, "user_id", user.ID)
		// Use template system for error messages
		errorMsg, _ := h.templateManager.RenderTemplate("receipts_list_error", user.Locale, nil)
		if errorMsg == "" {
			errorMsg, _ = h.templateManager.RenderTemplate("receipts_list_error", "en", nil)
		}
		h.answerCallback(callback.ID, errorMsg)
		return
	}

	// Get total count for pagination
	totalCount, err := h.receiptsService.GetUserReceiptsCount(ctx, user.ID)
	if err != nil {
		h.logger.Error("Failed to get user receipts count", "error", err, "user_id", user.ID)
		totalCount = 0
	}

	totalPages := (totalCount + limit - 1) / limit
	if totalPages == 0 {
		totalPages = 1
	}

	var message string
	var keyboard *tgbotapi.InlineKeyboardMarkup

	if len(receipts) == 0 && page == 1 {
		// No receipts at all
		message = h.templateManager.RenderMessage("no_receipts_found", user.Locale)
		keyboard = &tgbotapi.InlineKeyboardMarkup{
			InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{
				{
					tgbotapi.NewInlineKeyboardButtonData(
						h.templateManager.RenderButton("back", user.Locale),
						"receipts:menu",
					),
				},
			},
		}
	} else {
		// Create receipts list message with pagination
		data := struct {
			Receipts   []interface{}
			Page       int
			Total      int
			TotalPages int
		}{
			Page:       page,
			Total:      totalCount,
			TotalPages: totalPages,
		}

		// Convert receipts to interface slice for template
		for _, receipt := range receipts {
			var totalAmount interface{}
			if receipt.TotalAmount != nil {
				totalAmount = *receipt.TotalAmount
			}

			data.Receipts = append(data.Receipts, map[string]interface{}{
				"ID":           receipt.ID.String(),
				"ShortID":      receipt.ID.String()[:8],
				"FileName":     receipt.FileName,
				"MerchantName": receipt.MerchantName,
				"TotalAmount":  totalAmount,
				"CreatedAt":    receipt.CreatedAt.Format("2006-01-02 15:04"),
				"Processed":    receipt.Processed,
			})
		}

		var err error
		message, err = h.templateManager.RenderTemplate("receipts_list", user.Locale, data)
		if err != nil {
			h.logger.Error("Failed to render receipts list template", "error", err)
			message = h.templateManager.RenderMessage("receipts_list_error", user.Locale)
		}

		// Create keyboard with individual receipt buttons
		var rows [][]tgbotapi.InlineKeyboardButton

		// Add individual receipt buttons (max 5 per page)
		for _, receipt := range receipts {
			buttonText := receipt.FileName
			if receipt.MerchantName != nil && *receipt.MerchantName != "" {
				buttonText = *receipt.MerchantName
			}
			if len(buttonText) > 25 {
				buttonText = buttonText[:22] + "..."
			}
			if receipt.TotalAmount != nil {
				buttonText += fmt.Sprintf(" - €%.2f", *receipt.TotalAmount)
			}

			rows = append(rows, []tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardButtonData(
					buttonText,
					fmt.Sprintf("receipts:detail:%s", receipt.ID.String()),
				),
			})
		}

		// Add pagination controls
		var paginationRow []tgbotapi.InlineKeyboardButton
		if page > 1 {
			paginationRow = append(paginationRow,
				tgbotapi.NewInlineKeyboardButtonData(
					h.templateManager.RenderButton("previous", user.Locale),
					fmt.Sprintf("receipts:view:%d", page-1)))
		}
		if page < totalPages {
			paginationRow = append(paginationRow,
				tgbotapi.NewInlineKeyboardButtonData(
					h.templateManager.RenderButton("next", user.Locale),
					fmt.Sprintf("receipts:view:%d", page+1)))
		}
		if len(paginationRow) > 0 {
			rows = append(rows, paginationRow)
		}

		// Add back button
		rows = append(rows, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(
				h.templateManager.RenderButton("back", user.Locale),
				"receipts:menu",
			),
		})

		keyboard = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: rows}
	}

	// Check if current message is a photo message (from receipt detail view)
	if callback.Message.Photo != nil && len(callback.Message.Photo) > 0 {
		// Delete the photo message and send a new text message
		deleteMsg := tgbotapi.NewDeleteMessage(callback.Message.Chat.ID, callback.Message.MessageID)
		if _, err := h.bot.Send(deleteMsg); err != nil {
			h.logger.Warn("Failed to delete photo message", "error", err)
		}

		// Send new text message with receipts list
		newMsg := tgbotapi.NewMessage(callback.Message.Chat.ID, message)
		newMsg.ReplyMarkup = keyboard
		newMsg.ParseMode = "HTML"

		if _, err := h.bot.Send(newMsg); err != nil {
			h.logger.Error("Failed to send receipts list message", "error", err)
		}
	} else {
		// Update the existing text message
		edit := tgbotapi.NewEditMessageText(callback.Message.Chat.ID, callback.Message.MessageID, message)
		edit.ReplyMarkup = keyboard
		edit.ParseMode = "HTML"

		if _, err := h.bot.Send(edit); err != nil {
			h.logger.Error("Failed to update view receipts message", "error", err)
		}
	}

	h.answerCallback(callback.ID, h.templateManager.RenderMessage("callback_showing_receipts", user.Locale))
}

// handleReceiptDetail handles showing detailed receipt information
func (h *ReceiptsCallbackHandler) handleReceiptDetail(ctx context.Context, callback *tgbotapi.CallbackQuery, user *users.User, parts []string) {
	h.logger.Info("Handling receipt detail action", "user_id", user.TelegramID)

	if len(parts) < 3 {
		h.answerCallback(callback.ID, h.templateManager.RenderMessage("callback_invalid_receipt_id", user.Locale))
		return
	}

	receiptIDStr := parts[2]

	// Parse receipt ID as UUID
	receiptID, err := uuid.Parse(receiptIDStr)
	if err != nil {
		h.logger.Error("Invalid receipt ID format", "error", err, "receipt_id", receiptIDStr)
		h.answerCallback(callback.ID, h.templateManager.RenderMessage("callback_invalid_receipt_id", user.Locale))
		return
	}

	// Get detailed receipt with items
	receiptWithItems, err := h.receiptsService.GetReceiptWithItems(ctx, receiptID, user.ID)
	if err != nil {
		h.logger.Error("Failed to get receipt details", "error", err, "receipt_id", receiptID, "user_id", user.ID)
		h.answerCallback(callback.ID, h.templateManager.RenderMessage("callback_receipt_not_found", user.Locale))
		return
	}

	// Prepare template data
	data := struct {
		*receipts.ReceiptWithItems
		TotalAmount  interface{}
		NetAmount    interface{}
		TotalTax     interface{}
		AIConfidence interface{}
		Items        []interface{}
	}{
		ReceiptWithItems: receiptWithItems,
	}

	// Handle pointer fields for template
	if receiptWithItems.Receipt.TotalAmount != nil {
		data.TotalAmount = *receiptWithItems.Receipt.TotalAmount
	}
	if receiptWithItems.Receipt.NetAmount != nil {
		data.NetAmount = *receiptWithItems.Receipt.NetAmount
	}
	if receiptWithItems.Receipt.TotalTax != nil {
		data.TotalTax = *receiptWithItems.Receipt.TotalTax
	}
	if receiptWithItems.Receipt.AIConfidence != nil {
		confidence := *receiptWithItems.Receipt.AIConfidence
		// If confidence is between 0 and 1, convert to percentage
		if confidence <= 1.0 {
			confidence = confidence * 100
		}
		data.AIConfidence = confidence
	}

	// Convert items to interface slice for template
	for _, item := range receiptWithItems.Items {
		var quantity, unitPrice interface{}
		if item.Quantity != nil {
			quantity = *item.Quantity
		}
		if item.UnitPrice != nil {
			unitPrice = *item.UnitPrice
		}

		data.Items = append(data.Items, map[string]interface{}{
			"OriginalDescription":  item.OriginalDescription,
			"LocalizedDescription": item.LocalizedDescription,
			"Quantity":             quantity,
			"UnitPrice":            unitPrice,
			"TotalPrice":           item.TotalPrice,
		})
	}

	// Render receipt detail template
	message, err := h.templateManager.RenderTemplate("receipt_detail", user.Locale, data)
	if err != nil {
		h.logger.Error("Failed to render receipt detail template", "error", err)
		message = h.templateManager.RenderMessage("error_displaying_receipt", user.Locale)
	}

	// Create keyboard with back to receipts list button
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				h.templateManager.RenderButton("back_to_list", user.Locale),
				"receipts:view:1"),
		),
	)

	// Check if content is too long for photo caption (1024 char limit)
	// If so, prioritize content over image
	if len(message) > 1024 {
		// Content is more important than image - send as text message
		deleteMsg := tgbotapi.NewDeleteMessage(callback.Message.Chat.ID, callback.Message.MessageID)
		if _, err := h.bot.Send(deleteMsg); err != nil {
			h.logger.Warn("Failed to delete original message", "error", err)
		}

		// Send full content as text message
		textMsg := tgbotapi.NewMessage(callback.Message.Chat.ID, message)
		textMsg.ParseMode = "HTML"
		textMsg.ReplyMarkup = &keyboard

		if _, err := h.bot.Send(textMsg); err != nil {
			h.logger.Error("Failed to send receipt text message", "error", err)
		}
	} else if receiptWithItems.Receipt.TelegramFileID != nil && *receiptWithItems.Receipt.TelegramFileID != "" {
		// Content fits in caption - send with image
		photo := tgbotapi.NewPhoto(callback.Message.Chat.ID, tgbotapi.FileID(*receiptWithItems.Receipt.TelegramFileID))
		photo.Caption = message
		photo.ParseMode = "HTML"
		photo.ReplyMarkup = &keyboard

		deleteMsg := tgbotapi.NewDeleteMessage(callback.Message.Chat.ID, callback.Message.MessageID)
		if _, err := h.bot.Send(deleteMsg); err != nil {
			h.logger.Warn("Failed to delete original message", "error", err)
		}

		if _, err := h.bot.Send(photo); err != nil {
			h.logger.Error("Failed to send receipt photo", "error", err)
			// Fallback to text message if photo fails
			fallbackMsg := tgbotapi.NewMessage(callback.Message.Chat.ID, message)
			fallbackMsg.ParseMode = "HTML"
			fallbackMsg.ReplyMarkup = &keyboard
			h.bot.Send(fallbackMsg)
		}
	} else {
		// No image available - update existing message
		edit := tgbotapi.NewEditMessageText(callback.Message.Chat.ID, callback.Message.MessageID, message)
		edit.ReplyMarkup = &keyboard
		edit.ParseMode = "HTML"

		if _, err := h.bot.Send(edit); err != nil {
			h.logger.Error("Failed to update receipt detail message", "error", err)
		}
	}

	h.answerCallback(callback.ID, h.templateManager.RenderMessage("callback_receipt_details", user.Locale))
}

func (h *ReceiptsCallbackHandler) handleTaxSummary(ctx context.Context, callback *tgbotapi.CallbackQuery, user *users.User) {
	h.logger.Info("Handling tax summary action", "user_id", user.TelegramID)

	// Feature placeholder: Tax calculation functionality will be implemented
	// This will analyze receipts to calculate tax summaries and deductions
	message := h.templateManager.RenderMessage("tax_summary_coming_soon", user.Locale)

	// Create keyboard with back button
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				h.templateManager.RenderButton("back", user.Locale),
				"receipts:menu",
			),
		),
	)

	// Update the message
	edit := tgbotapi.NewEditMessageText(callback.Message.Chat.ID, callback.Message.MessageID, message)
	edit.ReplyMarkup = &keyboard
	edit.ParseMode = "HTML"

	if _, err := h.bot.Send(edit); err != nil {
		h.logger.Error("Failed to update tax summary message", "error", err)
	}

	h.answerCallback(callback.ID, "💰 Tax summary")
}

// handleReceiptStats handles the receipt statistics action
func (h *ReceiptsCallbackHandler) handleReceiptStats(ctx context.Context, callback *tgbotapi.CallbackQuery, user *users.User) {
	h.logger.Info("Handling receipt stats action", "user_id", user.TelegramID)

	// Feature placeholder: Statistics calculation functionality will be implemented
	// This will provide analytics on spending patterns, frequent items, and expense trends
	message := h.templateManager.RenderMessage("receipt_stats_coming_soon", user.Locale)

	// Create keyboard with back button
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				h.templateManager.RenderButton("back", user.Locale),
				"receipts:menu",
			),
		),
	)

	// Update the message
	edit := tgbotapi.NewEditMessageText(callback.Message.Chat.ID, callback.Message.MessageID, message)
	edit.ReplyMarkup = &keyboard
	edit.ParseMode = "HTML"

	if _, err := h.bot.Send(edit); err != nil {
		h.logger.Error("Failed to update receipt stats message", "error", err)
	}

	h.answerCallback(callback.ID, "📊 Statistics")
}

// handleBackToMenu handles returning to the receipts menu
func (h *ReceiptsCallbackHandler) HandleBackToReceiptsMenu(ctx context.Context, callback *tgbotapi.CallbackQuery, user *users.User) {
	h.logger.Info("Handling back to receipts menu", "user_id", user.TelegramID)

	// Clear any receipts-related states
	if h.stateManager != nil {
		h.stateManager.ClearUserState(user.TelegramID, "awaiting_receipt_upload")
		h.stateManager.ClearUserState(user.TelegramID, "viewing_receipts")
	}

	// Render receipts menu
	data := struct {
		IsAuthorized bool
		UserName     string
	}{
		IsAuthorized: user.IsAuthorized,
		UserName:     user.FirstName,
	}

	message, err := h.templateManager.RenderTemplate("receipts_menu", user.Locale, data)
	if err != nil {
		h.logger.Error("Failed to render receipts menu template", "error", err)
		message = "🧾 Receipt Management\n\nChoose an option below:"
	}

	// Create receipts menu keyboard
	keyboard := h.createReceiptsMenuKeyboard(user.Locale)

	// Update the message
	edit := tgbotapi.NewEditMessageText(callback.Message.Chat.ID, callback.Message.MessageID, message)
	edit.ReplyMarkup = keyboard
	edit.ParseMode = "HTML"

	if _, err := h.bot.Send(edit); err != nil {
		h.logger.Error("Failed to update receipts menu message", "error", err)
	}

	h.answerCallback(callback.ID, "🧾 Receipts menu")
}

// createReceiptsMenuKeyboard creates the receipts menu inline keyboard
func (h *ReceiptsCallbackHandler) createReceiptsMenuKeyboard(locale string) *tgbotapi.InlineKeyboardMarkup {
	rows := [][]tgbotapi.InlineKeyboardButton{
		// First row: Upload Receipt
		{
			tgbotapi.NewInlineKeyboardButtonData(
				h.templateManager.RenderButton("upload_receipt", locale),
				"receipts:upload",
			),
		},
		// Second row: View Receipts
		{
			tgbotapi.NewInlineKeyboardButtonData(
				h.templateManager.RenderButton("view_receipts", locale),
				"receipts:view",
			),
		},
		// Third row: Tax Summary and Statistics
		{
			tgbotapi.NewInlineKeyboardButtonData(
				h.templateManager.RenderButton("tax_summary", locale),
				"receipts:taxes",
			),
			tgbotapi.NewInlineKeyboardButtonData(
				h.templateManager.RenderButton("receipt_stats", locale),
				"receipts:stats",
			),
		},
		// Fourth row: Main Menu
		{
			tgbotapi.NewInlineKeyboardButtonData(
				h.templateManager.RenderButton("main_menu", locale),
				"menu_start",
			),
		},
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	return &keyboard
}

// answerCallback answers the callback query
func (h *ReceiptsCallbackHandler) answerCallback(callbackID, text string) {
	callback := tgbotapi.NewCallback(callbackID, text)
	if _, err := h.bot.Request(callback); err != nil {
		h.logger.Error("Failed to answer callback", "error", err, "callback_id", callbackID)
	}
}

// HandleReceiptDocument processes an uploaded document (PDF) for receipt parsing
func (h *ReceiptsCallbackHandler) HandleReceiptDocument(ctx context.Context, chatID int64, documentMessageID int, user *users.User, document *tgbotapi.Document) error {
	h.logger.Info("Processing receipt document upload",
		"user_id", user.ID,
		"telegram_id", user.TelegramID,
		"chat_id", chatID,
		"document_file_id", document.FileID,
		"document_size", document.FileSize,
		"mime_type", document.MimeType,
		"file_name", document.FileName)

	// Check if user is in receipt upload state
	if h.stateManager != nil {
		if state, exists := h.stateManager.GetUserState(user.TelegramID, "awaiting_receipt_upload"); !exists || state == "" {
			h.logger.Debug("User not awaiting receipt upload, ignoring document",
				"user_id", user.TelegramID)
			return nil
		}
	}

	// Get the stored message ID to edit
	var messageID int
	if h.stateManager != nil {
		if msgIDStr, exists := h.stateManager.GetUserState(user.TelegramID, "upload_message_id"); exists && msgIDStr != "" {
			parts := strings.Split(msgIDStr, ":")
			if len(parts) == 2 {
				if parsedID, err := strconv.Atoi(parts[1]); err == nil {
					messageID = parsedID
				}
			}
		}
	}

	// Update message with processing status
	if messageID > 0 {
		processingMsg, err := h.templateManager.RenderTemplate("processing_receipt", user.Locale, nil)
		if err != nil {
			h.logger.Error("Failed to render processing receipt template", "error", err)
			processingMsg = "⏳ Processing your receipt... Please wait a moment."
		}
		h.editMessage(chatID, messageID, processingMsg, nil)
	}

	// Get file info from Telegram
	fileConfig := tgbotapi.FileConfig{FileID: document.FileID}
	file, err := h.bot.GetFile(fileConfig)
	if err != nil {
		h.logger.Error("Failed to get document file from Telegram", "error", err, "file_id", document.FileID)
		if messageID > 0 {
			errorMsg, err := h.templateManager.RenderTemplate("receipt_upload_failed", user.Locale, nil)
			if err != nil {
				h.logger.Error("Failed to render receipt upload failed template", "error", err)
				// Fall back to English template if localized version fails
				errorMsg, _ = h.templateManager.RenderTemplate("receipt_upload_failed", "en", nil)
			}
			h.editMessageWithReceiptsMenu(chatID, messageID, errorMsg, user.Locale)
		}
		return fmt.Errorf("failed to get file: %w", err)
	}

	// Download file data from Telegram
	fileURL := file.Link(h.bot.Token)
	fileData, contentType, err := h.downloadFile(fileURL)
	if err != nil {
		h.logger.Error("Failed to download document file from Telegram", "error", err, "file_url", fileURL)
		if messageID > 0 {
			errorMsg, err := h.templateManager.RenderTemplate("receipt_upload_failed", user.Locale, nil)
			if err != nil {
				h.logger.Error("Failed to render receipt upload failed template", "error", err)
				// Fall back to English template if localized version fails
				errorMsg, _ = h.templateManager.RenderTemplate("receipt_upload_failed", "en", nil)
			}
			h.editMessageWithReceiptsMenu(chatID, messageID, errorMsg, user.Locale)
		}
		return fmt.Errorf("failed to download file: %w", err)
	}

	// Determine file extension based on content type or filename
	fileName := document.FileName
	if fileName == "" {
		// Generate filename based on content type
		if contentType == "application/pdf" {
			fileName = fmt.Sprintf("receipt_%s.pdf", file.FileUniqueID)
		} else {
			fileName = fmt.Sprintf("receipt_%s.bin", file.FileUniqueID)
		}
	}

	// Create receipt record
	createReq := receipts.CreateReceiptRequest{
		UserID:         user.ID,
		FileName:       fileName,
		FileSize:       int64(document.FileSize),
		ContentType:    contentType,
		TelegramFileID: &document.FileID,
		FileData:       fileData,
	}

	receipt, err := h.receiptsService.CreateReceipt(ctx, createReq)
	if err != nil {
		h.logger.Error("Failed to create receipt record", "error", err, "user_id", user.ID)
		if messageID > 0 {
			errorMsg, err := h.templateManager.RenderTemplate("receipt_upload_failed", user.Locale, nil)
			if err != nil {
				h.logger.Error("Failed to render receipt upload failed template", "error", err)
				// Fall back to English template if localized version fails
				errorMsg, _ = h.templateManager.RenderTemplate("receipt_upload_failed", "en", nil)
			}
			h.editMessageWithReceiptsMenu(chatID, messageID, errorMsg, user.Locale)
		}
		return fmt.Errorf("failed to create receipt: %w", err)
	}

	h.logger.Info("Receipt document uploaded successfully",
		"receipt_id", receipt.ID,
		"user_id", user.ID,
		"file_url", receipt.FileURL)

	// Update with success message
	if messageID > 0 {
		successMsg, err := h.templateManager.RenderTemplate("receipt_uploaded_success", user.Locale, nil)
		if err != nil {
			h.logger.Error("Failed to render receipt uploaded success template", "error", err)
			// Fall back to English template if localized version fails
			successMsg, _ = h.templateManager.RenderTemplate("receipt_uploaded_success", "en", nil)
		}
		h.editMessage(chatID, messageID, successMsg, nil)
	}

	// Start processing receipt with AI in background (only if not already processed)
	go func() {
		bgCtx := context.Background() // Use background context for async processing

		// Delete the user's document message after processing
		defer func() {
			h.logger.Info("Deleting user's original document message",
				"user_id", user.ID,
				"document_message_id", documentMessageID)
			h.DeleteMessage(chatID, documentMessageID)
			h.logger.Info("Requested deletion of user's document message",
				"user_id", user.ID,
				"document_message_id", documentMessageID)
		}()

		// Check if receipt is already processed (optimized upload flow handles this)
		if receipt.Processed {
			h.logger.Info("Receipt already processed during upload, skipping additional processing",
				"receipt_id", receipt.ID,
				"user_id", user.ID)

			// Update with success message and return to receipts menu
			if messageID > 0 {
				successMsg, err := h.templateManager.RenderTemplate("receipt_processed_success", user.Locale, nil)
				if err != nil {
					h.logger.Error("Failed to render success template", "error", err)
					// Fall back to English template if localized version fails
					successMsg, _ = h.templateManager.RenderTemplate("receipt_processed_success", "en", nil)
				}

				h.editMessageWithReceiptsMenu(chatID, messageID, successMsg, user.Locale)
			}
			return
		}

		if err := h.receiptsService.ProcessReceipt(bgCtx, receipt.ID); err != nil {
			h.logger.Error("Failed to process receipt with AI",
				"error", err,
				"receipt_id", receipt.ID,
				"user_id", user.ID)

			// Update with processing error and return to receipts menu
			if messageID > 0 {
				errorMsg, err := h.templateManager.RenderTemplate("receipt_processing_failed", user.Locale, nil)
				if err != nil {
					h.logger.Error("Failed to render receipt processing failed template", "error", err)
					errorMsg = "❌ Sorry, I couldn't process your receipt. Please try uploading it again."
				}
				h.editMessageWithReceiptsMenu(chatID, messageID, errorMsg, user.Locale)
			}
			return
		}

		// Success case: Update with processing success and return to receipts menu
		if messageID > 0 {
			processedMsg, err := h.templateManager.RenderTemplate("receipt_processed_success", user.Locale, nil)
			if err != nil {
				h.logger.Error("Failed to render receipt processed success template", "error", err)
				// Fall back to English template if localized version fails
				processedMsg, _ = h.templateManager.RenderTemplate("receipt_processed_success", "en", nil)
			}
			h.editMessageWithReceiptsMenu(chatID, messageID, processedMsg, user.Locale)
		}

		h.logger.Info("Receipt processed successfully",
			"user_id", user.ID,
			"receipt_id", receipt.ID)

	}()

	// Clear the upload state
	if h.stateManager != nil {
		h.stateManager.ClearUserState(user.TelegramID, "awaiting_receipt_upload")
		h.stateManager.ClearUserState(user.TelegramID, "upload_message_id")
	}

	return nil
}

// HandleReceiptPhoto processes an uploaded photo for receipt parsing
func (h *ReceiptsCallbackHandler) HandleReceiptPhoto(ctx context.Context, chatID int64, photoMessageID int, user *users.User, photo *tgbotapi.PhotoSize) error {
	h.logger.Info("Processing receipt photo upload",
		"user_id", user.ID,
		"telegram_id", user.TelegramID,
		"chat_id", chatID,
		"photo_file_id", photo.FileID,
		"photo_size", photo.FileSize)

	// Check if user is in receipt upload state
	if h.stateManager != nil {
		if state, exists := h.stateManager.GetUserState(user.TelegramID, "awaiting_receipt_upload"); !exists || state == "" {
			h.logger.Debug("User not awaiting receipt upload, ignoring photo",
				"user_id", user.TelegramID)
			return nil
		}
	}

	// Get the stored message ID to edit
	var messageID int
	if h.stateManager != nil {
		if msgIDStr, exists := h.stateManager.GetUserState(user.TelegramID, "upload_message_id"); exists && msgIDStr != "" {
			parts := strings.Split(msgIDStr, ":")
			if len(parts) == 2 {
				if parsedID, err := strconv.Atoi(parts[1]); err == nil {
					messageID = parsedID
				}
			}
		}
	}

	// Update message with processing status
	if messageID > 0 {
		processingMsg, err := h.templateManager.RenderTemplate("processing_receipt", user.Locale, nil)
		if err != nil {
			h.logger.Error("Failed to render processing receipt template", "error", err)
			processingMsg = "⏳ Processing your receipt... Please wait a moment."
		}
		h.editMessage(chatID, messageID, processingMsg, nil)
	}

	// Get file info from Telegram
	fileConfig := tgbotapi.FileConfig{FileID: photo.FileID}
	file, err := h.bot.GetFile(fileConfig)
	if err != nil {
		h.logger.Error("Failed to get file from Telegram", "error", err, "file_id", photo.FileID)
		if messageID > 0 {
			errorMsg, err := h.templateManager.RenderTemplate("receipt_upload_failed", user.Locale, nil)
			if err != nil {
				h.logger.Error("Failed to render receipt upload failed template", "error", err)
				// Fall back to English template if localized version fails
				errorMsg, _ = h.templateManager.RenderTemplate("receipt_upload_failed", "en", nil)
			}
			h.editMessageWithReceiptsMenu(chatID, messageID, errorMsg, user.Locale)
		}
		return fmt.Errorf("failed to get file: %w", err)
	}

	// Download file data from Telegram
	fileURL := file.Link(h.bot.Token)
	fileData, contentType, err := h.downloadFile(fileURL)
	if err != nil {
		h.logger.Error("Failed to download file from Telegram", "error", err, "file_url", fileURL)
		if messageID > 0 {
			errorMsg, err := h.templateManager.RenderTemplate("receipt_upload_failed", user.Locale, nil)
			if err != nil {
				h.logger.Error("Failed to render receipt upload failed template", "error", err)
				// Fall back to English template if localized version fails
				errorMsg, _ = h.templateManager.RenderTemplate("receipt_upload_failed", "en", nil)
			}
			h.editMessageWithReceiptsMenu(chatID, messageID, errorMsg, user.Locale)
		}
		return fmt.Errorf("failed to download file: %w", err)
	}

	// Create receipt record
	createReq := receipts.CreateReceiptRequest{
		UserID:         user.ID,
		FileName:       fmt.Sprintf("receipt_%s.jpg", file.FileUniqueID),
		FileSize:       int64(photo.FileSize),
		ContentType:    contentType,
		TelegramFileID: &photo.FileID,
		FileData:       fileData,
	}

	receipt, err := h.receiptsService.CreateReceipt(ctx, createReq)
	if err != nil {
		h.logger.Error("Failed to create receipt record", "error", err, "user_id", user.ID)
		if messageID > 0 {
			errorMsg, err := h.templateManager.RenderTemplate("receipt_upload_failed", user.Locale, nil)
			if err != nil {
				h.logger.Error("Failed to render receipt upload failed template", "error", err)
				// Fall back to English template if localized version fails
				errorMsg, _ = h.templateManager.RenderTemplate("receipt_upload_failed", "en", nil)
			}
			h.editMessageWithReceiptsMenu(chatID, messageID, errorMsg, user.Locale)
		}
		return fmt.Errorf("failed to create receipt: %w", err)
	}

	h.logger.Info("Receipt uploaded successfully",
		"receipt_id", receipt.ID,
		"user_id", user.ID,
		"file_url", receipt.FileURL)

	// Update with success message
	if messageID > 0 {
		successMsg, err := h.templateManager.RenderTemplate("receipt_uploaded_success", user.Locale, nil)
		if err != nil {
			h.logger.Error("Failed to render receipt uploaded success template", "error", err)
			// Fall back to English template if localized version fails
			successMsg, _ = h.templateManager.RenderTemplate("receipt_uploaded_success", "en", nil)
		}
		h.editMessage(chatID, messageID, successMsg, nil)
	}

	// Start processing receipt with AI in background (only if not already processed)
	go func() {
		bgCtx := context.Background() // Use background context for async processing

		// Delete the user's photo message after processing
		defer func() {
			h.logger.Info("Deleting user's original photo message",
				"user_id", user.ID,
				"photo_message_id", photoMessageID)
			h.DeleteMessage(chatID, photoMessageID)
			h.logger.Info("Requested deletion of user's photo message",
				"user_id", user.ID,
				"photo_message_id", photoMessageID)
		}()

		// Check if receipt is already processed (optimized upload flow handles this)
		if receipt.Processed {
			h.logger.Info("Receipt already processed during upload, skipping additional processing",
				"receipt_id", receipt.ID,
				"user_id", user.ID)

			if messageID > 0 {
				successMsg, err := h.templateManager.RenderTemplate("receipt_processed_success", user.Locale, nil)
				if err != nil {
					h.logger.Error("Failed to render success template", "error", err)
					successMsg, _ = h.templateManager.RenderTemplate("receipt_processed_success", "en", nil)
				}

				h.editMessageWithReceiptsMenu(chatID, messageID, successMsg, user.Locale)
			}
			return
		}

		if err := h.receiptsService.ProcessReceipt(bgCtx, receipt.ID); err != nil {
			h.logger.Error("Failed to process receipt with AI",
				"error", err,
				"receipt_id", receipt.ID,
				"user_id", user.ID)

			if messageID > 0 {
				errorMsg, err := h.templateManager.RenderTemplate("receipt_processing_failed", user.Locale, nil)
				if err != nil {
					h.logger.Error("Failed to render receipt processing failed template", "error", err)
					errorMsg = "❌ Sorry, I couldn't process your receipt. Please try uploading it again."
				}
				h.editMessageWithReceiptsMenu(chatID, messageID, errorMsg, user.Locale)
			}
			return
		}

		// Success case: Update with processing success and return to receipts menu
		if messageID > 0 {
			processedMsg, err := h.templateManager.RenderTemplate("receipt_processed_success", user.Locale, nil)
			if err != nil {
				h.logger.Error("Failed to render receipt processed success template", "error", err)
				// Fall back to English template if localized version fails
				processedMsg, _ = h.templateManager.RenderTemplate("receipt_processed_success", "en", nil)
			}
			h.editMessageWithReceiptsMenu(chatID, messageID, processedMsg, user.Locale)
		}

		h.logger.Info("Receipt processed successfully",
			"user_id", user.ID,
			"receipt_id", receipt.ID)

	}()

	// Clear the upload state
	if h.stateManager != nil {
		h.stateManager.ClearUserState(user.TelegramID, "awaiting_receipt_upload")
		h.stateManager.ClearUserState(user.TelegramID, "upload_message_id")
	}

	return nil
}

// downloadFile downloads a file from URL and returns the data and content type
func (h *ReceiptsCallbackHandler) downloadFile(url string) ([]byte, string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, "", fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("failed to download file: status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read file data: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")

	// Telegram often returns application/octet-stream for all files
	// Detect the actual content type from the file data
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = h.detectContentType(data)
	}

	return data, contentType, nil
}

// detectContentType detects the content type from file data by examining file headers
func (h *ReceiptsCallbackHandler) detectContentType(data []byte) string {
	if len(data) == 0 {
		return "application/octet-stream"
	}

	// Check for common image formats
	if len(data) >= 4 {
		// JPEG: FF D8 FF
		if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
			return "image/jpeg"
		}
		// PNG: 89 50 4E 47
		if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
			return "image/png"
		}
		// GIF: 47 49 46 38
		if data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x38 {
			return "image/gif"
		}
		// WebP: 52 49 46 46 (RIFF) + WebP signature
		if len(data) >= 12 && data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 &&
			data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50 {
			return "image/webp"
		}
		// BMP: 42 4D
		if data[0] == 0x42 && data[1] == 0x4D {
			return "image/bmp"
		}
		// PDF: 25 50 44 46 (%PDF)
		if data[0] == 0x25 && data[1] == 0x50 && data[2] == 0x44 && data[3] == 0x46 {
			return "application/pdf"
		}
	}

	return "image/jpeg"
}

// editMessage edits a message with new text and optional keyboard
func (h *ReceiptsCallbackHandler) editMessage(chatID int64, messageID int, text string, keyboard *tgbotapi.InlineKeyboardMarkup) {
	edit := tgbotapi.NewEditMessageText(chatID, messageID, text)
	edit.ParseMode = "HTML"
	if keyboard != nil {
		edit.ReplyMarkup = keyboard
	}

	if _, err := h.bot.Send(edit); err != nil {
		h.logger.Error("Failed to edit message", "error", err, "chat_id", chatID, "message_id", messageID)
	}
}

// editMessageWithReceiptsMenu edits a message and adds the receipts menu keyboard
func (h *ReceiptsCallbackHandler) editMessageWithReceiptsMenu(chatID int64, messageID int, text string, locale string) {
	keyboard := h.createReceiptsMenuKeyboard(locale)
	h.editMessage(chatID, messageID, text, keyboard)
}
