// gw-server is the console backend inside each D8Z gateway (Traefik)
// container: it serves the Vite console and spawns/stops child gateway
// bundles, keeping Traefik's file-provider routes in sync with them.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func main() {
	cfg := loadConfig()
	m := NewManager(cfg)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/self", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		ver, err := dockerOut(ctx, "version", "--format", "{{.Server.Version}}")
		resp := map[string]any{
			"id": cfg.ID, "prefix": cfg.Prefix, "role": cfg.Role, "isolated": cfg.Isolated, "dozzlePath": cfg.DozzlePath,
			"services": cfg.Services, "profiles": profiles, "docker": ver, "dockerOK": err == nil,
			"localServices": localServices, "localOps": m.LocalOps(), "projectsEnabled": cfg.ProjectsRoot != "",
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
		if err := m.Stop(r.PathValue("name")); err != nil {
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

	// Local services: start/stop containers in our own compose project
	// (deightz, the ollama gateway, gopher, the otel collector).
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

	// Projects: sibling source trees under GW_PROJECTS_ROOT. Provisioned
	// (self-template or git clone) and, if they carry their own
	// docker-compose.yaml, brought up/down directly - no nested gateway.
	mux.HandleFunc("GET /api/projects", func(w http.ResponseWriter, r *http.Request) {
		ps, err := m.ListProjects(r.Context())
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"projects": ps})
	})
	mux.HandleFunc("POST /api/projects", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name   string        `json:"name"`
			Source ProjectSource `json:"source"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if err := m.CreateProject(req.Name, req.Source); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"name": req.Name, "state": "starting"})
	})
	mux.HandleFunc("POST /api/projects/{name}/up", func(w http.ResponseWriter, r *http.Request) {
		if err := m.StartProject(r.PathValue("name")); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"state": "starting"})
	})
	mux.HandleFunc("DELETE /api/projects/{name}", func(w http.ResponseWriter, r *http.Request) {
		if err := m.StopProject(r.PathValue("name")); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"state": "stopping"})
	})
	mux.HandleFunc("GET /api/projects/{name}/log", func(w http.ResponseWriter, r *http.Request) {
		s, ok := m.ProjectLog(r.PathValue("name"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write([]byte(s))
	})

	// Jobs: one-shot actions against external systems (GitHub and Docker
	// Hub natively, GCP via the deploy/cloud container) - no running/stopped
	// state, just fire, stream, done/error.
	mux.HandleFunc("GET /api/jobs", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"jobs": JobCatalog(), "defaultProject": defaultProject()})
	})
	mux.HandleFunc("GET /api/jobs/runs", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"runs": m.JobRuns()})
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
