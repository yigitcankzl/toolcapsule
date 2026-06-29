package main

import (
	"encoding/json"
	"os"
	"time"
)

func main() {
	time.Sleep(2 * time.Second)
	_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"status": "finished"})
}
