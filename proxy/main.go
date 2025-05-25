package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

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
	}

	return func(w http.ResponseWriter, r *http.Request) {
		originalPath := r.URL.Path
		r.URL.Path = strings.TrimPrefix(r.URL.Path, prefix)
		r.Host = targetURL.Host
		log.Printf("[REVERSE] %s → %s%s", originalPath, target, r.URL.Path)
		proxy.ServeHTTP(w, r)
	}
}

// ForwardProxy handles outbound traffic (internal containers → Internet)
func ForwardProxyHandler(w http.ResponseWriter, req *http.Request) {
	log.Printf("[FORWARD] %s %s", req.Method, req.URL.String())

	if req.URL.Scheme == "" {
		// HTTP/1.1 CONNECT method (e.g., for HTTPS)
		handleTunneling(w, req)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(req.URL)
	proxy.ServeHTTP(w, req)
}

// CONNECT handling for HTTPS tunneling
func handleTunneling(w http.ResponseWriter, req *http.Request) {
	destConn, err := net.Dial("tcp", req.Host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	fmt.Fprint(clientConn, "HTTP/1.1 200 Connection Established\r\n\r\n")

	go transfer(destConn, clientConn)
	go transfer(clientConn, destConn)
}

func transfer(dst io.WriteCloser, src io.ReadCloser) {
	defer dst.Close()
	defer src.Close()
	io.Copy(dst, src)
}

func main() {
	// Reverse proxy: port 80
	go func() {
		r := chi.NewRouter()
		r.Handle("/", http.FileServer(http.Dir("./static")))
		r.Handle("/env/*", ReverseProxy("http://base", "/env"))
		r.Handle("/docker/*", ReverseProxy("http://docker", "/docker"))
		r.Handle("/voideditor/*", ReverseProxy("http://voideditor", "/voideditor"))
		r.Handle("/vscode/*", ReverseProxy("http://vscode", "/vscode"))
        // code-server has issues going through the proxy, so use http://localhost:8080
        // r.Handle("/vscode-server/*", ReverseProxy("http://vscode:8080", "/vscode-server"))


		log.Println("Reverse proxy listening on :80")
		log.Fatal(http.ListenAndServe(":80", r))
	}()

	// Forward proxy: port 3128
	http.HandleFunc("/", ForwardProxyHandler)
	log.Println("Forward proxy listening on :443")
	log.Fatal(http.ListenAndServe(":443", nil))
}
