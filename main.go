package main

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	_ "github.com/jackc/pgx/v5/stdlib"

	// Swagger (opsional): pastikan kamu sudah `swag init` dan module path sesuai go.mod
	_ "go-playground/docs"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// =====================================================
// Swagger Metadata
// =====================================================

// @title           Customer Manager API
// @version         0.1.0
// @description     Simple REST API for managing customers.
// @BasePath        /
// @schemes         http

// =====================================================
// Models
// =====================================================

type Customer struct {
	ID        int64
	FullName  string
	Email     string
	Phone     sql.NullString
	IsActive  bool
	CreatedAt time.Time
}

type CustomerResponse struct {
	ID        int64     `json:"id"`
	FullName  string    `json:"full_name"`
	Email     string    `json:"email"`
	Phone     *string   `json:"phone"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateCustomerRequest struct {
	FullName string `json:"full_name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
}

// ===== Bulk Validate (Concurrency) =====

type BulkValidateRequest struct {
	Workers   int               `json:"workers"`
	Customers []CustomerPayload `json:"customers"`
}

type CustomerPayload struct {
	FullName string `json:"full_name"`
	Email    string `json:"email"`
}

type ValidateResult struct {
	Index   int    `json:"index"`
	Email   string `json:"email"`
	Valid   bool   `json:"valid"`
	Message string `json:"message,omitempty"`
}

// ===== Swagger wrapper responses =====

type APIError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type CustomersListResponse struct {
	Data []CustomerResponse `json:"data"`
	Meta struct {
		Page  int `json:"page"`
		Limit int `json:"limit"`
		Count int `json:"count"`
	} `json:"meta"`
}

type CustomerSingleResponse struct {
	Data CustomerResponse `json:"data"`
}

// =====================================================
// main
// =====================================================

