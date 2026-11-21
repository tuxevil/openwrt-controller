package services

import (
	"database/sql"
	"os"
	"testing"

	"github.com/google/uuid"

	"openwrt-controller/internal/database"
)

func TestSentinelLearningConcurrentInitializationPostgres(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL_TEST")
	if os.Getenv("OPENWRT_INTEGRATION_DB") != "1" || dsn == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	oldDB := database.DB
	database.DB = db
	defer func() { database.DB = oldDB }()
	schema := "tenant_learning_" + uuid.NewString()[:8]
	quoted, err := database.SafeSQLSchemaIdent(schema)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA IF EXISTS " + quoted + " CASCADE")
	defer sentinelLearningSchemaReady.Delete(schema)
	if err := database.RunTenantMigrations(schema); err != nil {
		t.Fatal(err)
	}
	const callers = 8
	start := make(chan struct{})
	results := make(chan error, callers)
	for range callers {
		go func() {
			<-start
			results <- ensureSentinelLearningTables(schema)
		}()
	}
	close(start)
	for range callers {
		if err := <-results; err != nil {
			t.Error(err)
		}
	}
}
