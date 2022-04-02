package main

import (
	"encoding/json"
	"fmt"
	"github.com/jmcvetta/randutil"
	"github.com/spf13/viper"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
)

func statusHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func redirectHandler(w http.ResponseWriter, r *http.Request) {
	ipStr, _, err := net.SplitHostPort(r.RemoteAddr)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ip := net.ParseIP(ipStr)

	if ip.IsLoopback() || ip.IsPrivate() {
		overrideIP := os.Getenv("OVERRIDE_IP")

		if overrideIP == "" {
			overrideIP = "1.1.1.1"
		}

		ip = net.ParseIP(overrideIP)
	}

	var server *Server
	var distance float64

	if strings.HasPrefix(r.URL.Path, "/region") {
		parts := strings.Split(r.URL.Path, "/")

		// region = parts[2]
		if mirrors, ok := regionMap[parts[2]]; ok {
			choices := make([]randutil.Choice, len(mirrors))

			for i, item := range mirrors {
				if !item.Available {
					continue
				}

				choices[i] = randutil.Choice{
					Weight: item.Weight,
					Item:   item,
				}
			}

			choice, err := randutil.WeightedChoice(choices)

			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			server = choice.Item.(*Server)

			r.URL.Path = strings.Join(parts[3:], "/")
		}
	}

	if server == nil {
		server, distance, err = servers.Closest(ip)

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	scheme := r.URL.Scheme

	if scheme == "" {
		scheme = "https"
	}

	redirectPath := path.Join(server.Path, r.URL.Path)

	if dlMap != nil {
		if newPath, exists := dlMap[strings.TrimLeft(r.URL.Path, "/")]; exists {
			downloadsMapped.Inc()
			redirectPath = path.Join(server.Path, newPath)
		}
	}

	if strings.HasSuffix(r.URL.Path, "/") && !strings.HasSuffix(redirectPath, "/") {
		redirectPath += "/"
	}

	u := &url.URL{
		Scheme: scheme,
		Host:   server.Host,
		Path:   redirectPath,
	}

	server.Redirects.Inc()
	redirectsServed.Inc()

	if distance > 0 {
		w.Header().Set("X-Geo-Distance", fmt.Sprintf("%f", distance))
	}

	w.Header().Set("Location", u.String())
	w.WriteHeader(http.StatusFound)
}

func reloadHandler(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("Authorization")

	if token == "" || !strings.HasPrefix(token, "Bearer") || !strings.Contains(token, " ") {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	token = token[strings.Index(token, " ")+1:]

	if token != viper.GetString("reloadToken") {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	reloadConfig()

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func dlMapHandler(w http.ResponseWriter, r *http.Request) {
	if dlMap == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(dlMap)
}

func geoIPHandler(w http.ResponseWriter, r *http.Request) {
	ipStr, _, err := net.SplitHostPort(r.RemoteAddr)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ip := net.ParseIP(ipStr)

	var city City
	err = db.Lookup(ip, &city)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(city)
}
