package main

import (
	"database/sql"
	"io/ioutil"
	"os"
	"time"

	"github.com/lxc/lxd/lxd/db"
	"github.com/lxc/lxd/lxd/task"
	"github.com/lxc/lxd/shared"
	"github.com/lxc/lxd/shared/logger"
	"golang.org/x/net/context"

	log "github.com/lxc/lxd/shared/log15"
)

// This task function expires logs when executed. It's started by the Daemon
// and will run once every 24h.
func expireLogsTask(db *sql.DB) (task.Func, task.Schedule) {
	f := func(context.Context) {
		logger.Infof("Expiring log files")
		err := expireLogs(db)
		if err != nil {
			logger.Error("Failed to expire logs", log.Ctx{"err": err})
		}
		logger.Infof("Done expiring log files")
	}
	return f, task.Daily()
}

func expireLogs(dbObj *sql.DB) error {
	entries, err := ioutil.ReadDir(shared.LogPath())
	if err != nil {
		return err
	}

	result, err := db.ContainersList(dbObj, db.CTypeRegular)
	if err != nil {
		return err
	}

	newestFile := func(path string, dir os.FileInfo) time.Time {
		newest := dir.ModTime()

		entries, err := ioutil.ReadDir(path)
		if err != nil {
			return newest
		}

		for _, entry := range entries {
			if entry.ModTime().After(newest) {
				newest = entry.ModTime()
			}
		}

		return newest
	}

	for _, entry := range entries {
		// Check if the container still exists
		if shared.StringInSlice(entry.Name(), result) {
			// Remove any log file which wasn't modified in the past 48 hours
			logs, err := ioutil.ReadDir(shared.LogPath(entry.Name()))
			if err != nil {
				return err
			}

			for _, logfile := range logs {
				path := shared.LogPath(entry.Name(), logfile.Name())

				// Always keep the LXC config
				if logfile.Name() == "lxc.conf" {
					continue
				}

				// Deal with directories (snapshots)
				if logfile.IsDir() {
					newest := newestFile(path, logfile)
					if time.Since(newest).Hours() >= 48 {
						os.RemoveAll(path)
						if err != nil {
							return err
						}
					}

					continue
				}

				// Individual files
				if time.Since(logfile.ModTime()).Hours() >= 48 {
					err := os.Remove(path)
					if err != nil {
						return err
					}
				}
			}
		} else {
			// Empty directory if unchanged in the past 24 hours
			path := shared.LogPath(entry.Name())
			newest := newestFile(path, entry)
			if time.Since(newest).Hours() >= 24 {
				err := os.RemoveAll(path)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}
