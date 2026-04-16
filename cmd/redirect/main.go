package main

import (
	"flag"
	"log"
	"net/http"
	"strings"
)

func main() {
	addr := flag.String("addr", "0.0.0.0:8080", "address to listen on")
	flag.Parse()

	// Redirect paths per provider
	// /epic/<slug> -> com.epicgames.launcher://store/p/<slug>
	// /steam/<appid> -> steam://store/<appid>
	// /twitch/<campaign> -> twitch://stream/<campaign>
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		parts := strings.SplitN(path, "/", 2)
		if len(parts) < 2 {
			http.Error(w, "invalid path: expected /<provider>/<slug>", http.StatusBadRequest)
			return
		}

		provider, slug := parts[0], parts[1]

		var dest string
		switch provider {
		case "epic":
			dest = "com.epicgames.launcher://store/p/" + slug
		case "steam":
			dest = "steam://store/" + slug
		case "twitch":
			dest = "twitch://stream/" + slug
		default:
			http.Error(w, "unknown provider: "+provider, http.StatusBadRequest)
			return
		}

		log.Printf("Redirecting /%s/%s -> %s", provider, slug, dest)
		http.Redirect(w, r, dest, http.StatusMovedPermanently)
	})

	log.Printf("Redirect server listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}