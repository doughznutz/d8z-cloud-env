package jobs

import (
	"encoding/json"
	"net/http"
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

// JobsResponse is api.jobs()'s result: the catalog, with defaults and
// choices filled in, and the .env's own default project.
type JobsResponse struct {
	Jobs           []JobSpec `json:"jobs"`
	DefaultProject string    `json:"defaultProject"`
}

// RunsResponse is api.jobRuns()'s result: the live/last state of every
// job run, keyed by "<group>/<name>".
type RunsResponse struct {
	Runs map[string]map[string]string `json:"runs"`
}

// RegisterRoutes adds the Jobs tab's endpoints: the catalog, the live/last
// state of every run, starting one, and reading its log.
func RegisterRoutes(mux *http.ServeMux, c *Catalog) {
	mux.HandleFunc("GET /api/jobs", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, JobsResponse{Jobs: c.JobCatalog(), DefaultProject: defaultProject()})
	})
	mux.HandleFunc("GET /api/jobs/runs", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, RunsResponse{Runs: c.JobRuns()})
	})
	mux.HandleFunc("POST /api/jobs/{group}/{name}", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Project string            `json:"project"` // "" or "self": the .env's PROJECT
			Args    map[string]string `json:"args"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil && err.Error() != "EOF" {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		id, err := c.RunJob(r.PathValue("group"), r.PathValue("name"), req.Project, req.Args)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"id": id, "state": "starting"})
	})
	mux.HandleFunc("GET /api/jobs/log", func(w http.ResponseWriter, r *http.Request) {
		s, ok := c.JobLog(r.URL.Query().Get("id"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write([]byte(s))
	})
}
