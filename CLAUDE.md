# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go-based microservice called `shopping-service` that is part of the PocketPal application. It uses Fiber as the web framework, PostgreSQL for data storage, and includes comprehensive OpenTelemetry instrumentation for observability.

## Architecture

The project follows a clean architecture pattern with clear separation of concerns:

### Core Structure
- `cmd/main.go` - Minimal application entry point with graceful shutdown
- `config/` - Configuration management using Viper with environment variables and file support
- `internal/core/` - Business logic layer (domain services)
  - `users/` - User management business logic service
    - `service.go` - User CRUD operations, authorization, admin management
  - `families/` - Family management business logic service
    - `service.go` - Family CRUD operations, member management, access control
  - `shopping/` - Shopping list business logic service  
    - `service.go` - Shopping list and item management operations with family support
  - `telegram/` - Telegram interface layer (messaging only)
    - `service.go` - Bot lifecycle and dependency injection
    - `bot_service.go` - Message handling and template rendering
    - `templates.go` - Template management with localization
    - `templates/` - Localized message templates (en, uk, ru)
    - `models.go` - Telegram-specific data models
- `internal/infra/` - Infrastructure layer
  - `server/` - HTTP server with middleware and route management
  - `postgres/` - Database interface and connection management
- `pkg/telemetry/` - OpenTelemetry instrumentation with metrics and tracing
- `pkg/logger/` - Structured logging with slog
- `migrations/` - Database migration files

### Key Features
- **Clean Architecture** with dependency injection and separation of concerns
- **Domain Services** for users, families, and shopping list business logic
- **Family Management System** with admin-created families and member roles
- **Family-based Shopping Lists** with shared access and updates
- **Telegram Interface Layer** handles only messaging and templates
- **Multi-language Support** with 3 locales: English (en), Ukrainian (uk), Russian (ru)
- **Template-based Messaging** with embedded HTML templates
- **User Authorization System** with admin role management
- **Comprehensive Observability** with OpenTelemetry (Jaeger + OTLP)
- **Graceful Shutdown** handling with proper cleanup
- **UUID-based Models** consistent with PostgreSQL schema

The service uses a configuration-driven approach with extensive environment variable support (all prefixed with `SSV_`).

### Architectural Principles

**Separation of Concerns:**
- **Users Service** (`internal/core/users/`) handles all user-related business logic
- **Families Service** (`internal/core/families/`) handles family creation and member management
- **Shopping Service** (`internal/core/shopping/`) handles shopping list operations with family support
- **Telegram Layer** (`internal/core/telegram/`) handles only messaging and template rendering
- Business logic is completely separated from Telegram API interactions

**Localization Support:**
- **3 Supported Locales**: English (en), Ukrainian (uk), Russian (ru)
- **Automatic Detection**: User locale detected from Telegram language settings
- **Template-based**: All messages use localized HTML templates in `templates/{locale}/`
- **Fallback**: Defaults to English for unsupported locales
- **Database Storage**: User locale preferences stored and persisted

**Family Management System:**
- **Admin-only Creation**: Only authorized admins can create new families
- **Role-based Access**: Family creators become admins, added users become members
- **Shopping List Integration**: Families can create shared shopping lists
- **Member Management**: Add/remove users from families with proper authorization
- **Database Design**: Separate tables for families and family_members with UUID references
- **Telegram Commands**: `/families`, `/createfamily`, `/addfamilymember` with localized responses

## Development Commands

### Testing
- `make unit-test` - Run unit tests with coverage
- `make unit-test-ci` - Run unit tests for CI with JUnit output
- `make integration-test` - Run integration tests with integration tag

### Development
- `make dev` - Start development server with hot reload using Air
- `make mock` - Generate mocks using go generate
- `make vet` - Run comprehensive linting (go vet, gosec, govulncheck, staticcheck)

### Database
- `docker-compose up` - Start PostgreSQL database (default: postgres/postgres@localhost:5432/pocket-pal)

### Protobuf (if needed)
- `make generate-proto` - Generate Go code from protobuf definitions

### Tool Installation
- `make install-tools` - Install all development tools (security scanners, linters, migration tools, etc.)

## Key Dependencies

- **Web Framework**: Fiber v2 with comprehensive middleware
- **Database**: PostgreSQL with pgx/v5 driver and connection pooling
- **Caching**: Redis with go-redis/v8 client
- **Configuration**: Viper with environment variable support
- **Observability**: Full OpenTelemetry stack (Jaeger tracing + OTLP metrics)
- **Telegram**: go-telegram-bot-api/v5 for bot integration
- **Monitoring**: Prometheus metrics with /metrics endpoint
- **Logging**: Structured logging with samber/slog-fiber middleware
- **Development**: Air for hot reloading

## Configuration

The service uses environment variables prefixed with `SSV_` or a `.env` file. Key configurations include:

- **Database**: PostgreSQL connection settings
- **Redis**: Caching configuration  
- **Server**: HTTP server binding and timeouts
- **Telemetry**: OTLP and Jaeger endpoints
- **Telegram**: Bot token and admin user configuration
- **Rate limiting**: Request throttling settings

Default development services:
- Database: `postgresql://postgres:postgres@localhost:5432/pocket-pal`
- Redis: `redis://redis:redis@localhost:6379/0`

See `.env.example` for all available configuration options.

## Testing Notes

- Use `make unit-test` for standard testing
- Integration tests require the `integration` build tag
- No existing test files found - create tests following Go conventions
- Add to memory: Use localization for everithing
- to memizize all prompts should be in separate files