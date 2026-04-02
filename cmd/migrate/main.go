package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	migratepkg "github.com/golang-migrate/migrate/v4"
	"github.com/possibities/gin-core/internal/migration"
	"github.com/possibities/gin-core/pkg/config"
)

func main() {
	action := flag.String("action", "version", "migration action: up, down, steps, version")
	steps := flag.Int("steps", 0, "number of migration steps when action=steps")
	flag.Parse()

	cfg, err := config.LoadForMigration()
	if err != nil {
		panic(fmt.Errorf("load config: %w", err))
	}

	migrator := migration.New(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), migrator.Timeout())
	defer cancel()

	switch *action {
	case "up":
		err = migrator.Up(ctx)
	case "down":
		err = migrator.Down(ctx)
	case "steps":
		if *steps == 0 {
			panic("steps must be non-zero when action=steps")
		}
		err = migrator.Steps(ctx, *steps)
	case "version":
		version, dirty, versionErr := migrator.Version(ctx)
		if versionErr != nil {
			err = versionErr
			break
		}
		fmt.Printf("version=%d dirty=%t\n", version, dirty)
		return
	default:
		panic(fmt.Sprintf("unsupported action: %s", *action))
	}

	if err != nil {
		var dirty migratepkg.ErrDirty
		if errors.As(err, &dirty) {
			fmt.Fprintf(os.Stderr, "migration dirty state at version %d: %v\n", dirty.Version, err)
			os.Exit(1)
		}
		panic(err)
	}
}
