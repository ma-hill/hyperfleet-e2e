package main

import (
	"log"
	"os"

	specschemas "github.com/openshift-hyperfleet/hyperfleet-api-spec/schemas"
)

func main() {
	data, err := specschemas.FS.ReadFile("core/openapi.yaml")
	if err != nil {
		log.Fatalf("failed to read embedded schema: %v", err)
	}
	if err := os.WriteFile("openapi/openapi.yaml", data, 0600); err != nil {
		log.Fatalf("failed to write schema: %v", err)
	}
}
