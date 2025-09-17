package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

// io blocking to maintain most recent data
type keyedLocks struct {
	mu    sync.Mutex
	locks map[string]*sync.RWMutex
}

func newKeyedLocks() *keyedLocks {
	return &keyedLocks{locks: make(map[string]*sync.RWMutex)}
}

func (k *keyedLocks) get(key string) *sync.RWMutex {
	k.mu.Lock()
	l, ok := k.locks[key]
	if !ok {
		l = &sync.RWMutex{}
		k.locks[key] = l
	}
	k.mu.Unlock()
	return l
}

var httpClient = &http.Client{}
var redisClient *redis.Client
var fileLocks = newKeyedLocks()

func hashKey(key string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(key))
	return (h.Sum32() % 5) + 1
}

func main() {
	godotenv.Load()
	redisClient = redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS_URL"),
		Password: "", // No password set
		DB:       0,  // Use default DB
	})

	// a request multiplexer distributes requests to their corresponding url endpoints or "patterns"
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleRoot)
	mux.HandleFunc("GET /health", getHealth)
	mux.HandleFunc("PUT /api/fileserver/{fileName}", putFile)
	mux.HandleFunc("GET /api/fileserver/{fileName}", getFile)
	mux.HandleFunc("DELETE /api/fileserver/{fileName}", deleteFile)

	log.Printf("Server listening to localhost:%s...", os.Getenv("PORT"))
	http.ListenAndServe(":"+os.Getenv("PORT"), mux)
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "You've reached my fileserver middleware!\n")
}

func getHealth(w http.ResponseWriter, r *http.Request) {
	resp := map[string]bool{"ok": true}
	b, _ := json.Marshal(resp)
	w.WriteHeader(http.StatusOK)
	w.Write(b)
}

func putFile(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	log.Println("PUT", r.URL.Path)
	// get url param
	fileName := r.PathValue("fileName")
	if fileName == "" {
		http.Error(w, "no file name given", http.StatusBadRequest)
		return
	}

	// get the shard from hash of filename
	shard := strconv.Itoa(int(hashKey(fileName)))
	shardUrl := strings.Replace(os.Getenv("FILE_SERVER_URL"), "#", shard, -1)

	// read body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}
	r.Body.Close() // Close body after reading bytes

	// send back early response
	w.WriteHeader(http.StatusCreated)
	flusher, ok := w.(http.Flusher)
	if ok {
		flusher.Flush()
	}

	go func(fileName string, data []byte) {
		// lock access to file while writing
		lock := fileLocks.get(fileName)
		lock.Lock()
		defer lock.Unlock()

		// update cache cache
		err = redisClient.Set(ctx, fileName, bodyBytes, 0).Err()
		if err != nil {
			log.Println("Redis SET error")
		}

		// make new request to fileserver
		req, err := http.NewRequest(http.MethodPut, shardUrl+"/"+fileName, bytes.NewBuffer(bodyBytes))
		if err != nil {
			http.Error(w, "Could not create client request", http.StatusInternalServerError)
			return
		}
		req.Header.Set("Content-Type", "text/plain")

		// send request to fileserver
		_, err = httpClient.Do(req)
		if err != nil {
			http.Error(w, fmt.Sprintf("Fileserver Error: %s", err.Error()), http.StatusInternalServerError)
			return
		}
	}(fileName, bodyBytes)
}

func getFile(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	log.Println("GET", r.URL.Path)

	fileName := r.PathValue("fileName")
	if fileName == "" {
		http.Error(w, "no file name given", http.StatusBadRequest)
		return
	}

	lock := fileLocks.get(fileName)
	lock.RLock()
	defer lock.RUnlock()

	var bodyBytes []byte
	var responseCode int

	// check cache
	val, err := redisClient.Get(ctx, fileName).Result()
	if err == nil { // cache hit

		bodyBytes = []byte(val)
		responseCode = 200

	} else { // cache miss so make request to fileserver
		log.Println("Cache Miss!")

		// get the shard from hash of filename
		shard := strconv.Itoa(int(hashKey(fileName)))
		shardUrl := strings.Replace(os.Getenv("FILE_SERVER_URL"), "#", shard, -1)

		// make new request to fileserver
		req, err := http.NewRequest(http.MethodGet, shardUrl+"/"+fileName, nil)
		if err != nil {
			http.Error(w, "Could not create client request", http.StatusInternalServerError)
			return
		}

		// send request to fileserver
		resp, err := httpClient.Do(req)
		if err != nil {
			http.Error(w, fmt.Sprintf("Fileserver Error: %s", err.Error()), http.StatusInternalServerError)
			return
		}

		// create body of response
		bodyBytes, err = io.ReadAll(resp.Body)
		responseCode = resp.StatusCode
		if err != nil {
			http.Error(w, fmt.Sprintf("Reading fileserver body error: %s", err.Error()), http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()
	}

	w.WriteHeader(responseCode)
	w.Write(bodyBytes)
}

func deleteFile(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	log.Println("DELETE", r.URL.Path)

	fileName := r.PathValue("fileName")
	if fileName == "" {
		http.Error(w, "no file name given", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	flusher, ok := w.(http.Flusher)
	if ok {
		flusher.Flush()
	}

	go func(fileName string) {
		lock := fileLocks.get(fileName)
		lock.Lock()
		defer lock.Unlock()

		// update cache cache
		err := redisClient.Del(ctx, fileName).Err()
		if err != nil {
			log.Println("Redis DELETE error")
		}

		// get the shard from hash of filename
		shard := strconv.Itoa(int(hashKey(fileName)))
		shardUrl := strings.Replace(os.Getenv("FILE_SERVER_URL"), "#", shard, -1)

		// make new request to fileserver
		req, err := http.NewRequest(http.MethodDelete, shardUrl+"/"+fileName, nil)
		if err != nil {
			http.Error(w, "Could not create client request", http.StatusInternalServerError)
			return
		}

		// send request to fileserver
		_, err = httpClient.Do(req)
		if err != nil {
			http.Error(w, fmt.Sprintf("Fileserver Error: %s", err.Error()), http.StatusInternalServerError)
			return
		}
	}(fileName)
}
