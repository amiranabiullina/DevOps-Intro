// CGI/WAGI-shaped WASI module: the request comes in as env vars, the response goes to stdout.
package main

import (
	"fmt"
	"os"
	"time"
)

// Same logic as the Spin component in wasm/moscow-time: fixed UTC+3, JSON built with fmt.
var moscow = time.FixedZone("MSK", 3*60*60)

func main() {
	method, path := os.Getenv("REQUEST_METHOD"), os.Getenv("PATH_INFO")

	switch {
	case path != "/time":
		fmt.Print("Status: 404 Not Found\nContent-Type: text/plain\n\nnot found\n")
	case method != "GET":
		fmt.Print("Status: 405 Method Not Allowed\nAllow: GET\nContent-Type: text/plain\n\nmethod not allowed\n")
	default:
		now := time.Now().In(moscow)
		fmt.Print("Content-Type: application/json\n\n")
		fmt.Printf("{\"timezone\":%q,\"unix\":%d,\"iso\":%q,\"hour_minute\":%q}\n",
			"Europe/Moscow (UTC+3)", now.Unix(), now.Format(time.RFC3339), now.Format("15:04"))
	}
}
