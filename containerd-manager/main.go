package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/containerd/v2/pkg/oci"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

const (
	containerdSocket = "/run/containerd/containerd.sock"
	imageRef         = "docker.io/library/chat-server:latest"
	namespace        = "default"
	restartDelay     = 3 * time.Second
)

type serverConfig struct {
	id       string
	serverID string
	port     int
}

var servers = []serverConfig{
	{"chat-server-1", "1", 9001},
	{"chat-server-2", "2", 9002},
	{"chat-server-3", "3", 9003},
}

var mu sync.Mutex
var stopping bool

func main() {
	c, err := client.New(containerdSocket)
	if err != nil {
		log.Fatalf("failed to connect to containerd: %v", err)
	}
	defer c.Close()

	ctx := namespaces.WithNamespace(context.Background(), namespace)

	cleanupAll(ctx, c)

	for _, s := range servers {
		if err := startChatServer(ctx, c, s, true); err != nil {
			log.Printf("failed to start %s: %v", s.id, err)
		}
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down...")
	mu.Lock()
	stopping = true
	mu.Unlock()
	cleanupAll(ctx, c)
}

func startChatServer(ctx context.Context, c *client.Client, s serverConfig, watch bool) error {
	image, err := c.GetImage(ctx, imageRef)
	if err != nil {
		return fmt.Errorf("image not found: %w", err)
	}

	container, err := c.NewContainer(ctx, s.id,
		client.WithImage(image),
		client.WithNewSnapshot(s.id+"-snapshot", image),
		client.WithNewSpec(
			oci.WithImageConfig(image),
			oci.WithHostNamespace(specs.NetworkNamespace),
			oci.WithEnv([]string{
				fmt.Sprintf("SERVER_ID=%s", s.serverID),
				fmt.Sprintf("PORT=%d", s.port),
				"GATEWAY_URL=http://127.0.0.1:8080",
				"HEARTBEAT_INTERVAL=3",
			}),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		container.Delete(ctx, client.WithSnapshotCleanup)
		return fmt.Errorf("failed to create task: %w", err)
	}

	exitCh, err := task.Wait(ctx)
	if err != nil {
		task.Delete(ctx)
		container.Delete(ctx, client.WithSnapshotCleanup)
		return fmt.Errorf("failed to wait on task: %w", err)
	}

	if err := task.Start(ctx); err != nil {
		task.Delete(ctx)
		container.Delete(ctx, client.WithSnapshotCleanup)
		return fmt.Errorf("failed to start task: %w", err)
	}

	log.Printf("started %s (SERVER_ID=%s, port=%d, pid=%d)", s.id, s.serverID, s.port, task.Pid())

	if watch {
		go func() {
			status := <-exitCh
			code, _, _ := status.Result()

			mu.Lock()
			isStopping := stopping
			mu.Unlock()

			if isStopping {
				return
			}

			log.Printf("[EXIT] %s exited with code %d — restarting in %s", s.id, code, restartDelay)
			task.Delete(ctx, client.WithProcessKill)
			container.Delete(ctx, client.WithSnapshotCleanup)

			time.Sleep(restartDelay)
			if err := startChatServer(ctx, c, s, true); err != nil {
				log.Printf("[RESTART FAILED] %s: %v", s.id, err)
			}
		}()
	}

	return nil
}

func cleanupAll(ctx context.Context, c *client.Client) {
	for _, s := range servers {
		if task, err := getTask(ctx, c, s.id); err == nil {
			task.Kill(ctx, syscall.SIGKILL)
			task.Delete(ctx, client.WithProcessKill)
		}
		if ctr, err := c.LoadContainer(ctx, s.id); err == nil {
			ctr.Delete(ctx, client.WithSnapshotCleanup)
		}
	}
}

func getTask(ctx context.Context, c *client.Client, id string) (client.Task, error) {
	ctr, err := c.LoadContainer(ctx, id)
	if err != nil {
		return nil, err
	}
	return ctr.Task(ctx, nil)
}
