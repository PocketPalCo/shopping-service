# Observability Setup for Grafana

This document explains how the shopping service exports logs, metrics, and traces to your observability stack for viewing in Grafana.

## 📊 **Observability Components**

### 🗂️ **Logs (OTLP)**
- **Export**: Logs are exported to OTLP endpoint for ingestion by your log aggregator
- **Format**: Structured JSON logs with consistent attributes
- **Local**: Also written to local files in `logs/shopping-service.log`
- **Attributes**: All logs include component, environment, service info

### 📈 **Metrics (Prometheus/OTLP)**
- **HTTP Metrics**: Request duration, status codes, endpoints
- **Database Metrics**: Connection pool stats, query performance
- **Runtime Metrics**: Go runtime statistics
- **Bot Metrics**: Message counts, user activity (via logs)

### 🔍 **Traces (Jaeger)**
- **HTTP Requests**: Full request/response traces
- **Database Queries**: Query execution tracing
- **Bot Operations**: Message processing traces

## ⚙️ **Configuration**

### Environment Variables
```bash
# OTLP Endpoint for logs and metrics
SSV_OTLP_ENDPOINT=localhost:4317

# Jaeger endpoint for traces
SSV_JAEGER_ENDPOINT=http://localhost:14268/api/traces

# Log format (json recommended for parsing)
SSV_LOG_FORMAT=json
SSV_LOG_LEVEL=info

# Service identification
SSV_ENVIRONMENT=production
```

### OTLP Configuration
The service exports to OTLP using gRPC with these settings:
- **Endpoint**: Configurable via `SSV_OTLP_ENDPOINT`
- **Insecure**: Uses insecure connections (configure TLS as needed)
- **Batch Processing**: Logs are batched for efficient export
- **Resource Attributes**: Service name, version, environment

## 📋 **Log Structure**

### Standard Attributes
All logs include these consistent attributes:
```json
{
  "time": "2024-01-01T12:00:00Z",
  "level": "INFO",
  "msg": "Message text",
  "service": "shopping-service",
  "version": "1.0.0",
  "environment": "production",
  "component": "http_handler|telegram_bot|database|server|main"
}
```

### Component-Specific Attributes

**HTTP Requests:**
```json
{
  "component": "http_handler",
  "endpoint": "/v1/list",
  "method": "GET",
  "status_code": 200,
  "duration_ms": 45.2
}
```

**Telegram Bot:**
```json
{
  "component": "telegram_bot",
  "user_id": 123456789,
  "chat_id": 987654321,
  "command": "help",
  "message_type": "command",
  "authorized": true
}
```

**Database:**
```json
{
  "component": "database",
  "query": "SELECT * FROM users",
  "duration_ms": 12.5,
  "rows_affected": 5
}
```

## 🎯 **Grafana Dashboard Setup**

### Log Queries (Loki)
```logql
# All service logs
{service="shopping-service"}

# Component-specific logs
{service="shopping-service", component="telegram_bot"}

# Error logs only
{service="shopping-service"} |= "level=ERROR"

# Bot commands
{service="shopping-service", component="telegram_bot"} |= "command"

# Database errors
{service="shopping-service", component="database"} |= "ERROR"
```

### Metric Queries (Prometheus)
```promql
# HTTP request rate
rate(http_requests_total{service="shopping-service"}[5m])

# HTTP request duration
http_request_duration_ms{service="shopping-service"}

# Database connection pool
pgxpool_total_conns{service="shopping-service"}

# Error rate
rate(http_requests_total{service="shopping-service", status_code=~"5.."}[5m])
```

### Trace Queries (Jaeger)
- Service: `shopping-service`
- Operations: `GET /v1/list`, `telegram_message`, `db_query`

## 🚀 **Key Observability Features**

### 🔍 **Structured Logging**
- **Consistent format** across all components
- **Searchable attributes** for filtering and aggregation
- **Context propagation** with request IDs
- **Performance metrics** embedded in logs

### 📊 **Comprehensive Metrics**
- **HTTP performance** with percentiles and error rates
- **Database health** with connection pool monitoring
- **Application runtime** with Go-specific metrics
- **Business metrics** via structured logs

### 🎯 **Distributed Tracing**
- **End-to-end request** tracking
- **Service dependencies** visualization
- **Performance bottleneck** identification
- **Error correlation** across components

### 🤖 **Bot Observability**
- **Message processing** metrics
- **User activity** tracking
- **Command usage** analytics
- **Authorization events** monitoring

## 🛠️ **Monitoring Best Practices**

### Alerting Rules
```yaml
# High error rate
- alert: HighErrorRate
  expr: rate(http_requests_total{status_code=~"5.."}[5m]) > 0.1

# Database connection issues
- alert: DatabaseConnectionLow
  expr: pgxpool_idle_conns < 2

# Service down
- alert: ServiceDown
  expr: up{job="shopping-service"} == 0
```

### Dashboard Panels
1. **Service Health**: Request rate, error rate, response time
2. **Database**: Connection pool, query performance, errors
3. **Bot Activity**: Message volume, command usage, user stats
4. **Infrastructure**: Memory, CPU, goroutines
5. **Business Metrics**: Active users, popular commands

## 🔧 **Troubleshooting**

### Common Issues
1. **No logs in Grafana**: Check OTLP endpoint configuration
2. **Missing metrics**: Verify Prometheus scraping configuration  
3. **No traces**: Confirm Jaeger endpoint connectivity
4. **High cardinality**: Review label usage in metrics

### Debug Commands
```bash
# Check log export
curl -X POST http://localhost:4317/v1/logs \
  -H "Content-Type: application/json" \
  -d '{"test": "connectivity"}'

# View local logs
tail -f logs/shopping-service.log | jq .

# Check metrics endpoint
curl http://localhost:3001/metrics
```

Your logs will now be visible in Grafana with rich context and searchable attributes! 🎉