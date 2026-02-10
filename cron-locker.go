package main

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

type PostgresLocker struct {
	DB *gorm.DB
}

func (l *PostgresLocker) Lock(ctx context.Context, key string) error {
	_, err := gorm.G[*CronLock](db).Where("key=?", key).First(ctx)

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("cannot obtain cron lock. key '%s' already in use", key)
	}

	newRecord := &CronLock{Key: key}

	if err = gorm.G[CronLock](db).Create(ctx, newRecord); err != nil {
		return fmt.Errorf("error obtaining cron lock for key '%s': %w", key, err)
	}

	return nil
}

func (l *PostgresLocker) Extend(ctx context.Context, key string) error {
	return nil
}

func (l *PostgresLocker) Unlock(ctx context.Context, key string) error {
	_, err := gorm.G[CronLock](db).
		Scopes(func(db *gorm.Statement) {
			db.Unscoped = true
		}).
		Where("key=?", key).
		Delete(ctx)

	return err
}
