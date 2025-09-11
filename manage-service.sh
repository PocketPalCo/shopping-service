#!/bin/bash

# Shopping Service Management Script
# Easy management of the shopping service Docker stack

set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
COMPOSE_FILE="$SCRIPT_DIR/docker-compose-production.yml"

# Function to show usage
show_usage() {
    echo -e "${BLUE}Shopping Service Management Script${NC}"
    echo ""
    echo -e "${YELLOW}Usage:${NC}"
    echo "  $0 [COMMAND]"
    echo ""
    echo -e "${YELLOW}Commands:${NC}"
    echo "  start         Start the shopping service stack"
    echo "  stop          Stop the shopping service stack"
    echo "  restart       Restart the shopping service stack"
    echo "  status        Show status of all services"
    echo "  logs          Show logs from all services"
    echo "  logs-app      Show logs from shopping service only"
    echo "  logs-db       Show logs from PostgreSQL only"
    echo "  logs-redis    Show logs from Redis only"
    echo "  build         Build the shopping service image"
    echo "  ps            Show running containers"
    echo "  shell-app     Connect to shopping service container shell"
    echo "  shell-db      Connect to PostgreSQL shell"
    echo "  shell-redis   Connect to Redis shell"
    echo "  clean         Stop and remove all containers and volumes"
    echo "  setup         Initial setup and start"
    echo "  systemctl     Manage systemd service"
    echo "  health        Check health of all services"
    echo ""
}

# Function to check if Docker is running
check_docker() {
    if ! docker info >/dev/null 2>&1; then
        echo -e "${RED}❌ Docker is not running${NC}"
        exit 1
    fi
}

# Function to start services
start_services() {
    echo -e "${YELLOW}🚀 Starting shopping service stack...${NC}"
    check_docker
    
    cd "$SCRIPT_DIR"
    docker compose -f "$COMPOSE_FILE" up -d
    
    echo -e "${GREEN}✅ Services started successfully${NC}"
    echo ""
    show_service_info
}

# Function to stop services
stop_services() {
    echo -e "${YELLOW}🛑 Stopping shopping service stack...${NC}"
    check_docker
    
    cd "$SCRIPT_DIR"
    docker compose -f "$COMPOSE_FILE" down
    
    echo -e "${GREEN}✅ Services stopped successfully${NC}"
}

# Function to restart services
restart_services() {
    echo -e "${YELLOW}🔄 Restarting shopping service stack...${NC}"
    check_docker
    
    cd "$SCRIPT_DIR"
    docker compose -f "$COMPOSE_FILE" restart
    
    echo -e "${GREEN}✅ Services restarted successfully${NC}"
    echo ""
    show_service_info
}

# Function to show status
show_status() {
    echo -e "${YELLOW}📊 Shopping service stack status:${NC}"
    check_docker
    
    cd "$SCRIPT_DIR"
    docker compose -f "$COMPOSE_FILE" ps
}

# Function to show logs
show_logs() {
    check_docker
    cd "$SCRIPT_DIR"
    
    case "${2:-all}" in
        "all")
            echo -e "${YELLOW}📋 Showing logs from all services...${NC}"
            docker compose -f "$COMPOSE_FILE" logs -f
            ;;
        "app")
            echo -e "${YELLOW}📋 Showing shopping service logs...${NC}"
            docker compose -f "$COMPOSE_FILE" logs -f shopping-service
            ;;
        "db")
            echo -e "${YELLOW}📋 Showing PostgreSQL logs...${NC}"
            docker compose -f "$COMPOSE_FILE" logs -f postgres-shopping
            ;;
        "redis")
            echo -e "${YELLOW}📋 Showing Redis logs...${NC}"
            docker compose -f "$COMPOSE_FILE" logs -f redis-shopping
            ;;
    esac
}

# Function to build image
build_image() {
    echo -e "${YELLOW}🔨 Building shopping service image...${NC}"
    check_docker
    
    cd "$SCRIPT_DIR"
    docker compose -f "$COMPOSE_FILE" build --no-cache shopping-service
    
    echo -e "${GREEN}✅ Image built successfully${NC}"
}

# Function to show running containers
show_containers() {
    echo -e "${YELLOW}📦 Running containers:${NC}"
    check_docker
    
    cd "$SCRIPT_DIR"
    docker compose -f "$COMPOSE_FILE" ps
}

# Function to connect to shells
connect_shell() {
    check_docker
    cd "$SCRIPT_DIR"
    
    case "${2:-app}" in
        "app")
            echo -e "${YELLOW}🐚 Connecting to shopping service shell...${NC}"
            docker compose -f "$COMPOSE_FILE" exec shopping-service sh
            ;;
        "db")
            echo -e "${YELLOW}🐚 Connecting to PostgreSQL shell...${NC}"
            docker compose -f "$COMPOSE_FILE" exec postgres-shopping psql -U postgres -d pocket-pal
            ;;
        "redis")
            echo -e "${YELLOW}🐚 Connecting to Redis shell...${NC}"
            docker compose -f "$COMPOSE_FILE" exec redis-shopping redis-cli -a redis
            ;;
    esac
}

