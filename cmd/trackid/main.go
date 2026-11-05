// trackid runs the API server, serving both the core API and (when a frontend
// build is present) the prototype frontend. See server.Run.
package main

import (
	"log"

	"github.com/rodfileto/trackid/server"
)

func main() {
	if err := server.Run(server.Options{}); err != nil {
		log.Fatal(err)
	}
}