func main() {
	_ = godotenv.Load()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		panic("DATABASE_URL is empty. Set it in .env or export DATABASE_URL")
	}

	apiKey := os.Getenv("API_KEY") // untuk sesi 6

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	// Fail fast: cek koneksi DB saat startup
	{
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			panic(err)
		}
	}

	r := gin.Default()

	// request-id middleware (sesi 6)
	r.Use(requestIDMiddleware())

	// Swagger UI (opsional)
	// Akses: http://localhost:8080/docs/index.html
	r.GET("/docs/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Health basic (public)
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now().Format(time.RFC3339)})
	})

	// Health DB (public)
	r.GET("/db/health", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		if err := db.PingContext(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":  "down",
				"db":      "unreachable",
				"message": err.Error(),
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "db": "up"})
	})

	// =====================================================
	// Sesi 2: Concurrency Pattern (Worker Pool) - public
	// =====================================================
	r.POST("/customers/bulk-validate", func(c *gin.Context) {
		var req BulkValidateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, apiError("invalid_json", err.Error()))
			return
		}

		workers := req.Workers
		if workers <= 0 {
			workers = 5
		}
		if workers > 20 {
			workers = 20
		}

		start := time.Now()
		results := bulkValidateWithWorkerPool(req.Customers, workers)
		elapsed := time.Since(start)

		c.JSON(http.StatusOK, gin.H{
			"workers": workers,
			"count":   len(req.Customers),
			"elapsed": elapsed.String(),
			"results": results,
		})
	})

	// =====================================================
	// Sesi 3: Customers REST API + PostgreSQL
	// =====================================================

	// ListCustomers godoc
	// @Summary      List customers
	// @Description  Get customers with pagination and optional search
	// @Tags         customers
	// @Accept       json
	// @Produce      json
	// @Param        page   query     int     false  "Page number" default(1)
	// @Param        limit  query     int     false  "Limit per page" default(10)
	// @Param        search query     string  false  "Search by name/email"
	// @Param        active query     bool    false  "Active only" default(true)
	// @Success      200    {object}  CustomersListResponse
	// @Failure      500    {object}  APIError
	// @Router       /customers [get]
	r.GET("/customers", func(c *gin.Context) {
		page := parseIntDefault(c.Query("page"), 1)
		limit := parseIntDefault(c.Query("limit"), 10)
		search := strings.TrimSpace(c.Query("search"))
		activeOnly := parseBoolDefault(c.Query("active"), true)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		customers, err := listCustomers(ctx, db, activeOnly, search, page, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, apiError("internal_error", err.Error()))
			return
		}

		resp := make([]CustomerResponse, 0, len(customers))
		for _, cu := range customers {
			resp = append(resp, toCustomerResponse(cu))
		}

		c.JSON(http.StatusOK, gin.H{
			"data": resp,
			"meta": gin.H{"page": page, "limit": limit, "count": len(resp)},
		})
	})

	// GetCustomer godoc
	// @Summary      Get customer by ID
	// @Tags         customers
	// @Produce      json
	// @Param        id   path      int  true  "Customer ID"
	// @Success      200  {object}  CustomerSingleResponse
	// @Failure      400  {object}  APIError
	// @Failure      404  {object}  APIError
	// @Failure      500  {object}  APIError
	// @Router       /customers/{id} [get]
	r.GET("/customers/:id", func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, apiError("invalid_id", "id must be a positive integer"))
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		cu, err := getCustomerByID(ctx, db, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, apiError("not_found", "customer not found"))
				return
			}
			c.JSON(http.StatusInternalServerError, apiError("internal_error", err.Error()))
			return
		}

		c.JSON(http.StatusOK, gin.H{"data": toCustomerResponse(cu)})
	})

	// =====================================================
	// Sesi 6: Auth Token + Middleware (protect POST /customers)
	// =====================================================
	protected := r.Group("/")
	protected.Use(apiKeyAuthMiddleware(apiKey))

	// CreateCustomer godoc
	// @Summary      Create a customer
	// @Tags         customers
	// @Accept       json
	// @Produce      json
	// @Param        payload  body      CreateCustomerRequest  true  "Customer payload"
	// @Success      201      {object}  CustomerSingleResponse
	// @Failure      400      {object}  APIError
	// @Failure      401      {object}  APIError
	// @Failure      409      {object}  APIError
	// @Failure      500      {object}  APIError
	// @Router       /customers [post]
	protected.POST("/customers", func(c *gin.Context) {
		var req CreateCustomerRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, apiError("invalid_json", err.Error()))
			return
		}

		req.FullName = strings.TrimSpace(req.FullName)
		req.Email = strings.ToLower(strings.TrimSpace(req.Email))
		req.Phone = strings.TrimSpace(req.Phone)

		if req.FullName == "" {
			c.JSON(http.StatusBadRequest, apiError("validation_error", "full_name is required"))
			return
		}
		if req.Email == "" || !strings.Contains(req.Email, "@") {
			c.JSON(http.StatusBadRequest, apiError("validation_error", "email must be valid"))
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		created, err := createCustomer(ctx, db, req.FullName, req.Email, req.Phone)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "duplicate key value") {
				c.JSON(http.StatusConflict, apiError("duplicate", "email already exists"))
				return
			}
			c.JSON(http.StatusInternalServerError, apiError("internal_error", err.Error()))
			return
		}

		c.JSON(http.StatusCreated, gin.H{"data": toCustomerResponse(created)})
	})

	r.Run(":8080")
}

// =====================================================
// Middleware
// =====================================================

func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := strconv.FormatInt(time.Now().UnixNano(), 10)
		c.Writer.Header().Set("X-Request-Id", rid)
		c.Set("request_id", rid)
		c.Next()
	}
}

func apiKeyAuthMiddleware(apiKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		got := strings.TrimSpace(c.GetHeader("X-API-Key"))
		if apiKey == "" {
			c.AbortWithStatusJSON(http.StatusInternalServerError, apiError("misconfigured", "API_KEY is not set"))
			return
		}
		if got == "" || got != apiKey {
			c.AbortWithStatusJSON(http.StatusUnauthorized, apiError("unauthorized", "invalid or missing X-API-Key"))
			return
		}
		c.Next()
	}
}

// =====================================================
// Helpers (API response)
// =====================================================

func apiError(code, message string) gin.H {
	return gin.H{"error": gin.H{"code": code, "message": message}}
}

