// trackid runs the API server, serving both the core API and (when a frontend
// build is present) the prototype frontend. See internal/server.Run.
package main

import (
	"log"

	"github.com/rodfileto/trackid/internal/server"
)

func main() {
	if err := server.Run(); err != nil {
		log.Fatal(err)
	}
}
