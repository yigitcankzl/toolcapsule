package main

import (
	"encoding/json"
	"net/http"
	"os"
)

func main() {
	_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"mode": "docker", "method": http.MethodGet})
}
