package main

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/extra/bundebug"
	"github.com/uptrace/bun/migrate"
)

//go:embed migrations/*.sql
var sqlMigrations embed.FS

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Error loading .env file")
	}
	dsn := os.Getenv("DATABASE_URL")
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))

	db := bun.NewDB(sqldb, pgdialect.New())
	db.AddQueryHook(bundebug.NewQueryHook(
		bundebug.WithVerbose(true),
		bundebug.FromEnv("BUNDEBUG"),
	))

	migrations := migrate.NewMigrations()
	if err := migrations.Discover(sqlMigrations); err != nil {
		panic(err)
	}

	migrator := migrate.NewMigrator(db, migrations)
	ctx := context.Background()

	if err := migrator.Init(ctx); err != nil {
		log.Fatal(err)
	}

	var group *migrate.MigrationGroup

	if os.Getenv("DIR") == "down" {
		group, err = migrator.Rollback(ctx)
	} else {
		group, err = migrator.Migrate(ctx)
	}

	if err != nil {
		log.Fatal(err)
	}
	//group, err := migrator.Rollback(ctx)

	if group.IsZero() {
		fmt.Printf("there are no new migrations to run\n")
	}

	fmt.Printf("migrated to %s\n", group)
}
