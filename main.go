package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

var (
	wantDebug = flag.Bool("d", false, "debug mode")
)

const (
	logExt   = ".log"
	logDir   = "/var/lib/docker/containers"
	contName = "logspout"
)

type logSizeEntry struct {
	updated time.Time
	size    int64
}

type logSizeEntryMap map[string]logSizeEntry

func newLogSizeEntry(size int64) logSizeEntry {
	return logSizeEntry{size: size, updated: time.Now()}
}

func (l *logSizeEntry) updateSize(f os.FileInfo) {
	l.size = f.Size()
	l.updated = f.ModTime()
}

func main() {

	flag.Parse()

	// Map each container to its log info.
	containerLog := make(logSizeEntryMap)
	var mutex = &sync.Mutex{}

	cli, ctx := dockerEnv()

	// Garbage collect the map.
	go func(m *logSizeEntryMap) {
		maxTime := 24 * time.Hour // use max sizeOf instead?
		for {
			time.Sleep(1 * time.Hour)
			debug("starting gc")
			mutex.Lock()
			before := unsafe.Sizeof(*m)
			for k, v := range *m {
				if time.Since(v.updated) > maxTime {
					delete(*m, k)
				}
			}
			after := unsafe.Sizeof(*m)
			debug(fmt.Sprintf("finished gc %v %v ± %v", before, after, before-after))
			mutex.Unlock()
		}
	}(&containerLog)

	for {
		restart := false

		fmt.Println()
		filepath.Walk(logDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// If file is a log file...
			if !info.IsDir() && strings.HasSuffix(info.Name(), logExt) {
				mutex.Lock()
				cID := strings.Split(info.Name(), "-")[0]
				c, ok := containerLog[cID]

				// First iteration for the given container. No log information found.
				if !ok {
					containerLog[cID] = newLogSizeEntry(info.Size())
					debug(fmt.Sprintf("%s 0 %v ± %v", cID, info.Size(), info.Size()))
					mutex.Unlock()
					return nil
				}

				// Log rotation: Restart container.
				if info.Size() < c.size {
					debug(fmt.Sprintf("detected log rotation for %s", cID))
					delete(containerLog, cID)
					mutex.Unlock()
					restart = true
					return nil
				}

				// No log rotation: Simply update container log info.
				debug(fmt.Sprintf("%s %v %v ± %v", cID, c.size, info.Size(), info.Size()-c.size))
				c.updateSize(info)
				containerLog[cID] = c
				mutex.Unlock()
			}
			return err
		})

		if restart {
			restartLogspout(ctx, cli)
		}
		time.Sleep(10 * time.Second)
	}
}

func dockerEnv() (*client.Client, context.Context) {
	ctx := context.Background()
	cli, err := client.NewEnvClient()
	handleErr(err)
	return cli, ctx
}

func handleErr(err error) {
	if err != nil {
		panic(err)
	}
}

func runningContainers(ctx context.Context, cli *client.Client, options *types.ContainerListOptions) []types.Container {
	if options == nil {
		options = &types.ContainerListOptions{}
	}
	containers, err := cli.ContainerList(ctx, *options)
	handleErr(err)
	return containers
}

func restartLogspout(ctx context.Context, cli *client.Client) {
	containers := runningContainers(ctx, cli, &types.ContainerListOptions{Filters: func() filters.Args {
		f := filters.NewArgs()
		f.Add("name", contName)
		return f
	}()})
	for _, c := range containers {
		fmt.Printf("restarting container %s\n", c.Image)
		handleErr(cli.ContainerRestart(ctx, c.ID, nil))
	}
}

func debug(v ...interface{}) {
	if *wantDebug {
		for _, s := range v {
			fmt.Println(s)
		}
	}
}
