package main

import (
	"fmt"
	"os"
	"strconv"
)

const AppName = "Customer Manager CLI (Phase 1)"
const DefaultLimit = 10

func main() {
	fmt.Println(AppName)

	// ====== Variables & Zero Values ======
	var page int // 0
	var limit int = DefaultLimit
	var search string // ""
	var active bool   // false

	fmt.Println("Default values:")
	fmt.Println("page =", page, "| limit =", limit, "| search =", search, "| active =", active)

	// ====== Args handling ======
	// contoh pemakaian:
	// go run ./cmd/app list-customers 2 5
	// artinya: command=list-customers, page=2, limit=5
	if len(os.Args) < 2 {
		fmt.Println("No command provided.")
		fmt.Println("Example: go run ./cmd/app list-customers 2 5")
		return
	}

	command := os.Args[1]

	// parse optional args
	// os.Args[2] = page, os.Args[3] = limit
	if len(os.Args) >= 3 {
		p, err := strconv.Atoi(os.Args[2])
		if err != nil {
			fmt.Println("Invalid page. Must be integer.")
			return
		}
		page = p
	}

	if len(os.Args) >= 4 {
		l, err := strconv.Atoi(os.Args[3])
		if err != nil {
			fmt.Println("Invalid limit. Must be integer.")
			return
		}
		limit = l
	}

	// boolean demo (aktifkan kalau ada arg "active")
	// go run ./cmd/app list-customers 1 10 active
	if len(os.Args) >= 5 && os.Args[4] == "active" {
		active = true
	}

	// string demo (search)
	// go run ./cmd/app list-customers 1 10 active andi
	if len(os.Args) >= 6 {
		search = os.Args[5]
	}

	fmt.Println("\nArgs:", os.Args)
	fmt.Println("\nParsed Input:")
	fmt.Printf("command=%s page=%d limit=%d active=%t search=%q\n", command, page, limit, active, search)

	// ====== Type conversion demo ======
	// (page, limit) -> offset = (page-1)*limit
	if page > 0 {
		offset := (page - 1) * limit
		fmt.Println("offset =", offset)
	} else {
		fmt.Println("offset not computed (page must be > 0)")
	}
}
