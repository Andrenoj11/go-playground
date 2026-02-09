package main

import (
	"encoding/json"
	"log"
	"net/http"
)

// User adalah "bentuk data" yang akan kita kirim/terima lewat API
type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// users = "database sementara" (disimpan di memory, hilang kalau server mati)
var users = []User{
	{ID: 1, Name: "Abdi"},
	{ID: 2, Name: "Budi"},
}

// usersHandler menangani route "/users" untuk method GET dan POST
func usersHandler(w http.ResponseWriter, r *http.Request) {
	// Header ini memberi tahu client bahwa response kita JSON
	w.Header().Set("Content-Type", "application/json")

	// Kita bedakan behavior berdasarkan HTTP method
	switch r.Method {
	case http.MethodGet:
		// GET /users -> balikin seluruh list users
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(users)

	case http.MethodPost:
		// POST /users -> baca body JSON, lalu tambahkan ke slice

		// 1) Decode JSON dari body request ke struct User
		var payload User
		err := json.NewDecoder(r.Body).Decode(&payload)
		if err != nil {
			// Kalau body bukan JSON yang valid
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "invalid JSON body",
			})
			return
		}

		// 2) Validasi sederhana: Name tidak boleh kosong
		if payload.Name == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "name is required",
			})
			return
		}

		// 3) Buat ID baru (cara paling sederhana untuk pemula)
		newID := 1
		if len(users) > 0 {
			newID = users[len(users)-1].ID + 1
		}

		// 4) Buat user baru, lalu simpan
		newUser := User{ID: newID, Name: payload.Name}
		users = append(users, newUser)

		// 5) Balikin user yang baru dibuat
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode("Berhasil")

	default:
		// Kalau method bukan GET/POST -> 405 Method Not Allowed
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "method not allowed",
		})
	}
}

func main() {
	// Route: kalau ada request ke /users, panggil usersHandler
	http.HandleFunc("/users", usersHandler)

	// Info di terminal biar kamu tahu server sudah hidup
	log.Println("Server running on http://localhost:8080")

	// Nyalakan server di port 8080
	// Kalau error (misal port kepakai), program akan berhenti dan log error
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal(err)
	}
}
