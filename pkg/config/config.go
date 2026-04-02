package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	App       AppConfig       `mapstructure:"app"`
	DB        DBConfig        `mapstructure:"db"`
	Migration MigrationConfig `mapstructure:"migration"`
	Redis     RedisConfig     `mapstructure:"redis"`
	Cache     CacheConfig     `mapstructure:"cache"`
	JWT       JWTConfig       `mapstructure:"jwt"`
	Log       LogConfig       `mapstructure:"log"`
	Tracing   TracingConfig   `mapstructure:"tracing"`
	RBAC      RBACConfig      `mapstructure:"rbac"`
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
	Worker    WorkerConfig    `mapstructure:"worker"`
	Storage   StorageConfig   `mapstructure:"storage"`
	MQ        MQConfig        `mapstructure:"mq"`
	Scheduler SchedulerConfig `mapstructure:"scheduler"`
}

type AppConfig struct {
	Name               string `mapstructure:"name"`
	Env                string `mapstructure:"env"`
	Host               string `mapstructure:"host"`
	Port               int    `mapstructure:"port"`
	ReadTimeoutSec     int    `mapstructure:"read_timeout_sec"`
	WriteTimeoutSec    int    `mapstructure:"write_timeout_sec"`
	RequestTimeoutSec  int    `mapstructure:"request_timeout_sec"`
	ShutdownTimeoutSec int    `mapstructure:"shutdown_timeout_sec"`
}

type LogConfig struct {
	Level    string `mapstructure:"level"`
	Encoding string `mapstructure:"encoding"`
}

type TracingConfig struct {
	Enabled     bool    `mapstructure:"enabled"`
	Endpoint    string  `mapstructure:"endpoint"`
	Insecure    bool    `mapstructure:"insecure"`
	SampleRatio float64 `mapstructure:"sample_ratio"`
	TimeoutSec  int     `mapstructure:"timeout_sec"`
}

type DBConfig struct {
	Host               string `mapstructure:"host"`
	Port               int    `mapstructure:"port"`
	User               string `mapstructure:"user"`
	Password           string `mapstructure:"password"`
	Name               string `mapstructure:"name"`
	SSLMode            string `mapstructure:"ssl_mode"`
	DialTimeoutSec     int    `mapstructure:"dial_timeout_sec"`
	MaxOpenConns       int    `mapstructure:"max_open_conns"`
	MaxIdleConns       int    `mapstructure:"max_idle_conns"`
	ConnMaxLifetimeSec int    `mapstructure:"conn_max_lifetime_sec"`
	ConnMaxIdleTimeSec int    `mapstructure:"conn_max_idle_time_sec"`
}

type RedisConfig struct {
	Addr            string `mapstructure:"addr"`
	Password        string `mapstructure:"password"`
	DB              int    `mapstructure:"db"`
	DialTimeoutSec  int    `mapstructure:"dial_timeout_sec"`
	ReadTimeoutSec  int    `mapstructure:"read_timeout_sec"`
	WriteTimeoutSec int    `mapstructure:"write_timeout_sec"`
}

type MigrationConfig struct {
	AutoRun             bool   `mapstructure:"auto_run"`
	Table               string `mapstructure:"table"`
	TimeoutSec          int    `mapstructure:"timeout_sec"`
	StatementTimeoutSec int    `mapstructure:"statement_timeout_sec"`
}

type CacheConfig struct {
	NullTTLSec   int `mapstructure:"null_ttl_sec"`
	TTLJitterSec int `mapstructure:"ttl_jitter_sec"`
}

type JWTConfig struct {
	Issuer                string `mapstructure:"issuer"`
	AccessTTLSec          int    `mapstructure:"access_ttl_sec"`
	RefreshTTLSec         int    `mapstructure:"refresh_ttl_sec"`
	AccessSecret          string `mapstructure:"access_secret"`
	RefreshSecret         string `mapstructure:"refresh_secret"`
	PreviousAccessSecret  string `mapstructure:"previous_access_secret"`
	PreviousRefreshSecret string `mapstructure:"previous_refresh_secret"`
}