# Function to clean everything
clean_all() {
    echo -e "${RED}🧹 Cleaning all containers and volumes...${NC}"
    read -p "Are you sure? This will remove all data! (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        check_docker
        cd "$SCRIPT_DIR"
        docker compose -f "$COMPOSE_FILE" down -v --remove-orphans
        docker system prune -f
        echo -e "${GREEN}✅ Cleanup completed${NC}"
    else
        echo -e "${YELLOW}Cleanup cancelled${NC}"
    fi
}

# Function for initial setup
initial_setup() {
    echo -e "${GREEN}🎯 Initial Shopping Service Setup${NC}"
    echo ""
    
    check_docker
    
    echo -e "${YELLOW}📋 Building and starting services...${NC}"
    cd "$SCRIPT_DIR"
    
    # Build and start
    docker compose -f "$COMPOSE_FILE" up -d --build
    
    # Wait for services to be healthy
    echo -e "${YELLOW}⏳ Waiting for services to be healthy...${NC}"
    sleep 30
    
    # Show status
    show_service_info
    
    echo -e "${GREEN}✅ Setup completed successfully!${NC}"
    echo ""
    echo -e "${YELLOW}🔗 Service URLs:${NC}"
    echo "  • Shopping Service: http://localhost:3009"
    echo "  • PostgreSQL: localhost:5433"
    echo "  • Redis: localhost:6380"
}

# Function to manage systemd service
manage_systemctl() {
    case "${2:-status}" in
        "enable")
            systemctl --user enable shopping-service.service
            echo -e "${GREEN}✅ Systemd service enabled${NC}"
            ;;
        "disable")
            systemctl --user disable shopping-service.service
            echo -e "${GREEN}✅ Systemd service disabled${NC}"
            ;;
        "start")
            systemctl --user start shopping-service.service
            echo -e "${GREEN}✅ Systemd service started${NC}"
            ;;
        "stop")
            systemctl --user stop shopping-service.service
            echo -e "${GREEN}✅ Systemd service stopped${NC}"
            ;;
        "restart")
            systemctl --user restart shopping-service.service
            echo -e "${GREEN}✅ Systemd service restarted${NC}"
            ;;
        "status")
            systemctl --user status shopping-service.service
            ;;
        "logs")
            journalctl --user -u shopping-service.service -f
            ;;
    esac
}

# Function to check health
check_health() {
    echo -e "${YELLOW}🏥 Checking service health...${NC}"
    check_docker
    
    cd "$SCRIPT_DIR"
    
    # Check containers
    echo -e "${BLUE}Container Status:${NC}"
    docker compose -f "$COMPOSE_FILE" ps
    
    echo ""
    
    # Check service endpoints
    echo -e "${BLUE}Service Health:${NC}"
    
    # Check shopping service
    if curl -s http://localhost:3009/health >/dev/null 2>&1; then
        echo -e "${GREEN}✅ Shopping Service (3009) - Healthy${NC}"
    else
        echo -e "${RED}❌ Shopping Service (3009) - Unhealthy${NC}"
    fi
    
    # Check PostgreSQL
    if nc -z localhost 5433 2>/dev/null; then
        echo -e "${GREEN}✅ PostgreSQL (5433) - Accessible${NC}"
    else
        echo -e "${RED}❌ PostgreSQL (5433) - Not accessible${NC}"
    fi
    
    # Check Redis
    if nc -z localhost 6380 2>/dev/null; then
        echo -e "${GREEN}✅ Redis (6380) - Accessible${NC}"
    else
        echo -e "${RED}❌ Redis (6380) - Not accessible${NC}"
    fi
}

# Function to show service info
show_service_info() {
    echo -e "${BLUE}🎯 Service Information:${NC}"
    echo "  • Shopping Service: http://localhost:3009"
    echo "  • PostgreSQL: localhost:5433 (user: postgres, db: pocket-pal)"
    echo "  • Redis: localhost:6380 (auth: redis)"
    echo ""
    echo -e "${BLUE}📊 Management:${NC}"
    echo "  • Systemd: systemctl --user [start|stop|restart|status] shopping-service.service"
    echo "  • Logs: $0 logs"
    echo "  • Health: $0 health"
}

# Main execution
case "${1:-help}" in
    "start")
        start_services
        ;;
    "stop")
        stop_services
        ;;
    "restart")
        restart_services
        ;;
    "status")
        show_status
        ;;
    "logs")
        show_logs "$@"
        ;;
    "logs-app")
        show_logs "logs" "app"
        ;;
    "logs-db")
        show_logs "logs" "db"
        ;;
    "logs-redis")
        show_logs "logs" "redis"
        ;;
    "build")
        build_image
        ;;
    "ps")
        show_containers
        ;;
    "shell-app")
        connect_shell "shell" "app"
        ;;
    "shell-db")
        connect_shell "shell" "db"
        ;;
    "shell-redis")
        connect_shell "shell" "redis"
        ;;
    "clean")
        clean_all
        ;;
    "setup")
        initial_setup
        ;;
    "systemctl")
        manage_systemctl "$@"
        ;;
    "health")
        check_health
        ;;
    "help"|*)
        show_usage
        ;;
esac