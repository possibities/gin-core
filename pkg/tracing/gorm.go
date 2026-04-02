package tracing

import (
	"time"

	pkglogger "github.com/possibities/gin-core/pkg/logger"
	"github.com/possibities/gin-core/pkg/metrics"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

const (
	gormSpanKey      = "otel:span"
	gormStartedAtKey = "otel:started_at"
)

func RegisterGORMCallbacks(db *gorm.DB, provider *Provider, registry *metrics.Registry) error {
	if err := db.Callback().Query().Before("gorm:query").Register("otel:before_query", beforeGORM(provider, "query")); err != nil {
		return err
	}
	if err := db.Callback().Query().After("gorm:query").Register("otel:after_query", afterGORM("query", registry)); err != nil {
		return err
	}
	if err := db.Callback().Create().Before("gorm:create").Register("otel:before_create", beforeGORM(provider, "create")); err != nil {
		return err
	}
	if err := db.Callback().Create().After("gorm:create").Register("otel:after_create", afterGORM("create", registry)); err != nil {
		return err
	}
	if err := db.Callback().Update().Before("gorm:update").Register("otel:before_update", beforeGORM(provider, "update")); err != nil {
		return err
	}
	if err := db.Callback().Update().After("gorm:update").Register("otel:after_update", afterGORM("update", registry)); err != nil {
		return err
	}
	if err := db.Callback().Delete().Before("gorm:delete").Register("otel:before_delete", beforeGORM(provider, "delete")); err != nil {
		return err
	}
	if err := db.Callback().Delete().After("gorm:delete").Register("otel:after_delete", afterGORM("delete", registry)); err != nil {
		return err
	}
	if err := db.Callback().Row().Before("gorm:row").Register("otel:before_row", beforeGORM(provider, "row")); err != nil {
		return err
	}
	if err := db.Callback().Row().After("gorm:row").Register("otel:after_row", afterGORM("row", registry)); err != nil {
		return err
	}
	if err := db.Callback().Raw().Before("gorm:raw").Register("otel:before_raw", beforeGORM(provider, "raw")); err != nil {
		return err
	}
	if err := db.Callback().Raw().After("gorm:raw").Register("otel:after_raw", afterGORM("raw", registry)); err != nil {
		return err
	}
	return nil
}

func beforeGORM(provider *Provider, operation string) func(*gorm.DB) {
	return func(tx *gorm.DB) {
		tracer := provider.Tracer("db.sql")
		ctx := tx.Statement.Context
		ctx, span := tracer.Start(ctx, "db."+operation, trace.WithSpanKind(trace.SpanKindClient))

		attrs := []attribute.KeyValue{
			attribute.String("db.system", "postgres"),
			attribute.String("db.operation", operation),
		}
		if table := tx.Statement.Table; table != "" {
			attrs = append(attrs, attribute.String("db.table", table))
		}
		span.SetAttributes(attrs...)

		tx.Statement.Context = pkglogger.WithRequestContext(ctx, pkglogger.FromContext(ctx), pkglogger.TraceIDFromContext(ctx))
		tx.InstanceSet(gormSpanKey, span)
		tx.InstanceSet(gormStartedAtKey, time.Now())
	}
}

func afterGORM(operation string, registry *metrics.Registry) func(*gorm.DB) {
	return func(tx *gorm.DB) {
		if registry != nil {
			startedAtValue, ok := tx.InstanceGet(gormStartedAtKey)
			if ok {
				if startedAt, ok := startedAtValue.(time.Time); ok {
					registry.ObserveDBQuery(operation, tx.Statement.Table, time.Since(startedAt))
				}
			}
		}

		value, ok := tx.InstanceGet(gormSpanKey)
		if !ok {
			return
		}
		span, ok := value.(trace.Span)
		if !ok {
			return
		}
		if tx.Error != nil {
			span.RecordError(tx.Error)
			span.SetStatus(codes.Error, tx.Error.Error())
		} else {
			span.SetStatus(codes.Ok, operation)
		}
		span.End()
	}
}
