// Offline maintenance only: no server initialization, workers or migrations.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/rcy1314/echo-noise/config"
	"github.com/rcy1314/echo-noise/internal/backup"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/updates"
)

func run() error {
	if len(os.Args) < 2 {
		return errors.New("plan, backup or settle required")
	}
	action := os.Args[1]
	flags := flag.NewFlagSet(action, flag.ContinueOnError)
	destination := flags.String("output", "", "archive outside all source directories")
	task := flags.String("task", "", "explicit task ID")
	instance := flags.String("instance", "", "confirmed paired instance ID")
	reason := flags.String("reason", "", "operator-confirmed-stop or manual-recovery-complete")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	if err := config.LoadConfig(); err != nil {
		return err
	}
	dbType := os.Getenv("DB_TYPE")
	if dbType == "" {
		dbType = config.Config.Database.Type
	}
	if dbType != "sqlite" {
		return errors.New("sqlite_backup_required")
	}
	if action != "plan" && action != "backup" && action != "settle" {
		return errors.New("unknown action")
	}
	db, err := backup.OpenOffline(database.SQLitePath(), action == "settle")
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)
	layout := backup.DefaultLayout()
	switch action {
	case "plan":
		plan, err := backup.PlanOffline(db, layout, "config")
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(plan)
	case "backup":
		if strings.TrimSpace(*destination) == "" {
			return errors.New("output required")
		}
		if err := backup.CreateOfflineArchive(*destination, db, layout, "config"); err != nil {
			return err
		}
	case "settle":
		// Invoked exclusively by a trusted host administrator with the app stopped.
		// It can record failure, never assert success or restore any business data.
		if err := updates.NewTaskService(db).SettleOffline(*instance, *task, *reason); err != nil {
			return err
		}
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]bool{"ok": true})
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