type RBACConfig struct {
	Enabled             bool `mapstructure:"enabled"`
	AutoLoadIntervalSec int  `mapstructure:"autoload_interval_sec"`
}

type RateLimitConfig struct {
	Enabled   bool `mapstructure:"enabled"`
	RPS       int  `mapstructure:"rps"`
	Burst     int  `mapstructure:"burst"`
	WindowSec int  `mapstructure:"window_sec"`
}

type WorkerConfig struct {
	Workers         int `mapstructure:"workers"`
	QueueSize       int `mapstructure:"queue_size"`
	SubmitTimeoutMs int `mapstructure:"submit_timeout_ms"`
}

type StorageConfig struct {
	Driver          string `mapstructure:"driver"`
	LocalDir        string `mapstructure:"local_dir"`
	PublicBaseURL   string `mapstructure:"public_base_url"`
	SignedURLSecret string `mapstructure:"signed_url_secret"`
}

type MQConfig struct {
	Enabled            bool   `mapstructure:"enabled"`
	Driver             string `mapstructure:"driver"`
	URL                string `mapstructure:"url"`
	Exchange           string `mapstructure:"exchange"`
	PublishTimeoutMs   int    `mapstructure:"publish_timeout_ms"`
	OperationTimeoutMs int    `mapstructure:"operation_timeout_ms"`
	PollIntervalMs     int    `mapstructure:"poll_interval_ms"`
	BatchSize          int    `mapstructure:"batch_size"`
	MaxAttempts        int    `mapstructure:"max_attempts"`
	LockTimeoutSec     int    `mapstructure:"lock_timeout_sec"`
}

type SchedulerConfig struct {
	Enabled            bool   `mapstructure:"enabled"`
	Queue              string `mapstructure:"queue"`
	Concurrency        int    `mapstructure:"concurrency"`
	TaskTimeoutSec     int    `mapstructure:"task_timeout_sec"`
	LockTTLSec         int    `mapstructure:"lock_ttl_sec"`
	OutboxDispatchCron string `mapstructure:"outbox_dispatch_cron"`
}

func Load() (*Config, error) {
	return load((*Config).Validate)
}

func LoadForMigration() (*Config, error) {
	return load((*Config).ValidateForMigration)
}

