package main

import (
	"encoding/base64"
	"html/template"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/pressly/chi"
	"github.com/pressly/chi/render"
)

type H map[string]interface{}

// HTTP basic auth middleware
func basicAuth(password string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)

			s := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
			if len(s) != 2 {
				http.Error(w, http.StatusText(401), 401)
				return
			}

			b, err := base64.StdEncoding.DecodeString(s[1])
			if err != nil {
				log.Printf("base64.StdEncoding.DecodeString() Error: %s\n", err)
				http.Error(w, http.StatusText(401), 401)
				return
			}

			pair := strings.SplitN(string(b), ":", 2)
			if len(pair) != 2 {
				log.Printf("strings.SplitN() Error: %s\n", err)
				http.Error(w, http.StatusText(401), 401)
				return
			}

			if pair[0] != "admin" || pair[1] != password {
				http.Error(w, http.StatusText(401), 401)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// GET / (root index)
func rootIndexHandler(w http.ResponseWriter, r *http.Request) {
	var data map[string]*Record
	q := r.FormValue("q")
	p := r.FormValue("p")

	if q != "" { // GET /?q=query
		if len(q) >= 3 {
			data = db.find(q)
		} else {
			http.Error(w, http.StatusText(422), 422)
			return
		}
	} else if p == "1" { // GET /?p=1
		data = db.getPaused()
	}

	totalCount, err := db.keyCount()
	if err != nil {
		log.Printf("db.keyCount() Error: %s\n", err)
		http.Error(w, http.StatusText(500), 500)
		return
	}

	tmpl, err := template.New("index").Parse(indexTmpl)
	if err != nil {
		log.Printf("template.ParseFiles() Error: %s\n", err)
		http.Error(w, http.StatusText(500), 500)
		return
	}

	if err = tmpl.Execute(w, H{"data": data, "total_count": totalCount, "q": q, "p": p}); err != nil {
		log.Printf("tmpl.Execute() Error: %s\n", err)
		http.Error(w, http.StatusText(500), 500)
	}
}

// POST /
func rootCreateHandler(w http.ResponseWriter, r *http.Request) {
	var rec *Record

	key := r.FormValue("key")
	if len(key) < 4 {
		http.Error(w, http.StatusText(422), 422)
		return
	}

	p := r.FormValue("paused")
	if p == "1" {
		rec = &Record{Paused: true}
	}

	// Save
	if err := db.put(key, rec); err != nil {
		log.Printf("db.put(%s) Error: %s\n", key, err)
		http.Error(w, http.StatusText(500), 500)
		return
	}

	// Redirect to key view
	http.Redirect(w, r, strings.Join([]string{"/", key}, ""), 302)
}

// GET /:key
func rootReadHandler(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")

	rec, err := db.get(key)
	if err == errRecordNotFound {
		http.Error(w, http.StatusText(404), 404)
		return
	} else if err != nil {
		log.Printf("db.get(%s) Error: %s\n", key, err)
		http.Error(w, http.StatusText(500), 500)
		return
	}
	data := map[string]*Record{key: rec}

	totalCount, err := db.keyCount()
	if err != nil {
		log.Printf("db.keyCount() Error: %s\n", err)
		http.Error(w, http.StatusText(500), 500)
		return
	}

	tmpl, err := template.New("index").Parse(indexTmpl)
	if err != nil {
		log.Printf("template.ParseFiles() Error: %s\n", err)
		http.Error(w, http.StatusText(500), 500)
		return
	}

	if err = tmpl.Execute(w, H{"data": data, "total_count": totalCount}); err != nil {
		log.Printf("tmpl.Execute() Error: %s\n", err)
		http.Error(w, http.StatusText(500), 500)
	}
}

// GET /api/
func apiIndexHandler(w http.ResponseWriter, r *http.Request) {
	var data map[string]*Record

	q := r.FormValue("q")
	p := r.FormValue("p")
	if len(q) >= 3 { // GET /api/?q=query
		data = db.find(q)
	} else if p == "1" { // GET /api/?p=1
		data = db.getPaused()
	} else {
		http.Error(w, http.StatusText(422), 422)
		return
	}

	render.JSON(w, r, H{"data": data})
}

// GET /api/:key
func apiReadHandler(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")

	data, err := db.get(key)
	if err == errRecordNotFound {
		http.Error(w, http.StatusText(404), 404)
		return
	} else if err != nil {
		log.Printf("db.get(%s) Error: %s\n", key, err)
		http.Error(w, http.StatusText(500), 500)
		return
	}

	render.JSON(w, r, H{"data": H{key: data}})
}

// PUT /api/:key
func apiPutHandler(w http.ResponseWriter, r *http.Request) {
	var data Record

	key := chi.URLParam(r, "key")
	if len(key) < 4 {
		http.Error(w, http.StatusText(422), 422)
		return
	}

	// Bind
	if err := render.Bind(r.Body, &data); err != nil && err != io.EOF {
		log.Printf("render.Bind() Error: %s\n", err)
		http.Error(w, http.StatusText(400), 400)
		return
	}

	// Save
	if err := db.put(key, &data); err != nil {
		log.Printf("db.put(%s) Error: %s\n", key, err)
		http.Error(w, http.StatusText(500), 500)
		return
	}

	render.JSON(w, r, H{"data": H{key: data}})
}

// DELETE /api/:key
func apiDeleteHandler(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")

	// Delete
	if err := db.delete(key); err != nil {
		log.Printf("db.delete(%s) Error: %s\n", key, err)
		http.Error(w, http.StatusText(500), 500)
		return
	}

	render.NoContent(w, r)
}

// GET /css/nogo.css
func cssHandler(w http.ResponseWriter, r *http.Request) {
	var data = []byte(nogoCSS)

	w.Header().Set("Cache-Control", "public, max-age=31536000")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Content-Type", "text/css")

	w.Write(data)
}
