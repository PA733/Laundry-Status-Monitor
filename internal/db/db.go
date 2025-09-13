package db

import (
	"fmt"
	"log"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"laundry-status-backend/config"
	"laundry-status-backend/internal/model"
)

// Init initializes the database connection and runs migrations.
func Init(cfg *config.DatabaseConfig) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetimeMinutes) * time.Minute)

	log.Println("Running database migrations...")
	if err := db.AutoMigrate(
		&model.Dorm{},
		&model.Machine{},
		&model.OccupancyOpen{},
		&model.OccupancyHistory{},
	); err != nil {
		return nil, fmt.Errorf("automigrate failed: %w", err)
	}

	if cfg.EnableTimescale {
		log.Println("TimescaleDB is enabled, applying TimescaleDB-specific DDL...")
		if err := applyTimescaleDDL(db); err != nil {
			log.Printf("Warning: failed to apply some TimescaleDB DDL: %v. Continuing without them.", err)
		}
	}

	log.Println("Database initialization complete.")
	return db, nil
}

// func applyTimescaleDDL(db *gorm.DB) error {
// 	ddls := []string{
// 		"CREATE EXTENSION IF NOT EXISTS btree_gist;",
// 		"SELECT create_hypertable('occupancy_histories', 'observed_at', if_not_exists => TRUE);",
// 		"ALTER TABLE occupancy_histories ADD COLUMN period tstzrange;",
// 		// The application layer is now responsible for populating the 'period' column.
// 		// "UPDATE occupancy_histories SET period = tstzrange(observed_at, LEAD(observed_at, 1, 'infinity') OVER (PARTITION BY machine_id ORDER BY observed_at));",
// 		"CREATE INDEX idx_occupancy_history_period ON occupancy_histories USING GIST (machine_id, period);",
// 		"CREATE INDEX idx_occupancy_history_machine_id_observed_at ON occupancy_histories (machine_id, observed_at DESC);",
// 	}

// 	for _, ddl := range ddls {
// 		if err := db.Exec(ddl).Error; err != nil {
// 			log.Printf("DDL execution warning (query: %q): %v", ddl, err)
// 		}
// 	}

// 	return nil
// }

func applyTimescaleDDL(db *gorm.DB) error {
	ddls := []string{
		// 1) 必备扩展
		"CREATE EXTENSION IF NOT EXISTS timescaledb;",
		"CREATE EXTENSION IF NOT EXISTS btree_gist;",

		// 2) 把 occupancy_histories 设为 hypertable（observed_at 为 time dimension）
		"SELECT create_hypertable('occupancy_histories', 'observed_at', if_not_exists => TRUE);",

		// 3) 基本校验：起止必须有效
		"ALTER TABLE occupancy_histories " +
			"ADD CONSTRAINT occupancy_histories_period_valid CHECK (period_start < period_end);",

		// 4) 表达式 GIST 索引：支持 @>、&& 等范围操作（下界闭、上界开）
		"CREATE INDEX idx_occupancy_history_period_expr ON occupancy_histories " +
			"USING GIST (machine_id, tstzrange(period_start, period_end, '[)'));",

		// 5) 常用倒序时间索引：便于按机器拉最新记录
		"CREATE INDEX idx_occupancy_history_machine_id_observed_at ON occupancy_histories (machine_id, observed_at DESC);",
	}

	for _, ddl := range ddls {
		if err := db.Exec(ddl).Error; err != nil {
			return fmt.Errorf("DDL failed on %q: %w", ddl, err)
		}
	}
	return nil
}
