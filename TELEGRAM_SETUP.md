# Telegram Bot Setup Guide

This guide will help you set up the Telegram bot with user authorization for the PocketPal Shopping Service.

## Prerequisites

1. Go 1.24 or later
2. PostgreSQL database running (use `docker-compose up` to start)
3. Telegram account

## Step 1: Create a Telegram Bot

1. Open Telegram and search for `@BotFather`
2. Send `/newbot` command
3. Follow the prompts to choose a name and username for your bot
4. Save the bot token provided by BotFather

## Step 2: Get Your Telegram User ID (Admin Setup)

1. Search for `@userinfobot` on Telegram  
2. Send any message to get your Telegram user ID
3. Save this ID - you'll need it for admin configuration

## Step 3: Configure Environment Variables

1. Copy the example environment file:
   ```bash
   cp .env.example .env
   ```

2. Edit `.env` file with your configuration:
   ```bash
   # Set your bot token from BotFather
   SSV_TELEGRAM_BOT_TOKEN=your_actual_bot_token_here
   
   # Set your Telegram ID as admin (comma-separated for multiple admins)
   SSV_TELEGRAM_ADMINS=your_telegram_user_id
   
   # Optional: Enable debug mode
   SSV_TELEGRAM_DEBUG=true
   ```

## Step 4: Install Dependencies and Run Migrations

1. Install the Telegram bot dependency:
   ```bash
   go get github.com/go-telegram-bot-api/telegram-bot-api/v5
   ```

2. Install migration tools (if not already installed):
   ```bash
   make install-tools
   ```

3. Run database migrations:
   ```bash
   migrate -path migrations -database "postgresql://postgres:postgres@localhost:5432/pocket-pal?sslmode=disable" up
   ```

## Step 5: Start the Service

1. Start the database:
   ```bash
   docker-compose up -d
   ```

2. Run the service:
   ```bash
   make dev
   # OR
   go run cmd/main.go
   ```

## Step 6: Test the Bot

1. Find your bot on Telegram using the username you created
2. Send `/start` command
3. You should see a welcome message indicating you need authorization

## Bot Commands

### For All Users:
- `/start` - Welcome message and authorization status
- `/help` - Show available commands
- `/status` - Show your authorization status

### For Admins Only:
- `/authorize <telegram_id>` - Authorize a user
- `/revoke <telegram_id>` - Revoke user authorization  
- `/users` - List all authorized users

## User Authorization Flow

1. **New User**: When someone first messages the bot, they're automatically added to the database but not authorized
2. **Authorization Request**: Users can see their Telegram ID via `/status` command
3. **Admin Authorization**: Admins use `/authorize <telegram_id>` to grant access
4. **Authorized Access**: Once authorized, users can access all bot features

## Database Tables

The bot creates two main tables:

### users
- Stores Telegram user information
- Tracks authorization status
- Records who authorized each user and when

### user_sessions  
- Manages active user sessions
- Automatic session cleanup (7-day expiration)
- Tracks chat activity

## Security Features

1. **Authorization Required**: Only authorized users can access shopping features
2. **Admin-Only Commands**: Critical commands restricted to configured admins
3. **Session Management**: Automatic session cleanup and validation
4. **Database Integrity**: Proper foreign key relationships and constraints

## Troubleshooting

### Bot Not Responding
- Check bot token is correct
- Verify bot is not blocked by Telegram
- Check logs for connection errors

### Admin Commands Not Working
- Verify your Telegram ID is in `SSV_TELEGRAM_ADMINS`
- Check environment variable format (comma-separated, no spaces)
- Restart service after config changes

### Database Connection Issues
- Ensure PostgreSQL is running (`docker-compose up`)
- Check database credentials in `.env`
- Verify database exists and migrations have run

### Authorization Issues
- Use `/status` command to check authorization status
- Admin must use exact Telegram ID from `/status` command
- Check logs for authorization errors

## Environment Variables Reference

| Variable | Description | Example |
|----------|-------------|---------|
| `SSV_TELEGRAM_BOT_TOKEN` | Bot token from @BotFather | `1234567890:ABCdef...` |
| `SSV_TELEGRAM_DEBUG` | Enable debug logging | `true` or `false` |
| `SSV_TELEGRAM_ADMINS` | Admin Telegram IDs | `123456789,987654321` |

For more environment variables, see `.env.example`.