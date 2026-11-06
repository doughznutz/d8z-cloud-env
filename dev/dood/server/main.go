// gw-server is the console backend inside each D8Z gateway - a dood
// container (Traefik + gw-server): it serves the Vite console and
// spawns/stops child gateway bundles, keeping Traefik's file-provider routes
// in sync with them.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// writeJSON sends v as an uncached JSON response with the given status code.
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

// writeErr sends err as a JSON {"error": ...} body with the given status code.
func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

// main loads the config, brings up the Manager (and its NATS hub), registers
// every /api route plus the console's static files, starts the reconcile loop
// that keeps Traefik routes in sync with running children, and serves HTTP on
// cfg.Listen under cfg.Prefix.
func main() {
	cfg := loadConfig()
	m := NewManager(cfg)

	if m.nats != nil {
		if err := m.nats.Hello("gw-server " + cfg.ID + " online"); err != nil {
			log.Printf("nats: hello: %v", err)
		}
		m.nats.StartHealthLoop(15*time.Second, nil)
		m.nats.ServePing()
	}
	m.startWorkspaceNats()

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/self", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		ver, err := dockerOut(ctx, "version", "--format", "{{.Server.Version}}")
		resp := map[string]any{
			"id": cfg.ID, "prefix": cfg.Prefix, "role": cfg.Role, "isolated": cfg.Isolated, "dozzlePath": cfg.DozzlePath,
			"services": cfg.Services, "profiles": profiles, "docker": ver, "dockerOK": err == nil,
			"localServices": localServices, "localOps": m.LocalOps(), "projectsEnabled": cfg.ProjectsRoot != "",
			"natsReady": m.nats != nil, "requests": m.Requests(),
		}
		if cfg.WorkspaceName != "" {
			ws := map[string]any{"name": cfg.WorkspaceName, "assigned": false}
			if a := m.assignment(); a != nil {
				ws = map[string]any{"name": a.Name, "source": a.Source, "repo": a.Repo, "assigned": true}
			}
			resp["workspace"] = ws
		}
		if err != nil {
			resp["dockerError"] = err.Error()
		}
		bundle, berr := m.Bundle(ctx)
		resp["bundle"] = bundle
		if berr != nil {
			resp["bundleError"] = berr.Error()
		}
		writeJSON(w, http.StatusOK, resp)
	})

	mux.HandleFunc("GET /api/gateways", func(w http.ResponseWriter, r *http.Request) {
		gws, err := m.List(r.Context())
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"gateways": gws, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"gateways": gws})
	})

	mux.HandleFunc("POST /api/gateways", func(w http.ResponseWriter, r *http.Request) {
		var req SpawnRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if err := m.Spawn(r.Context(), req); err != nil {
			code := http.StatusBadRequest
			if errors.Is(err, errExists) {
				code = http.StatusConflict
			}
			writeErr(w, code, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"name": req.Name, "state": "starting"})
	})

	mux.HandleFunc("DELETE /api/gateways/{name}", func(w http.ResponseWriter, r *http.Request) {
		// ?config=merge|branch|discard: what happens to its config changes
		// (default: saved as a branch).
		if err := m.Stop(r.PathValue("name"), r.URL.Query().Get("config")); err != nil {
			writeErr(w, http.StatusConflict, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"state": "stopping"})
	})

	logs := func(w http.ResponseWriter, r *http.Request, get func(context.Context) (string, error)) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		s, err := get(ctx)
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write([]byte(s))
	}
	mux.HandleFunc("GET /api/services/{svc}/logs", func(w http.ResponseWriter, r *http.Request) {
		logs(w, r, func(ctx context.Context) (string, error) { return m.BundleLogs(ctx, r.PathValue("svc")) })
	})
	mux.HandleFunc("GET /api/gateways/{name}/services/{svc}/logs", func(w http.ResponseWriter, r *http.Request) {
		logs(w, r, func(ctx context.Context) (string, error) {
			return m.ChildLogs(ctx, r.PathValue("name"), r.PathValue("svc"))
		})
	})

	mux.HandleFunc("GET /api/gateways/{name}/log", func(w http.ResponseWriter, r *http.Request) {
		s, ok := m.Log(r.PathValue("name"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write([]byte(s))
	})

	// NATS: this gateway's own inbox (always local/self - a descendant's
	// inbox is read the same way any of its other per-gateway data is, by
	// talking to its own gw-server through the console's nested iframe), and
	// sending a message either into our own inbox or straight into a named
	// child's - a plain publish that crosses its leaf link on its own, same
	// as the hello/health heartbeat already does.
	mux.HandleFunc("GET /api/nats/inbox", func(w http.ResponseWriter, r *http.Request) {
		if m.nats == nil {
			writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("nats not available"))
			return
		}
		max := 50
		if v := r.URL.Query().Get("max"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				max = n
			}
		}
		msgs, err := m.nats.Inbox(max)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"messages": msgs})
	})
	sendMsg := func(w http.ResponseWriter, r *http.Request, id string) {
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if m.nats == nil {
			writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("nats not available"))
			return
		}
		if err := m.nats.Publish(id, req.Text); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"state": "sent"})
	}
	mux.HandleFunc("POST /api/nats/send", func(w http.ResponseWriter, r *http.Request) {
		sendMsg(w, r, cfg.ID)
	})
	mux.HandleFunc("POST /api/gateways/{name}/nats/send", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !nameRE.MatchString(name) {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("bad name"))
			return
		}
		sendMsg(w, r, m.childID(name))
	})

	// A workspace gateway's open work (uncommitted / unpushed), asked of it
	// over NATS - the console shows it before stopping the gateway.
	mux.HandleFunc("GET /api/gateways/{name}/workspace-status", func(w http.ResponseWriter, r *http.Request) {
		st, err := m.ChildWorkspaceStatus(r.PathValue("name"))
		if err != nil {
			writeErr(w, http.StatusBadGateway, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(st)
	})

	// Local services: start/stop containers in our own compose project
	// (deightz and the optional og / gopher / carol / dozzle).
	mux.HandleFunc("POST /api/services/{svc}/start", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Project string `json:"project"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil && err.Error() != "EOF" {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if err := m.StartLocal(r.PathValue("svc"), req.Project); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"state": "starting"})
	})
	mux.HandleFunc("POST /api/services/{svc}/stop", func(w http.ResponseWriter, r *http.Request) {
		if err := m.StopLocal(r.PathValue("svc")); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"state": "stopping"})
	})

	registerWorkspaceRoutes(mux, m)
	registerRequestRoutes(mux, m)
	// Saved config branches (cfgtree.go).
	mux.HandleFunc("GET /api/branches", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"branches": m.listBranches()})
	})
	mux.HandleFunc("DELETE /api/branches/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := m.deleteBranch(r.PathValue("id")); err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"state": "deleted"})
	})
	// The Deployment tab: every gateway's own deployment.yaml (inherited
	// from its parent - config.go).
	registerDeploymentRoutes(mux, m)
	registerDeploymentEditRoutes(mux, m)

	// Jobs: one-shot actions against external systems (GitHub, Docker Hub,
	// GCP - all native) - no running/stopped state, just fire, stream,
	// done/error.
	mux.HandleFunc("GET /api/jobs", func(w http.ResponseWriter, r *http.Request) {
		catalog := m.JobCatalog()
		writeJSON(w, http.StatusOK, map[string]any{"jobs": catalog, "defaultProject": defaultProject()})
	})
	mux.HandleFunc("GET /api/jobs/runs", func(w http.ResponseWriter, r *http.Request) {
		runs := m.JobRuns()
		writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
	})
	mux.HandleFunc("POST /api/jobs/{group}/{name}", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Project string            `json:"project"` // "" or "self": the .env's PROJECT
			Args    map[string]string `json:"args"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil && err.Error() != "EOF" {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		id, err := m.RunJob(r.PathValue("group"), r.PathValue("name"), req.Project, req.Args)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"id": id, "state": "starting"})
	})
	mux.HandleFunc("GET /api/jobs/log", func(w http.ResponseWriter, r *http.Request) {
		s, ok := m.JobLog(r.URL.Query().Get("id"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write([]byte(s))
	})

	// The console: index.html is never cached (it names hashed assets).
	files := http.FileServer(http.Dir(cfg.StaticDir))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Cache-Control", "no-store")
			http.ServeFile(w, r, filepath.Join(cfg.StaticDir, "index.html"))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		files.ServeHTTP(w, r)
	})

	// Traefik forwards the full external path; drop our mount prefix.
	var h http.Handler = mux
	if cfg.Prefix != "" {
		h = http.StripPrefix(cfg.Prefix, mux)
	}

	go func() {
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := m.Reconcile(ctx); err != nil {
				log.Printf("reconcile: %v", err)
			}
			cancel()
			time.Sleep(15 * time.Second)
		}
	}()

	log.Printf("gw-server %s (prefix %q, isolated %v) on %s", cfg.ID, cfg.Prefix, cfg.Isolated, cfg.Listen)
	srv := &http.Server{Addr: cfg.Listen, Handler: h, ReadHeaderTimeout: 10 * time.Second}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
}
