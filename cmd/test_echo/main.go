package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

func main() {
	if err := database.InitPostgres(); err != nil {
		log.Fatal(err)
	}
	graph, err := services.GenerateEchoLocation(context.Background(), "2dc5179f-290b-4997-9528-213b75f8087d")
	if err != nil {
		log.Fatal(err)
	}
	b, _ := json.MarshalIndent(graph, "", "  ")
	fmt.Println(string(b))
}
