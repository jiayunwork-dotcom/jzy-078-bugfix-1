package main

import (
	"log"
	"os"

	"cycloneservice/httpapi"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	r := httpapi.NewRouter()
	log.Printf("cyclone sizing service listening on :%s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