func load(validate func(*Config) error) (*Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetConfigName("config")
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config"
	}
	v.AddConfigPath(configPath)

	v.SetEnvPrefix("APP")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read base config: %w", err)
	}

	env := os.Getenv("APP_ENV")
	if env == "" {
		env = v.GetString("app.env")
	}
	if env != "" {
		v.SetConfigName("config." + env)
		_ = v.MergeInConfig()
	}
	if err := rejectFileBackedSecrets(v); err != nil {
		return nil, err
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	applyRuntimeSecrets(&cfg)
	if err := validate(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("app.name", "gin-boilerplate")
	v.SetDefault("app.env", "dev")
	v.SetDefault("app.host", "0.0.0.0")
	v.SetDefault("app.port", 8080)
	v.SetDefault("app.read_timeout_sec", 10)
	v.SetDefault("app.write_timeout_sec", 10)
	v.SetDefault("app.request_timeout_sec", 30)
	v.SetDefault("app.shutdown_timeout_sec", 25)
	v.SetDefault("db.host", "127.0.0.1")
	v.SetDefault("db.port", 5432)
	v.SetDefault("db.user", "postgres")
	v.SetDefault("db.password", "")
	v.SetDefault("db.name", "gin_boilerplate")
	v.SetDefault("db.ssl_mode", "disable")
	v.SetDefault("db.dial_timeout_sec", 2)
	v.SetDefault("db.max_open_conns", 25)
	v.SetDefault("db.max_idle_conns", 12)
	v.SetDefault("db.conn_max_lifetime_sec", 300)
	v.SetDefault("db.conn_max_idle_time_sec", 180)
	v.SetDefault("migration.auto_run", false)
	v.SetDefault("migration.table", "schema_migrations")
	v.SetDefault("migration.timeout_sec", 30)
	v.SetDefault("migration.statement_timeout_sec", 5)
	v.SetDefault("redis.addr", "127.0.0.1:6379")
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 0)
	v.SetDefault("redis.dial_timeout_sec", 2)
	v.SetDefault("redis.read_timeout_sec", 2)
	v.SetDefault("redis.write_timeout_sec", 2)
	v.SetDefault("cache.null_ttl_sec", 30)
	v.SetDefault("cache.ttl_jitter_sec", 60)
	v.SetDefault("jwt.issuer", "gin-boilerplate")
	v.SetDefault("jwt.access_ttl_sec", 7200)
	v.SetDefault("jwt.refresh_ttl_sec", 604800)
	v.SetDefault("log.level", "info")
	v.SetDefault("log.encoding", "json")
	v.SetDefault("tracing.enabled", true)
	v.SetDefault("tracing.endpoint", "")
	v.SetDefault("tracing.insecure", false)
	v.SetDefault("tracing.sample_ratio", 1.0)
	v.SetDefault("tracing.timeout_sec", 5)
	v.SetDefault("rbac.enabled", true)
	v.SetDefault("rbac.autoload_interval_sec", 30)
	v.SetDefault("rate_limit.enabled", true)
	v.SetDefault("rate_limit.rps", 20)
	v.SetDefault("rate_limit.burst", 40)
	v.SetDefault("rate_limit.window_sec", 1)
	v.SetDefault("worker.workers", 4)
	v.SetDefault("worker.queue_size", 128)
	v.SetDefault("worker.submit_timeout_ms", 100)
	v.SetDefault("storage.driver", "local")
	v.SetDefault("storage.local_dir", "data/files")
	v.SetDefault("storage.public_base_url", "/files")
	v.SetDefault("mq.enabled", false)
	v.SetDefault("mq.driver", "rabbitmq")
	v.SetDefault("mq.url", "")
	v.SetDefault("mq.exchange", "app.events")
	v.SetDefault("mq.publish_timeout_ms", 2000)
	v.SetDefault("mq.operation_timeout_ms", 2000)
	v.SetDefault("mq.poll_interval_ms", 1000)
	v.SetDefault("mq.batch_size", 50)
	v.SetDefault("mq.max_attempts", 10)
	v.SetDefault("mq.lock_timeout_sec", 30)
	v.SetDefault("scheduler.enabled", false)
	v.SetDefault("scheduler.queue", "scheduler")
	v.SetDefault("scheduler.concurrency", 4)
	v.SetDefault("scheduler.task_timeout_sec", 30)
	v.SetDefault("scheduler.lock_ttl_sec", 30)
	v.SetDefault("scheduler.outbox_dispatch_cron", "@every 5s")
}

func (c *Config) Validate() error {
	if err := c.validateApp(); err != nil {
		return err
	}
	if err := c.validateDB(); err != nil {
		return err
	}
	if err := c.validateMigration(); err != nil {
		return err
	}
	if err := c.validateRedis(); err != nil {
		return err
	}
	if err := c.validateCache(); err != nil {
		return err
	}
	if err := c.validateJWT(); err != nil {
		return err
	}
	if err := c.validateLog(); err != nil {
		return err
	}
	if err := c.validateTracing(); err != nil {
		return err
	}
	if err := c.validateRBAC(); err != nil {
		return err
	}
	if err := c.validateRateLimit(); err != nil {
		return err
	}
	if err := c.validateWorker(); err != nil {
		return err
	}
	if err := c.validateStorage(); err != nil {
		return err
	}
	if err := c.validateMQ(); err != nil {
		return err
	}
	if err := c.validateScheduler(); err != nil {
		return err
	}
	return nil
}

func (c *Config) ValidateForMigration() error {
	if err := c.validateDB(); err != nil {
		return err
	}
	if err := c.validateMigration(); err != nil {
		return err
	}
	return nil
}

