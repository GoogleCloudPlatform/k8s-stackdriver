/*
Copyright 2026 Google Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Command cloud-monitoring-proxy reads a configured set of metrics from
// Google Cloud Monitoring and re-exposes them on a Prometheus /metrics
// endpoint.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/cache"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/config"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/export"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/gcm"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/poll"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/scope"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/telemetry"
)

// version is stamped at build time via -ldflags.
var version = "dev"

func main() {
	var (
		configPath = flag.String("config", "", "path to the YAML config file")
		listen     = flag.String("listen", ":9090", "HTTP listen address (overridden by server.listen in config)")
	)
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	if err := run(context.Background(), *configPath, *listen); err != nil {
		slog.Error("exiting", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, configPath, listen string) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	mux := newMux()

	if configPath != "" {
		cfgStore, err := config.NewStore(configPath)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		go func() {
			if err := cfgStore.Watch(ctx); err != nil {
				slog.Error("config watcher stopped", "error", err)
			}
		}()
		cfg := cfgStore.Current()
		listen = cfg.Server.Listen

		sc, err := scope.Discover(ctx, cfg.Scope)
		if err != nil {
			return err
		}
		slog.Info("scope resolved", "project", sc.ProjectID, "location", sc.Location,
			"cluster", sc.ClusterName, "clusterScoped", sc.ClusterScopeFilter)

		tm := telemetry.New(version)
		tm.Registry.MustRegister(telemetry.ConfigCollector(cfgStore))

		client, err := gcm.New(ctx, sc.ProjectID, cfg.Polling.Concurrency, tm.GCMHooks())
		if err != nil {
			return err
		}
		defer func() { _ = client.Close() }()

		descs := gcm.NewDescriptorCache(client)
		go refreshDescriptors(ctx, descs, cfg.Polling.DescriptorRefresh.Std())

		store := cache.NewStore()
		tm.Registry.MustRegister(telemetry.CacheCollector(store))
		mgr := poll.New(client, descs, store, sc, tm.PollHooks())
		go mgr.Run(ctx, cfgStore)

		gatherer := export.NewGatherer(store, func() bool {
			return cfgStore.Current().Server.EmitTimestamps
		})
		mux.Handle("GET /metrics", export.MetricsHandler(gatherer, tm.Registry))
		mux.Handle("GET /readyz", export.ReadyzHandler(mgr.Ready))
		mux.Handle("GET /api/v1/discovery", export.DiscoveryHandler(descs))
		slog.Info("pipeline wired", "metrics", len(cfg.Metrics), "listen", listen, "version", version)
	}

	srv := &http.Server{
		Addr:              listen,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("http server listening", "addr", listen, "version", version)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("shutting down http server: %w", err)
	}
	return nil
}

// refreshDescriptors retries the initial load every 30s until it succeeds
// (the poller reports no_descriptor until then), then refreshes at the
// configured cadence.
func refreshDescriptors(ctx context.Context, descs *gcm.DescriptorCache, interval time.Duration) {
	for {
		err := descs.Refresh(ctx)
		if err == nil {
			slog.Info("descriptor cache loaded", "descriptors", descs.Len())
			break
		}
		slog.Error("initial descriptor refresh failed; retrying in 30s", "error", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
		}
	}
	descs.StartRefreshing(ctx, interval)
}

func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, "ok")
	})
	return mux
}