func toCustomerResponse(c Customer) CustomerResponse {
	var phone *string
	if c.Phone.Valid {
		p := c.Phone.String
		phone = &p
	}
	return CustomerResponse{
		ID:        c.ID,
		FullName:  c.FullName,
		Email:     c.Email,
		Phone:     phone,
		IsActive:  c.IsActive,
		CreatedAt: c.CreatedAt,
	}
}

// =====================================================
// Helpers (parse)
// =====================================================

func parseIntDefault(s string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func parseBoolDefault(s string, def bool) bool {
	if strings.TrimSpace(s) == "" {
		return def
	}
	b, err := strconv.ParseBool(s)
	if err != nil {
		return def
	}
	return b
}

// =====================================================
// DB functions
// =====================================================

func listCustomers(ctx context.Context, db *sql.DB, activeOnly bool, search string, page, limit int) ([]Customer, error) {
	offset := (page - 1) * limit
	searchLike := "%" + search + "%"

	q := `
SELECT id, full_name, email, phone, is_active, created_at
FROM customers
WHERE ($1::bool = false OR is_active = true)
  AND (full_name ILIKE $2 OR email ILIKE $2)
ORDER BY id ASC
LIMIT $3 OFFSET $4;
`
	rows, err := db.QueryContext(ctx, q, activeOnly, searchLike, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Customer
	for rows.Next() {
		var c Customer
		if err := rows.Scan(&c.ID, &c.FullName, &c.Email, &c.Phone, &c.IsActive, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func getCustomerByID(ctx context.Context, db *sql.DB, id int64) (Customer, error) {
	q := `
SELECT id, full_name, email, phone, is_active, created_at
FROM customers
WHERE id = $1;
`
	var c Customer
	err := db.QueryRowContext(ctx, q, id).Scan(&c.ID, &c.FullName, &c.Email, &c.Phone, &c.IsActive, &c.CreatedAt)
	if err != nil {
		return Customer{}, err
	}
	return c, nil
}

func createCustomer(ctx context.Context, db *sql.DB, fullName, email, phone string) (Customer, error) {
	q := `
INSERT INTO customers (full_name, email, phone)
VALUES ($1, $2, $3)
RETURNING id, full_name, email, phone, is_active, created_at;
`
	var phoneNull sql.NullString
	if phone != "" {
		phoneNull = sql.NullString{String: phone, Valid: true}
	}

	var c Customer
	err := db.QueryRowContext(ctx, q, fullName, email, phoneNull).Scan(&c.ID, &c.FullName, &c.Email, &c.Phone, &c.IsActive, &c.CreatedAt)
	if err != nil {
		return Customer{}, err
	}
	return c, nil
}

// =====================================================
// Concurrency: Worker Pool Bulk Validate
// =====================================================

func bulkValidateWithWorkerPool(customers []CustomerPayload, workers int) []ValidateResult {
	type job struct {
		index int
		data  CustomerPayload
	}

	jobs := make(chan job)
	resultsCh := make(chan ValidateResult)

	// start workers
	for w := 0; w < workers; w++ {
		go func() {
			for j := range jobs {
				time.Sleep(120 * time.Millisecond) // simulasi kerja
				ok, msg := validateCustomer(j.data)
				resultsCh <- ValidateResult{
					Index:   j.index,
					Email:   strings.TrimSpace(j.data.Email),
					Valid:   ok,
					Message: msg,
				}
			}
		}()
	}

	// producer
	go func() {
		for i, c := range customers {
			jobs <- job{index: i, data: c}
		}
		close(jobs)
	}()

	// collect N results
	out := make([]ValidateResult, len(customers))
	for i := 0; i < len(customers); i++ {
		r := <-resultsCh
		out[r.Index] = r
	}
	return out
}

func validateCustomer(c CustomerPayload) (bool, string) {
	name := strings.TrimSpace(c.FullName)
	email := strings.TrimSpace(c.Email)

	if name == "" {
		return false, "full_name is required"
	}
	if email == "" {
		return false, "email is required"
	}
	if !strings.Contains(email, "@") {
		return false, "email must contain '@'"
	}
	if len(email) > 150 {
		return false, "email too long"
	}
	if strings.HasSuffix(strings.ToLower(email), "@example.com") {
		return false, "example.com emails are not allowed"
	}
	return true, ""
}
