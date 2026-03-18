package config

import "github.com/spf13/viper"

// setDefaults registers every configuration key with a development-safe default.
// This serves two purposes:
//  1. Enables env-only bootstrap: Viper's AutomaticEnv only maps env vars to
//     keys it already knows about. Without defaults, env-only mode produces
//     zero-values because Viper's internal key registry is empty.
//  2. Self-documents expected keys and their types for operators.
//
// Production deployments MUST override critical keys (DSN, JWT secret, etc.)
// via environment variables or a mounted YAML file.
func setDefaults(v *viper.Viper) {
	setAppDefaults(v)
	setHTTPDefaults(v)
	setGRPCDefaults(v)
	setDBDefaults(v)
	setRedisDefaults(v)
	setCacheDefaults(v)
	setBrokerDefaults(v)
	setObservabilityDefaults(v)
	setResilienceDefaults(v)
	setOutboxDefaults(v)
	setAuthDefaults(v)
}

func setAppDefaults(v *viper.Viper) {
	v.SetDefault("app.name", "wolf-prime")
	v.SetDefault("app.env", EnvDevelopment)
	v.SetDefault("app.debug", false)
	v.SetDefault("app.shutdown_timeout", "15s")
	v.SetDefault("modules", map[string]bool{"iam": true})
}

func setHTTPDefaults(v *viper.Viper) {
	v.SetDefault("http.port", 8080)
	v.SetDefault("http.read_timeout", "10s")
	v.SetDefault("http.read_header_timeout", "5s")
	v.SetDefault("http.write_timeout", "15s")
	v.SetDefault("http.idle_timeout", "120s")
	v.SetDefault("http.max_header_bytes", 1048576)
	v.SetDefault("http.cors.allowed_origins", []string{"http://localhost:3000"})
	v.SetDefault("http.cors.allowed_methods", []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"})
	v.SetDefault("http.cors.allowed_headers", []string{"Authorization", "Content-Type", "X-Request-ID", "Idempotency-Key"})
	v.SetDefault("http.cors.allow_credentials", true)
	v.SetDefault("http.cors.max_age", 86400)
}

func setGRPCDefaults(v *viper.Viper) {
	v.SetDefault("grpc.port", 9090)
	v.SetDefault("grpc.max_recv_msg_size", 4194304)
	v.SetDefault("grpc.max_send_msg_size", 4194304)
	v.SetDefault("grpc.keepalive_time", "30s")
	v.SetDefault("grpc.keepalive_timeout", "10s")
}

func setDBDefaults(v *viper.Viper) {
	// DSN intentionally left empty — must be provided by env or YAML.
	v.SetDefault("db.write.dsn", "")
	v.SetDefault("db.write.max_open_conns", 25)
	v.SetDefault("db.write.max_idle_conns", 10)
	v.SetDefault("db.write.conn_max_lifetime", "5m")
	v.SetDefault("db.write.conn_max_idle_time", "1m")
	v.SetDefault("db.write.statement_timeout", 30000)
	v.SetDefault("db.write.idle_in_transaction_session_timeout", 60000)
	v.SetDefault("db.write.simple_protocol", false)

	v.SetDefault("db.read.dsn", "")
	v.SetDefault("db.read.max_open_conns", 50)
	v.SetDefault("db.read.max_idle_conns", 25)
	v.SetDefault("db.read.conn_max_lifetime", "5m")
	v.SetDefault("db.read.conn_max_idle_time", "1m")
	v.SetDefault("db.read.statement_timeout", 30000)
	v.SetDefault("db.read.idle_in_transaction_session_timeout", 60000)
	v.SetDefault("db.read.simple_protocol", false)
}

func setRedisDefaults(v *viper.Viper) {
	v.SetDefault("redis.addr", "localhost:6379")
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 0)
	v.SetDefault("redis.pool_size", 20)
	v.SetDefault("redis.min_idle_conns", 5)
	v.SetDefault("redis.read_timeout", "3s")
	v.SetDefault("redis.write_timeout", "3s")
	v.SetDefault("redis.dial_timeout", "5s")
}

func setCacheDefaults(v *viper.Viper) {
	v.SetDefault("cache.driver", CacheDriverRedis)
	v.SetDefault("cache.local.enabled", true)
	v.SetDefault("cache.local.size", 10000)
	v.SetDefault("cache.local.ttl", "10s")
}

func setBrokerDefaults(v *viper.Viper) {
	v.SetDefault("broker.driver", BrokerInProcess)
	v.SetDefault("kafka.brokers", []string{"localhost:9092"})
	v.SetDefault("kafka.consumer_group", "wolf-prime")
	v.SetDefault("rabbitmq.url", "amqp://guest:guest@localhost:5672/")
	v.SetDefault("rabbitmq.prefetch_count", 10)
	v.SetDefault("nats.url", "nats://localhost:4222")
	v.SetDefault("nats.stream", "wolf")
	v.SetDefault("nats.subjects", []string{"wolf.>"})
	v.SetDefault("nats.replicas", 1)
}

func setObservabilityDefaults(v *viper.Viper) {
	v.SetDefault("log.level", "debug")
	v.SetDefault("log.format", "json")
	v.SetDefault("otel.enabled", false)
	v.SetDefault("otel.exporter", "otlp")
	v.SetDefault("otel.endpoint", "localhost:4317")
	v.SetDefault("otel.sample_rate", 1.0)
	v.SetDefault("otel.insecure", true)
	v.SetDefault("metrics.enabled", true)
	v.SetDefault("metrics.port", 9091)
}

func setResilienceDefaults(v *viper.Viper) {
	v.SetDefault("cb.max_requests", 5)
	v.SetDefault("cb.interval", "10s")
	v.SetDefault("cb.timeout", "30s")
	v.SetDefault("rate_limit.rps", 1000)
	v.SetDefault("rate_limit.burst", 2000)
	v.SetDefault("load_shed.max_concurrent", 5000)
	v.SetDefault("security_headers.enabled", true)
	v.SetDefault("security_headers.hsts", true)
	v.SetDefault("security_headers.hsts_max_age", 31536000)
	v.SetDefault("security_headers.content_type_nosniff", true)
	v.SetDefault("security_headers.frame_deny", true)
	v.SetDefault("security_headers.xss_protection", true)
	v.SetDefault("security_headers.referrer_policy", "strict-origin-when-cross-origin")
}

func setOutboxDefaults(v *viper.Viper) {
	v.SetDefault("outbox.poll_interval", "500ms")
	v.SetDefault("outbox.batch_size", 100)
	v.SetDefault("outbox.max_retries", 5)
	v.SetDefault("outbox.retention", "72h")
	v.SetDefault("outbox.poll_timeout", "30s")
	v.SetDefault("outbox.health_threshold", "60s")
	v.SetDefault("outbox.notify_enabled", false)
}

func setAuthDefaults(v *viper.Viper) {
	v.SetDefault("jwt.signing_method", "HS256")
	v.SetDefault("jwt.secret_key", "")
	v.SetDefault("jwt.access_token_ttl", "15m")
	v.SetDefault("jwt.refresh_token_ttl", "168h")
	v.SetDefault("jwt.issuer", "wolf-prime")
	v.SetDefault("jwt.audience", []string{"wolf-prime-api"})
	v.SetDefault("bcrypt.cost", 12)
}
