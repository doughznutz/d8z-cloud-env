package main

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// ReverseProxy routes inbound requests
func ReverseProxy(target string, prefix string) http.HandlerFunc {
	targetURL, _ := url.Parse(target)
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Header.Set("X-Forwarded-For", req.Header.Get("X-Real-IP")) // Or use req.RemoteAddr if X-Real-IP isn't set
		req.Header.Set("X-Forwarded-Proto", "http")                    // Or "https" if your proxy is behind HTTPS
		req.Header.Set("X-Forwarded-Host", req.Host)
		req.Header.Set("Host", targetURL.Host) // Ensure the Host header is correctly set

		// Handle websockets
		if strings.ToLower(req.Header.Get("Connection")) == "upgrade" && strings.ToLower(req.Header.Get("Upgrade")) == "websocket" {
			req.Header.Set("Connection", "Upgrade")
			req.Header.Set("Upgrade", "websocket")
		} else {
			log.Printf("Connection = %s, Upgrade = %s\n", req.Header.Get("Connection"), req.Header.Get("Upgrade"))
		}

		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("Reverse proxy error: %v", err)
			http.Error(w, "Proxy Error", http.StatusInternalServerError)
		}
	}

	return func(w http.ResponseWriter, r *http.Request) {
		originalPath := r.URL.Path
		r.URL.Path = strings.TrimPrefix(r.URL.Path, prefix)
		log.Printf("[REVERSE] %s → %s,%s", originalPath, target, r.URL.Path)
		proxy.ServeHTTP(w, r)
	}
}

func main() {

	// Fashboard / Router: port 80

	r := chi.NewRouter()

	r.Handle("/", http.FileServer(http.Dir("./static")))
	r.Get("/status", serviceStatus)

	// Redirect VNC traffic through to the respective servers. 80->80
	r.Handle("/vnc/*", ReverseProxy("http://vnc:6080", "/vnc"))
	r.Handle("/voideditor/*", ReverseProxy("http://voideditor:6080", "/voideditor"))
	r.Handle("/vscode/*", ReverseProxy("http://vscode:6080", "/vscode"))


	// Things that need a port change or https dont work here.
	// Maybe separate the dashboard from the reverse proxy
	// code-server has issues going through the proxy, so use http://localhost:8080
	// r.Handle("/vscode-server/*", ReverseProxy("http://vscode:8080", "/vscode-server"))
	// r.Handle("/adminer/*", ReverseProxy("http://adminer", "/adminer"))
	// r.Handle("/dozzle*", ReverseProxy("http://dozzle:8080", "/dozzle"))
	
	log.Println("Dashboard listening on :80")
	log.Fatal(http.ListenAndServe(":80", r))

}

func serviceStatus(w http.ResponseWriter, r *http.Request) {
	services := map[string]string{
		"vnc":         "vnc:6080",
		"voideditor":  "voideditor:6080",
		"vscode":      "vscode:6080",
		"codeserver":  "codeserver:8080",
		"dozzle":      "dozzle:8080",
		"adminer":     "adminer:8080",
	}

	status := make(map[string]string)
	for name, address := range services {
		_, err := net.DialTimeout("tcp", address, 1*time.Second)
		if err != nil {
			status[name] = "down"
		} else {
			status[name] = "running"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