func (c *Config) validateApp() error {
	if strings.TrimSpace(c.App.Name) == "" {
		return fmt.Errorf("app.name is required")
	}
	if c.App.Port <= 0 || c.App.Port > 65535 {
		return fmt.Errorf("invalid app.port")
	}
	if c.App.ReadTimeoutSec <= 0 || c.App.WriteTimeoutSec <= 0 || c.App.ShutdownTimeoutSec <= 0 {
		return fmt.Errorf("invalid timeout config")
	}
	return nil
}

func (c *Config) validateDB() error {
	if c.DB.Host == "" {
		return fmt.Errorf("db.host is required")
	}
	if c.DB.Port <= 0 || c.DB.Port > 65535 {
		return fmt.Errorf("invalid db.port")
	}
	if c.DB.User == "" {
		return fmt.Errorf("db.user is required")
	}
	if c.DB.Name == "" {
		return fmt.Errorf("db.name is required")
	}
	if c.DB.SSLMode == "" {
		return fmt.Errorf("db.ssl_mode is required")
	}
	if c.DB.DialTimeoutSec <= 0 ||
		c.DB.MaxOpenConns <= 0 ||
		c.DB.MaxIdleConns <= 0 ||
		c.DB.ConnMaxLifetimeSec <= 0 ||
		c.DB.ConnMaxIdleTimeSec <= 0 {
		return fmt.Errorf("invalid db connection pool config")
	}
	return nil
}

func (c *Config) validateMigration() error {
	if c.Migration.Table == "" {
		return fmt.Errorf("migration.table is required")
	}
	if c.Migration.TimeoutSec <= 0 || c.Migration.StatementTimeoutSec <= 0 {
		return fmt.Errorf("invalid migration config")
	}
	return nil
}

func (c *Config) validateRedis() error {
	if c.Redis.Addr == "" {
		return fmt.Errorf("redis.addr is required")
	}
	if _, _, err := net.SplitHostPort(c.Redis.Addr); err != nil {
		return fmt.Errorf("invalid redis.addr: %w", err)
	}
	if c.Redis.DialTimeoutSec <= 0 || c.Redis.ReadTimeoutSec <= 0 || c.Redis.WriteTimeoutSec <= 0 {
		return fmt.Errorf("invalid redis timeout config")
	}
	return nil
}

func (c *Config) validateJWT() error {
	if c.JWT.Issuer == "" {
		return fmt.Errorf("jwt.issuer is required")
	}
	if c.JWT.AccessTTLSec <= 0 || c.JWT.RefreshTTLSec <= 0 {
		return fmt.Errorf("invalid jwt ttl config")
	}
	if c.JWT.AccessSecret == "" {
		return fmt.Errorf("jwt.access_secret is required")
	}
	if c.JWT.RefreshSecret == "" {
		return fmt.Errorf("jwt.refresh_secret is required")
	}
	return nil
}

func (c *Config) validateCache() error {
	if c.Cache.NullTTLSec <= 0 || c.Cache.TTLJitterSec < 0 {
		return fmt.Errorf("invalid cache config")
	}
	return nil
}

func (c *Config) validateLog() error {
	if c.Log.Level == "" {
		return fmt.Errorf("log.level is required")
	}
	if !strings.EqualFold(c.Log.Encoding, "json") {
		return fmt.Errorf("log.encoding must be json")
	}
	return nil
}

func (c *Config) validateTracing() error {
	if c.Tracing.TimeoutSec <= 0 {
		return fmt.Errorf("invalid tracing.timeout_sec")
	}
	if c.Tracing.SampleRatio < 0 || c.Tracing.SampleRatio > 1 {
		return fmt.Errorf("tracing.sample_ratio must be between 0 and 1")
	}
	return nil
}

func (c *Config) validateRBAC() error {
	if c.RBAC.Enabled && c.RBAC.AutoLoadIntervalSec <= 0 {
		return fmt.Errorf("invalid rbac.autoload_interval_sec")
	}
	return nil
}

func (c *Config) validateRateLimit() error {
	if c.RateLimit.Enabled && (c.RateLimit.WindowSec <= 0 || (c.RateLimit.RPS <= 0 && c.RateLimit.Burst <= 0)) {
		return fmt.Errorf("invalid rate_limit config")
	}
	return nil
}

