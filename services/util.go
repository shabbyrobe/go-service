package services

import (
	"context"
	"time"

	service "github.com/shabbyrobe/go-service"
)

func StartTimeout(timeout time.Duration, services ...*Service) error {
	return service.StartTimeout(timeout, Runner(), services...)
}

func HaltTimeout(timeout time.Duration, services ...*Service) error {
	return service.HaltTimeout(timeout, Runner(), services...)
}

func MustHalt(ctx context.Context, services ...*Service) {
	service.MustHalt(ctx, Runner(), services...)
}

func MustHaltTimeout(timeout time.Duration, services ...*Service) {
	service.MustHaltTimeout(timeout, Runner(), services...)
}

func ShutdownTimeout(timeout time.Duration) error {
	return service.ShutdownTimeout(timeout, Runner())
}

func MustShutdown(ctx context.Context) {
	service.MustShutdown(ctx, Runner())
}

func MustShutdownTimeout(timeout time.Duration) {
	service.MustShutdownTimeout(timeout, Runner())
}
