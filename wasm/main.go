package main

import (
	"fmt"
	"net/http"
	"time"

	spinhttp "github.com/spinframework/spin-go-sdk/v2/http"
)

// TinyGo has no embedded tzdata, so time.LoadLocation("Europe/Moscow") fails; Moscow is a fixed UTC+3.
var moscow = time.FixedZone("MSK", 3*60*60)

func init() {
	spinhttp.Handle(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		now := time.Now().In(moscow)
		w.Header().Set("Content-Type", "application/json")
		// fmt instead of encoding/json: TinyGo's reflection support is limited.
		fmt.Fprintf(w, "{\"timezone\":%q,\"unix\":%d,\"iso\":%q,\"hour_minute\":%q}\n",
			"Europe/Moscow (UTC+3)", now.Unix(), now.Format(time.RFC3339), now.Format("15:04"))
	})
}
