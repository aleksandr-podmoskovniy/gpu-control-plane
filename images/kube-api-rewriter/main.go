package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/client-go/rest"
)

var (
	upstreamAddr    = flag.String("upstream", "https://kubernetes.default.svc", "Upstream Kubernetes API server")
	listenAddr      = flag.String("listen", "127.0.0.1:23915", "Address to listen for controller requests")
	metricsAddr     = flag.String("metrics-listen", "127.0.0.1:9090", "Address to serve metrics and health endpoints")
	metricsRegistry = prometheus.NewRegistry()
)

func main() {
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	restCfg, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("failed to build in-cluster config: %v", err)
	}

	transport, err := rest.TransportFor(restCfg)
	if err != nil {
		log.Fatalf("failed to construct transport: %v", err)
	}

	upstreamURL, err := url.Parse(*upstreamAddr)
	if err != nil {
		log.Fatalf("failed to parse upstream address: %v", err)
	}

	reverse := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = upstreamURL.Scheme
			req.URL.Host = upstreamURL.Host
			if upstreamURL.Path != "" {
				req.URL.Path = singleJoiningSlash(upstreamURL.Path, req.URL.Path)
			}
			if upstreamURL.RawQuery != "" {
				req.URL.RawQuery = upstreamURL.RawQuery + "&" + req.URL.RawQuery
			}
			if req.URL.RawQuery == "" {
				req.URL.RawQuery = upstreamURL.RawQuery
			}
			if req.Header.Get("Authorization") == "" && restCfg.BearerToken != "" {
				req.Header.Set("Authorization", "Bearer "+restCfg.BearerToken)
			}
			if restCfg.UserAgent != "" {
				req.Header.Set("User-Agent", restCfg.UserAgent)
			}
		},
		Transport:     transport,
		FlushInterval: 200 * time.Millisecond,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/proxy/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/proxy/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	metricsRegistry.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	mux.Handle("/proxy/metrics", promhttp.HandlerFor(metricsRegistry, promhttp.HandlerOpts{}))

	proxyServer := &http.Server{
		Addr:    *listenAddr,
		Handler: withLogging(reverse),
	}

	metricsServer := &http.Server{
		Addr:    *metricsAddr,
		Handler: mux,
	}

	errorC := make(chan error, 2)
	go func() {
		log.Printf("kube-api-rewriter listening on %s", *listenAddr)
		errorC <- proxyServer.ListenAndServe()
	}()
	go func() {
		log.Printf("metrics listening on %s", *metricsAddr)
		errorC <- metricsServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelShutdown()
		_ = proxyServer.Shutdown(shutdownCtx)
		_ = metricsServer.Shutdown(shutdownCtx)
	case err := <-errorC:
		if err != nil && !errorsIsServerClosed(err) {
			log.Fatalf("server error: %v", err)
		}
	}
}

func singleJoiningSlash(a, b string) string {
	aSlash := strings.HasSuffix(a, "/")
	bSlash := strings.HasPrefix(b, "/")
	switch {
	case aSlash && bSlash:
		return a + b[1:]
	case !aSlash && !bSlash:
		return a + "/" + b
	default:
		return a + b
	}
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.String(), rec.status, time.Since(start))
	})
}

func errorsIsServerClosed(err error) bool {
	return err == http.ErrServerClosed
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	r.status = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}