func (c *Config) validateWorker() error {
	if c.Worker.Workers <= 0 || c.Worker.QueueSize <= 0 || c.Worker.SubmitTimeoutMs <= 0 {
		return fmt.Errorf("invalid worker config")
	}
	return nil
}

func (c *Config) validateStorage() error {
	switch strings.ToLower(strings.TrimSpace(c.Storage.Driver)) {
	case "local":
		if strings.TrimSpace(c.Storage.LocalDir) == "" {
			return fmt.Errorf("storage.local_dir is required")
		}
		if strings.TrimSpace(c.Storage.PublicBaseURL) == "" {
			return fmt.Errorf("storage.public_base_url is required")
		}
		return nil
	default:
		return fmt.Errorf("unsupported storage.driver")
	}
}

func (c *Config) validateMQ() error {
	if !c.MQ.Enabled {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(c.MQ.Driver)) {
	case "rabbitmq":
	default:
		return fmt.Errorf("unsupported mq.driver")
	}
	if strings.TrimSpace(c.MQ.URL) == "" {
		return fmt.Errorf("mq.url is required")
	}
	if strings.TrimSpace(c.MQ.Exchange) == "" {
		return fmt.Errorf("mq.exchange is required")
	}
	if c.MQ.PublishTimeoutMs <= 0 ||
		c.MQ.OperationTimeoutMs <= 0 ||
		c.MQ.PollIntervalMs <= 0 ||
		c.MQ.BatchSize <= 0 ||
		c.MQ.MaxAttempts <= 0 ||
		c.MQ.LockTimeoutSec <= 0 {
		return fmt.Errorf("invalid mq config")
	}
	return nil
}

func (c *Config) validateScheduler() error {
	if !c.Scheduler.Enabled {
		return nil
	}
	if strings.TrimSpace(c.Scheduler.Queue) == "" {
		return fmt.Errorf("scheduler.queue is required")
	}
	if c.Scheduler.Concurrency <= 0 || c.Scheduler.TaskTimeoutSec <= 0 || c.Scheduler.LockTTLSec <= 0 {
		return fmt.Errorf("invalid scheduler config")
	}
	if strings.TrimSpace(c.Scheduler.OutboxDispatchCron) == "" {
		return fmt.Errorf("scheduler.outbox_dispatch_cron is required")
	}
	return nil
}

func (c DBConfig) DSN() string {
	parts := []string{
		"host=" + c.Host,
		"port=" + strconv.Itoa(c.Port),
		"user=" + c.User,
		"dbname=" + c.Name,
		"sslmode=" + c.SSLMode,
		"connect_timeout=" + strconv.Itoa(c.DialTimeoutSec),
	}
	if c.Password != "" {
		parts = append(parts, "password="+c.Password)
	}
	return strings.Join(parts, " ")
}

func applyRuntimeSecrets(cfg *Config) {
	cfg.JWT.AccessSecret = strings.TrimSpace(os.Getenv("APP_JWT_ACCESS_SECRET"))
	cfg.JWT.RefreshSecret = strings.TrimSpace(os.Getenv("APP_JWT_REFRESH_SECRET"))
	cfg.JWT.PreviousAccessSecret = strings.TrimSpace(os.Getenv("APP_JWT_PREVIOUS_ACCESS_SECRET"))
	cfg.JWT.PreviousRefreshSecret = strings.TrimSpace(os.Getenv("APP_JWT_PREVIOUS_REFRESH_SECRET"))
	cfg.Storage.SignedURLSecret = strings.TrimSpace(os.Getenv("APP_STORAGE_SIGNED_URL_SECRET"))
}

func rejectFileBackedSecrets(v *viper.Viper) error {
	runtimeOnlyKeys := []string{
		"jwt.access_secret",
		"jwt.refresh_secret",
		"jwt.previous_access_secret",
		"jwt.previous_refresh_secret",
		"storage.signed_url_secret",
	}
	for _, key := range runtimeOnlyKeys {
		if v.InConfig(key) {
			return fmt.Errorf("%s must be provided via runtime environment, not config files", key)
		}
	}
	return nil
}
