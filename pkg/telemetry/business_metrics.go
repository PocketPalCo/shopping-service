package telemetry

import (
	api "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/metric"
	"log/slog"
)

// Business metrics for application-level monitoring
var (
	// Telegram Bot metrics
	TelegramMessagesTotal api.Int64Counter
	TelegramCommandsTotal api.Int64Counter
	TelegramUsersActive   api.Int64UpDownCounter
	TelegramErrorsTotal   api.Int64Counter

	// Shopping List metrics
	ShoppingListOperations api.Int64Counter
	ShoppingItemOperations api.Int64Counter
	ShoppingListsActive    api.Int64UpDownCounter

	// User & Family metrics
	UserOperationsTotal   api.Int64Counter
	FamilyOperationsTotal api.Int64Counter
	ActiveFamilies        api.Int64UpDownCounter
	ActiveUsers           api.Int64UpDownCounter

	// Error tracking
	ApplicationErrorsTotal api.Int64Counter
	DatabaseErrorsTotal    api.Int64Counter
)

// InitBusinessMetrics initializes all business-level metrics
func InitBusinessMetrics(provider *metric.MeterProvider) error {
	meter := provider.Meter("business")

	var err error

	// Telegram Bot Metrics
	TelegramMessagesTotal, err = meter.Int64Counter("telegram.messages.total",
		api.WithDescription("Total Telegram messages processed by type"))
	if err != nil {
		return err
	}

	TelegramCommandsTotal, err = meter.Int64Counter("telegram.commands.total",
		api.WithDescription("Total Telegram commands executed by command type"))
	if err != nil {
		return err
	}

	TelegramUsersActive, err = meter.Int64UpDownCounter("telegram.users.active",
		api.WithDescription("Number of active Telegram users"))
	if err != nil {
		return err
	}

	TelegramErrorsTotal, err = meter.Int64Counter("telegram.errors.total",
		api.WithDescription("Total Telegram bot errors by type"))
	if err != nil {
		return err
	}

	// Shopping List Metrics
	ShoppingListOperations, err = meter.Int64Counter("shopping.list.operations.total",
		api.WithDescription("Total shopping list operations by type (create, update, delete)"))
	if err != nil {
		return err
	}

	ShoppingItemOperations, err = meter.Int64Counter("shopping.item.operations.total",
		api.WithDescription("Total shopping item operations by type (add, remove, complete)"))
	if err != nil {
		return err
	}

	ShoppingListsActive, err = meter.Int64UpDownCounter("shopping.lists.active",
		api.WithDescription("Number of active shopping lists"))
	if err != nil {
		return err
	}

	// User & Family Metrics
	UserOperationsTotal, err = meter.Int64Counter("user.operations.total",
		api.WithDescription("Total user operations by type"))
	if err != nil {
		return err
	}

	FamilyOperationsTotal, err = meter.Int64Counter("family.operations.total",
		api.WithDescription("Total family operations by type"))
	if err != nil {
		return err
	}

	ActiveFamilies, err = meter.Int64UpDownCounter("families.active",
		api.WithDescription("Number of active families"))
	if err != nil {
		return err
	}

	ActiveUsers, err = meter.Int64UpDownCounter("users.active",
		api.WithDescription("Number of active users"))
	if err != nil {
		return err
	}

	// Error Metrics
	ApplicationErrorsTotal, err = meter.Int64Counter("application.errors.total",
		api.WithDescription("Total application errors by component and type"))
	if err != nil {
		return err
	}

	DatabaseErrorsTotal, err = meter.Int64Counter("database.errors.total",
		api.WithDescription("Total database errors by operation and type"))
	if err != nil {
		return err
	}

	slog.Info("Business metrics initialized successfully")
	return nil
}
