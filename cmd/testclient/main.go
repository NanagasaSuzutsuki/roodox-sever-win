package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"roodox_server/client"
)

func main() {
	opts := client.ConnectionOptions{
		SharedSecret:    os.Getenv("ROODOX_SHARED_SECRET"),
		TLSEnabled:      readBoolEnv("ROODOX_TLS_ENABLED"),
		TLSRootCertPath: os.Getenv("ROODOX_TLS_ROOT_CERT_PATH"),
		TLSServerName:   os.Getenv("ROODOX_TLS_SERVER_NAME"),
	}

	c, err := client.NewRoodoxClientWithOptions("127.0.0.1:50051", opts)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := c.WriteFile(ctx, "test.txt", []byte("hello roodox"), 0); err != nil {
		log.Fatal("WriteFile:", err)
	}

	history, err := c.GetHistory(ctx, "test.txt")
	if err != nil {
		log.Fatal("GetHistory:", err)
	}
	log.Printf("history: %+v\n", history)
}

func readBoolEnv(name string) bool {
	v := os.Getenv(name)
	if v == "" {
		return false
	}
	parsed, err := strconv.ParseBool(v)
	return err == nil && parsed
}
