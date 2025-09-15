package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"rsc.io/quote"
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
var fileLocks = newKeyedLocks()

const port string = "8080"
const serverUrl string = "http://localhost:900#/api/fileserver"

func hashKey(key string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(key))
	return (h.Sum32() % 5) + 1
}

func main() {
	test := quote.Hello()
	fmt.Println(test)

	// a request multiplexer distributes requests to their corresponding url endpoints or "patterns"
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleRoot)
	mux.HandleFunc("GET /health", getHealth)
	mux.HandleFunc("PUT /api/fileserver/{fileName}", putFile)
	mux.HandleFunc("GET /api/fileserver/{fileName}", getFile)
	mux.HandleFunc("DELETE /api/fileserver/{fileName}", deleteFile)

	fmt.Printf("Server listening to localhost:%s...", port)
	http.ListenAndServe(":"+port, mux)
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
	// get url param
	fileName := r.PathValue("fileName")
	if fileName == "" {
		http.Error(w, "no file name given", http.StatusBadRequest)
		return
	}
	// get the shard from hash of filename
	shard := strconv.Itoa(int(hashKey(fileName)))
	shardUrl := strings.Replace(serverUrl, "#", shard, -1)

	// read body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close() // Close body after reading bytes

	lock := fileLocks.get(fileName)
	lock.Lock()
	defer lock.Unlock()

	// make new request to fileserver
	req, err := http.NewRequest(http.MethodPut, shardUrl+"/"+fileName, bytes.NewBuffer(bodyBytes))
	if err != nil {
		http.Error(w, "Could not create client request", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "text/plain")

	// send request to fileserver
	resp, err := httpClient.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("Fileserver Error: %s", err.Error()), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(resp.StatusCode)
}

func getFile(w http.ResponseWriter, r *http.Request) {
	fileName := r.PathValue("fileName")
	if fileName == "" {
		http.Error(w, "no file name given", http.StatusBadRequest)
		return
	}
	// get the shard from hash of filename
	shard := strconv.Itoa(int(hashKey(fileName)))
	shardUrl := strings.Replace(serverUrl, "#", shard, -1)

	lock := fileLocks.get(fileName)
	lock.RLock()
	defer lock.RUnlock()

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
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Reading fileserver body error: %s", err.Error()), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	w.WriteHeader(resp.StatusCode)
	w.Write(bodyBytes)
}

func deleteFile(w http.ResponseWriter, r *http.Request) {
	fileName := r.PathValue("fileName")
	if fileName == "" {
		http.Error(w, "no file name given", http.StatusBadRequest)
		return
	}

	// get the shard from hash of filename
	shard := strconv.Itoa(int(hashKey(fileName)))
	shardUrl := strings.Replace(serverUrl, "#", shard, -1)

	lock := fileLocks.get(fileName)
	lock.Lock()
	defer lock.Unlock()

	// make new request to fileserver
	req, err := http.NewRequest(http.MethodDelete, shardUrl+"/"+fileName, nil)
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

	w.WriteHeader(resp.StatusCode)
}
