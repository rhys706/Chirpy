package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/rhys706/chirpy/internal/database"
)

type apiConfig struct {
	fileserverHits atomic.Int32
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1) // Safely increments
		next.ServeHTTP(w, r)      // Calls the next handler in the middleware chain
	})
}

func (cfg *apiConfig) metricsHandler(w http.ResponseWriter, r *http.Request) {
	hits := cfg.fileserverHits.Load() // Safely reads the integer value
	w.Header().Set("Content-Type", "text/html")
	html := `<html>
	<body>
		<h1>Welcome, Chirpy Admin</h1>
		<p>Chirpy has been visited %d times!</p>
	</body>
	</html>`
	fmt.Fprintf(w, html, hits)
}

func (cfg *apiConfig) resetHandler(w http.ResponseWriter, r *http.Request) {
	cfg.fileserverHits.Store(0) // Resets the hit counter to 0
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("Fileserver hit counter reset"))
}

func profanity_replacer(tweet string) string {
	word_list := strings.Split(tweet, " ")
	for i, word := range word_list {
		if strings.ToLower(word) == "kerfuffle" || strings.ToLower(word) == "sharbert" || strings.ToLower(word) == "fornax" {
			word_list[i] = "****"
		}
	}
	sentence := strings.Join(word_list, " ")
	return sentence
}

func jsonHandler(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		// these tags indicate how the keys in the JSON should be mapped to the struct fields
		// the struct fields must be exported (start with a capital letter) if you want them parsed
		Body string `json:"body"`
	}

	type goodReturnVals struct {
		// the key will be the name of struct field unless you give it an explicit JSON tag
		// Valid bool `json:"valid"`
		Body string `json:"cleaned_body"`
	}

	type badReturnVals struct {
		// the key will be the name of struct field unless you give it an explicit JSON tag
		Error string `json:"error`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respBody := badReturnVals{
			Error: "Something went wrong",
		}
		dat, _ := json.Marshal(respBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write(dat)
		return
	}

	if n := len(params.Body); n > 140 {
		respBody := badReturnVals{
			Error: "Chirp is too long",
		}
		dat, _ := json.Marshal(respBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		w.Write(dat)
		return
	}

	sentence := profanity_replacer(params.Body)

	respBody := goodReturnVals{
		Body: sentence,
	}
	dat, _ := json.Marshal(respBody)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(dat)
	return
}

func main() {
	godotenv.Load()

	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	dbQueries := database.New(db)

	if err != nil {
		fmt.Println(dbQueries)
	}

	apiCfg := apiConfig{}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8") // normal header
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	mux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app/", http.FileServer(http.Dir(".")))))
	mux.Handle("GET /admin/metrics", http.HandlerFunc(apiCfg.metricsHandler))
	mux.Handle("POST /admin/reset", http.HandlerFunc(apiCfg.resetHandler))
	mux.Handle("POST /api/validate_chirp", http.HandlerFunc(jsonHandler))

	server := http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	err = server.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}
