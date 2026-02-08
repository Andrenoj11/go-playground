package main

import (
	"fmt"
	"net/http"
)

func handler(hw http.ResponseWriter, hr *http.Request) {
	fmt.Fprintln(hw, "Hello API")
}

func main() {
	http.HandleFunc("/", handler)
	http.ListenAndServe(":8000", nil)
}
