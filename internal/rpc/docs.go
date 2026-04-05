package rpc

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed swagger-ui/*
var swaggerUI embed.FS

//go:embed openapi.yaml
var openapiSpec []byte

func registerDocs(mux *http.ServeMux) {
	mux.HandleFunc("/docs/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write(openapiSpec)
	})

	uiFS, _ := fs.Sub(swaggerUI, "swagger-ui")
	fileServer := http.FileServer(http.FS(uiFS))
	mux.Handle("/docs/", http.StripPrefix("/docs/", fileServer))

	mux.HandleFunc("/docs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/docs/", http.StatusMovedPermanently)
	})
}
