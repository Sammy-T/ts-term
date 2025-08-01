package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	fmt.Println("Running server...")

	fsHandler := http.FileServer(http.Dir("web/dist"))

	http.Handle("/", fsHandler)

	log.Fatal(http.ListenAndServe(":3000", nil))
}
